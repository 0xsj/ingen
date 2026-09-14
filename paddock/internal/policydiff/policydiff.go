package policydiff

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"

	"ingen/paddock/internal/policy"
)

const Schema = "paddock.policy-diff/v1"

type Input struct {
	Path            string `json:"path"`
	SHA256          string `json:"sha256,omitempty"`
	CanonicalSHA256 string `json:"canonical_sha256,omitempty"`
}

type Document struct {
	Schema  string   `json:"schema"`
	Status  string   `json:"status"`
	Before  Input    `json:"before"`
	After   Input    `json:"after"`
	Summary Summary  `json:"summary"`
	Changes []Change `json:"changes"`
}

type Summary struct {
	Added   int `json:"added"`
	Removed int `json:"removed"`
	Changed int `json:"changed"`
	Total   int `json:"total"`
}

type Change struct {
	Path   string          `json:"path"`
	Kind   string          `json:"kind"`
	Before json.RawMessage `json:"before,omitempty"`
	After  json.RawMessage `json:"after,omitempty"`
}

func Compare(before, after policy.Policy) Document {
	before = normalized(before)
	after = normalized(after)
	document := Document{Schema: Schema, Status: "unchanged", Changes: []Change{}}
	addScalarChanges(&document, before.Project, after.Project, "project")
	addScalarChanges(&document, before.Source.Language, after.Source.Language, "source.language")
	addScalarChanges(&document, before.Source.Unit, after.Source.Unit, "source.unit")
	addValueChange(&document, "source.roots", before.Source.Roots, after.Source.Roots)
	compareComponents(&document, before.Components, after.Components)
	compareRules(&document, before.Rules, after.Rules)
	compareWaivers(&document, before.Waivers, after.Waivers)
	if len(document.Changes) > 0 {
		document.Status = "changed"
	}
	for _, change := range document.Changes {
		switch change.Kind {
		case "added":
			document.Summary.Added++
		case "removed":
			document.Summary.Removed++
		case "changed":
			document.Summary.Changed++
		}
	}
	document.Summary.Total = len(document.Changes)
	return document
}

func normalized(input policy.Policy) policy.Policy {
	input.Source.Roots = append([]string(nil), input.Source.Roots...)
	sort.Strings(input.Source.Roots)
	return input
}

func addScalarChanges(document *Document, before, after, path string) {
	addValueChange(document, path, before, after)
}

func addValueChange(document *Document, path string, before, after any) {
	if reflect.DeepEqual(before, after) {
		return
	}
	document.Changes = append(document.Changes, Change{
		Path:   path,
		Kind:   "changed",
		Before: encodeValue(before),
		After:  encodeValue(after),
	})
}

func compareComponents(document *Document, before, after map[string]policy.Component) {
	keys := unionKeys(before, after)
	for _, key := range keys {
		left, leftOK := before[key]
		right, rightOK := after[key]
		path := "components." + key
		switch {
		case !leftOK:
			document.Changes = append(document.Changes, Change{Path: path, Kind: "added", After: encodeValue(right)})
		case !rightOK:
			document.Changes = append(document.Changes, Change{Path: path, Kind: "removed", Before: encodeValue(left)})
		default:
			addValueChange(document, path, left, right)
		}
	}
}

func compareRules(document *Document, before, after []policy.Rule) {
	left := make(map[string]policy.Rule, len(before))
	right := make(map[string]policy.Rule, len(after))
	for _, rule := range before {
		left[rule.ID] = rule
	}
	for _, rule := range after {
		right[rule.ID] = rule
	}
	keys := unionKeys(left, right)
	for _, key := range keys {
		beforeRule, beforeOK := left[key]
		afterRule, afterOK := right[key]
		path := "rules." + key
		switch {
		case !beforeOK:
			document.Changes = append(document.Changes, Change{Path: path, Kind: "added", After: encodeValue(afterRule)})
		case !afterOK:
			document.Changes = append(document.Changes, Change{Path: path, Kind: "removed", Before: encodeValue(beforeRule)})
		default:
			addValueChange(document, path, beforeRule, afterRule)
		}
	}
}

func compareWaivers(document *Document, before, after []policy.Waiver) {
	count := len(before)
	if len(after) > count {
		count = len(after)
	}
	for index := 0; index < count; index++ {
		path := fmt.Sprintf("waivers[%d]", index)
		switch {
		case index >= len(before):
			document.Changes = append(document.Changes, Change{Path: path, Kind: "added", After: encodeValue(after[index])})
		case index >= len(after):
			document.Changes = append(document.Changes, Change{Path: path, Kind: "removed", Before: encodeValue(before[index])})
		default:
			addValueChange(document, path, before[index], after[index])
		}
	}
}

func unionKeys[T any](before, after map[string]T) []string {
	seen := make(map[string]bool, len(before)+len(after))
	for key := range before {
		seen[key] = true
	}
	for key := range after {
		seen[key] = true
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func encodeValue(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(fmt.Sprintf("%q", err.Error()))
	}
	return json.RawMessage(data)
}

func Text(w io.Writer, document Document) error {
	if _, err := fmt.Fprintf(w, "POLICY-DIFF %s (%d changes)\n", document.Status, document.Summary.Total); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  before: %s\n  after:  %s\n", document.Before.Path, document.After.Path); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  summary: +%d added, -%d removed, ~%d changed\n", document.Summary.Added, document.Summary.Removed, document.Summary.Changed); err != nil {
		return err
	}
	for _, change := range document.Changes {
		before := string(bytes.TrimSpace(change.Before))
		after := string(bytes.TrimSpace(change.After))
		if before == "" {
			before = "<none>"
		}
		if after == "" {
			after = "<none>"
		}
		if _, err := fmt.Fprintf(w, "  [%s] %s: %s -> %s\n", change.Kind, change.Path, before, after); err != nil {
			return err
		}
	}
	return nil
}
