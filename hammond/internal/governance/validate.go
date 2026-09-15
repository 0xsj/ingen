package governance

import (
	"fmt"
	"strings"
	"time"
)

type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	return "invalid Hammond governance record: " + strings.Join(e.Problems, "; ")
}

func (record Record) Validate() error {
	return record.ValidateWithPolicy(DefaultReviewPolicy())
}

func (record Record) ValidateWithPolicy(policy ReviewPolicy) error {
	problems := make([]string, 0)
	if err := policy.Validate(); err != nil {
		problems = append(problems, "review policy: "+err.Error())
	}
	if record.Schema != Schema {
		problems = append(problems, fmt.Sprintf("schema must be %s", Schema))
	}
	if strings.TrimSpace(record.RecordID) == "" {
		problems = append(problems, "record_id is required")
	}
	problems = append(problems, validateContractReference(record.Contract, "contract")...)
	if !validState(record.State) {
		problems = append(problems, fmt.Sprintf("state %q is invalid", record.State))
	}
	if len(record.Events) == 0 {
		problems = append(problems, "events must contain at least one event")
	}

	seenIDs := make(map[string]struct{}, len(record.Events))
	seenReviewCycles := make(map[string]struct{}, len(record.Events))
	derivedState := State("")
	activeReviewCycleID := ""
	var previousAt time.Time
	for index, event := range record.Events {
		path := fmt.Sprintf("events[%d]", index)
		if _, exists := seenIDs[event.ID]; exists && strings.TrimSpace(event.ID) != "" {
			problems = append(problems, path+".id is duplicated")
		}
		if strings.TrimSpace(event.ID) != "" {
			seenIDs[event.ID] = struct{}{}
		}

		eventProblems, eventAt := validateEvent(event, record.Contract, activeReviewCycleID, path)
		problems = append(problems, eventProblems...)
		if !eventAt.IsZero() {
			if !previousAt.IsZero() && eventAt.Before(previousAt) {
				problems = append(problems, path+".at must not precede the previous event")
			}
			previousAt = eventAt
		}
		if event.Type == EventReviewOpened && strings.TrimSpace(event.ReviewCycleID) != "" {
			if _, exists := seenReviewCycles[event.ReviewCycleID]; exists {
				problems = append(problems, path+".review_cycle_id is duplicated")
			}
			seenReviewCycles[event.ReviewCycleID] = struct{}{}
			activeReviewCycleID = event.ReviewCycleID
		}

		nextState, err := transitionWithPolicy(derivedState, event, activeReviewCycleID, record.Events[:index], policy)
		if err != nil {
			problems = append(problems, path+": "+err.Error())
		} else {
			derivedState = nextState
		}
	}
	if derivedState != "" && record.State != derivedState {
		problems = append(problems, fmt.Sprintf("state is %q but events derive %q", record.State, derivedState))
	}

	if len(problems) > 0 {
		return &ValidationError{Problems: problems}
	}
	return nil
}

// AppendEvent applies one lifecycle event and returns a new materialized
// record. The existing record and its event slice are left unchanged.
func (record Record) AppendEvent(event Event) (Record, error) {
	return record.AppendEventWithPolicy(event, DefaultReviewPolicy())
}

// AppendEventWithPolicy applies one event using an explicit review policy and
// returns a new materialized record. The existing record and event slice are
// left unchanged.
func (record Record) AppendEventWithPolicy(event Event, policy ReviewPolicy) (Record, error) {
	if err := record.ValidateWithPolicy(policy); err != nil {
		return Record{}, err
	}
	nextState, err := transitionWithPolicy(record.State, event, activeReviewCycle(record.Events), record.Events, policy)
	if err != nil {
		return Record{}, err
	}
	updated := record
	updated.State = nextState
	updated.Events = append(append([]Event(nil), record.Events...), event)
	if err := updated.ValidateWithPolicy(policy); err != nil {
		return Record{}, err
	}
	return updated, nil
}

func validateContractReference(reference ContractReference, path string) []string {
	problems := make([]string, 0)
	if strings.TrimSpace(reference.ProjectID) == "" {
		problems = append(problems, path+".project_id is required")
	}
	if strings.TrimSpace(reference.ID) == "" {
		problems = append(problems, path+".id is required")
	}
	if reference.Version < 1 {
		problems = append(problems, path+".version must be positive")
	}
	if strings.TrimSpace(reference.Schema) == "" {
		problems = append(problems, path+".schema is required")
	}
	if strings.TrimSpace(reference.Artifact.URI) == "" {
		problems = append(problems, path+".artifact.uri is required")
	}
	if !validSHA256(reference.Artifact.SHA256) {
		problems = append(problems, path+".artifact.sha256 must be a lowercase SHA-256 digest")
	}
	return problems
}

