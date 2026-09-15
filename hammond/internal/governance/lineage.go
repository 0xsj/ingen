package governance

import (
	"fmt"
	"time"
)

// ValidateLineage validates records together, including references between
// predecessor and successor contract identities.
func ValidateLineage(records []Record) error {
	return ValidateLineageWithPolicy(records, DefaultReviewPolicy())
}

// ValidateLineageWithPolicy validates records and their references using an
// explicit review policy.
func ValidateLineageWithPolicy(records []Record, policy ReviewPolicy) error {
	problems := make([]string, 0)
	byIdentity := make(map[string]Record, len(records))
	for index, record := range records {
		if err := record.ValidateWithPolicy(policy); err != nil {
			problems = append(problems, fmt.Sprintf("record[%d]: %v", index, err))
		}
		key := record.Contract.Identity().Key()
		if _, exists := byIdentity[key]; exists {
			problems = append(problems, fmt.Sprintf("record[%d] duplicates contract identity %s", index, key))
		} else {
			byIdentity[key] = record
		}
	}

	edges := make(map[string]map[string]struct{})
	addEdge := func(from, to ContractIdentity, path string) {
		fromKey := from.Key()
		toKey := to.Key()
		if fromKey == toKey {
			problems = append(problems, path+" creates a self-link")
			return
		}
		if _, exists := byIdentity[fromKey]; !exists {
			problems = append(problems, fmt.Sprintf("%s references missing predecessor %s", path, fromKey))
		}
		if _, exists := byIdentity[toKey]; !exists {
			problems = append(problems, fmt.Sprintf("%s references missing successor %s", path, toKey))
		}
		if edges[fromKey] == nil {
			edges[fromKey] = make(map[string]struct{})
		}
		edges[fromKey][toKey] = struct{}{}
	}

	for recordIndex, record := range records {
		current := record.Contract.Identity()
		for eventIndex, event := range record.Events {
			path := fmt.Sprintf("record[%d].events[%d]", recordIndex, eventIndex)
			switch event.Type {
			case EventAmendmentCreated:
				if event.Predecessor != nil && event.Successor != nil {
					if !event.Predecessor.Equal(current) {
						problems = append(problems, path+" amendment predecessor does not match its record")
					}
					addEdge(*event.Predecessor, *event.Successor, path)
					if successorRecord, exists := byIdentity[event.Successor.Key()]; exists {
						if message := validateEventOrder(successorRecord, EventRegistered, event, "successor must be registered before amendment"); message != "" {
							problems = append(problems, path+" "+message)
						}
					}
				}
			case EventSuperseded:
				if event.Successor != nil {
					if !record.HasAmendmentLink(*event.Successor) {
						problems = append(problems, path+" supersession has no amendment link to successor")
					}
					addEdge(current, *event.Successor, path)
					if successorRecord, exists := byIdentity[event.Successor.Key()]; exists {
						if message := validateEventOrder(successorRecord, EventApprovalRecorded, event, "successor must be approved before supersession"); message != "" {
							problems = append(problems, path+" "+message)
						}
					}
				}
			}
		}
	}

	visited := make(map[string]bool)
	active := make(map[string]bool)
	var visit func(string)
	visit = func(node string) {
		if active[node] {
			problems = append(problems, fmt.Sprintf("lineage contains a cycle at %s", node))
			return
		}
		if visited[node] {
			return
		}
		visited[node] = true
		active[node] = true
		for next := range edges[node] {
			visit(next)
		}
		delete(active, node)
	}
	for key := range byIdentity {
		visit(key)
	}

	if len(problems) > 0 {
		return &ValidationError{Problems: problems}
	}
	return nil
}

func (record Record) HasAmendmentLink(successor ContractIdentity) bool {
	for _, event := range record.Events {
		if event.Type == EventAmendmentCreated && event.Successor != nil && event.Successor.Equal(successor) {
			return true
		}
	}
	return false
}

func validateEventOrder(record Record, prerequisite EventType, dependent Event, message string) string {
	prerequisiteAt, ok := firstEventAt(record, prerequisite)
	if !ok {
		return message
	}
	dependentAt, err := parseUTC(dependent.At)
	if err != nil {
		return ""
	}
	if prerequisiteAt.After(dependentAt) {
		return message
	}
	return ""
}

func firstEventAt(record Record, eventType EventType) (time.Time, bool) {
	for _, event := range record.Events {
		if event.Type != eventType {
			continue
		}
		at, err := parseUTC(event.At)
		if err == nil {
			return at, true
		}
	}
	return time.Time{}, false
}