func validateIdentity(identity *ContractIdentity, path string) []string {
	if identity == nil {
		return []string{path + " is required"}
	}
	problems := make([]string, 0)
	if strings.TrimSpace(identity.ProjectID) == "" {
		problems = append(problems, path+".project_id is required")
	}
	if strings.TrimSpace(identity.ID) == "" {
		problems = append(problems, path+".id is required")
	}
	if identity.Version < 1 {
		problems = append(problems, path+".version must be positive")
	}
	if strings.TrimSpace(identity.Schema) == "" {
		problems = append(problems, path+".schema is required")
	}
	if !validSHA256(identity.ArtifactSHA256) {
		problems = append(problems, path+".artifact_sha256 must be a lowercase SHA-256 digest")
	}
	return problems
}

func validateEvent(event Event, contract ContractReference, activeReviewCycleID, path string) ([]string, time.Time) {
	problems := make([]string, 0)
	if strings.TrimSpace(event.ID) == "" {
		problems = append(problems, path+".id is required")
	}
	if strings.TrimSpace(event.Actor) == "" {
		problems = append(problems, path+".actor is required")
	}
	eventAt, err := parseUTC(event.At)
	if err != nil {
		problems = append(problems, path+".at must be an RFC3339 UTC timestamp")
	}
	if !validEventType(event.Type) {
		problems = append(problems, path+".type is invalid")
		return problems, eventAt
	}

	switch event.Type {
	case EventReviewOpened:
		if strings.TrimSpace(event.ReviewCycleID) == "" {
			problems = append(problems, path+".review_cycle_id is required")
		}
	case EventApprovalRecorded:
		if strings.TrimSpace(event.Role) == "" {
			problems = append(problems, path+".role is required")
		}
		problems = append(problems, validateReviewCycle(event, activeReviewCycleID, path)...)
		if event.Decision != DecisionApprove {
			problems = append(problems, path+".decision must be approve")
		}
		problems = append(problems, validateDecisionDigest(event, contract, path)...)
	case EventRejectionRecorded:
		if strings.TrimSpace(event.Role) == "" {
			problems = append(problems, path+".role is required")
		}
		problems = append(problems, validateReviewCycle(event, activeReviewCycleID, path)...)
		if event.Decision != DecisionReject {
			problems = append(problems, path+".decision must be reject")
		}
		problems = append(problems, validateDecisionDigest(event, contract, path)...)
		if strings.TrimSpace(event.Reason) == "" {
			problems = append(problems, path+".reason is required")
		}
	case EventAmendmentCreated:
		problems = append(problems, validateIdentity(event.Predecessor, path+".predecessor")...)
		problems = append(problems, validateIdentity(event.Successor, path+".successor")...)
		if event.Predecessor != nil && event.Successor != nil {
			if event.Predecessor.Equal(*event.Successor) {
				problems = append(problems, path+".predecessor and successor must differ")
			}
			if event.Predecessor.ProjectID != event.Successor.ProjectID || event.Predecessor.ID != event.Successor.ID {
				problems = append(problems, path+".successor must retain project_id and id")
			}
			if event.Successor.Version <= event.Predecessor.Version {
				problems = append(problems, path+".successor.version must be greater than predecessor.version")
			}
		}
		if !validAmendmentKind(event.AmendmentKind) {
			problems = append(problems, path+".amendment_kind is invalid")
		}
		if strings.TrimSpace(event.Reason) == "" {
			problems = append(problems, path+".reason is required")
		}
	case EventSuperseded:
		problems = append(problems, validateIdentity(event.Successor, path+".successor")...)
	}

	return problems, eventAt
}

func validateReviewCycle(event Event, activeReviewCycleID, path string) []string {
	problems := make([]string, 0)
	if strings.TrimSpace(event.ReviewCycleID) == "" {
		problems = append(problems, path+".review_cycle_id is required")
	} else if activeReviewCycleID == "" {
		problems = append(problems, path+".review_cycle_id has no active review cycle")
	} else if event.ReviewCycleID != activeReviewCycleID {
		problems = append(problems, path+".review_cycle_id must match the active review cycle")
	}
	return problems
}

func validateDecisionDigest(event Event, contract ContractReference, path string) []string {
	problems := make([]string, 0)
	if !validSHA256(event.ArtifactSHA256) {
		problems = append(problems, path+".artifact_sha256 must be a lowercase SHA-256 digest")
	} else if event.ArtifactSHA256 != contract.Artifact.SHA256 {
		problems = append(problems, path+".artifact_sha256 must match contract.artifact.sha256")
	}
	return problems
}

func parseUTC(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	_, offset := parsed.Zone()
	if offset != 0 {
		return time.Time{}, fmt.Errorf("timestamp is not UTC")
	}
	return parsed, nil
}

func validState(state State) bool {
	switch state {
	case StateRegistered, StateInReview, StateApproved, StateRejected, StateSuperseded:
		return true
	default:
		return false
	}
}

func validEventType(eventType EventType) bool {
	switch eventType {
	case EventRegistered, EventReviewOpened, EventApprovalRecorded, EventRejectionRecorded, EventAmendmentCreated, EventSuperseded:
		return true
	default:
		return false
	}
}

func validAmendmentKind(kind AmendmentKind) bool {
	switch kind {
	case AmendmentClarifying, AmendmentAdditive, AmendmentRestrictive, AmendmentCorrective, AmendmentBreaking:
		return true
	default:
		return false
	}
}
