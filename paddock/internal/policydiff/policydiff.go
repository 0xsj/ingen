package policydiff

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"

	"ingen/paddock/internal/policy"
	"ingen/paddock/internal/policytest"
)

const Schema = "paddock.policy-diff/v1"

type Input struct {
	Path            string `json:"path"`
	SHA256          string `json:"sha256,omitempty"`
	CanonicalSHA256 string `json:"canonical_sha256,omitempty"`
}

type Document struct {
	Schema  string               `json:"schema"`
	Status  string               `json:"status"`
	Before  Input                `json:"before"`
	After   Input                `json:"after"`
	Summary Summary              `json:"summary"`
	Changes []Change             `json:"changes"`
	Tests   *policytest.Document `json:"tests,omitempty"`
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

func (d Document) Validate() error {
	if d.Schema != Schema {
		return fmt.Errorf("policy diff schema must be %s, got %q", Schema, d.Schema)
	}
	if d.Status != "unchanged" && d.Status != "changed" {
		return fmt.Errorf("policy diff has unsupported status %q", d.Status)
	}
	if d.Before.Path == "" || d.After.Path == "" {
		return fmt.Errorf("policy diff before and after paths are required")
	}
	for name, input := range map[string]Input{"before": d.Before, "after": d.After} {
		if input.SHA256 != "" && !isSHA256(input.SHA256) {
			return fmt.Errorf("policy diff %s sha256 must be a hexadecimal SHA-256 digest", name)
		}
		if input.CanonicalSHA256 != "" && !isSHA256(input.CanonicalSHA256) {
			return fmt.Errorf("policy diff %s canonical_sha256 must be a hexadecimal SHA-256 digest", name)
		}
	}
	if d.Summary.Added < 0 || d.Summary.Removed < 0 || d.Summary.Changed < 0 || d.Summary.Total < 0 || d.Summary.Total != len(d.Changes) || d.Summary.Added+d.Summary.Removed+d.Summary.Changed != d.Summary.Total {
		return fmt.Errorf("policy diff summary does not match changes")
	}
	wantStatus := "unchanged"
	if d.Summary.Total > 0 {
		wantStatus = "changed"
	}
	if d.Status != wantStatus {
		return fmt.Errorf("policy diff status does not match change count")
	}
	for _, change := range d.Changes {
		if change.Path == "" {
			return fmt.Errorf("policy diff changes require a path")
		}
		switch change.Kind {
		case "added":
			if len(change.After) == 0 {
				return fmt.Errorf("added policy diff change %q requires after", change.Path)
			}
		case "removed":
			if len(change.Before) == 0 {
				return fmt.Errorf("removed policy diff change %q requires before", change.Path)
			}
		case "changed":
			if len(change.Before) == 0 || len(change.After) == 0 {
				return fmt.Errorf("changed policy diff change %q requires before and after", change.Path)
			}
		default:
			return fmt.Errorf("policy diff change %q has unsupported kind %q", change.Path, change.Kind)
		}
	}
	if d.Tests != nil {
		if err := d.Tests.Validate(); err != nil {
			return fmt.Errorf("validate policy diff tests: %w", err)
		}
	}
	return nil
}

func isSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

func Compare(before, after policy.Policy) Document {
	before = normalized(before)
	after = normalized(after)
	document := Document{Schema: Schema, Status: "unchanged", Changes: []Change{}}
	addScalarChanges(&document, before.Project, after.Project, "project")
	addScalarChanges(&document, before.Source.Language, after.Source.Language, "source.language")
	addScalarChanges(&document, before.Source.Unit, after.Source.Unit, "source.unit")
	addValueChange(&document, "source.roots", before.Source.Roots, after.Source.Roots)
	addValueChange(&document, "source.include", before.Source.Include, after.Source.Include)
	addValueChange(&document, "source.exclude", before.Source.Exclude, after.Source.Exclude)
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
	input.Source.Include = append([]string(nil), input.Source.Include...)
	sort.Strings(input.Source.Include)
	input.Source.Exclude = append([]string(nil), input.Source.Exclude...)
	sort.Strings(input.Source.Exclude)
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
	if err := document.Validate(); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "POLICY-DIFF %s (%d changes)\n", document.Status, document.Summary.Total); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  before: %s\n  after:  %s\n", document.Before.Path, document.After.Path); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  summary: +%d added, -%d removed, ~%d changed\n", document.Summary.Added, document.Summary.Removed, document.Summary.Changed); err != nil {
		return err
	}
	if document.Tests != nil {
		if _, err := fmt.Fprintf(w, "  tests: %s (%d/%d passed)\n", document.Tests.Status, document.Tests.Passed, len(document.Tests.Cases)); err != nil {
			return err
		}
		for _, testCase := range document.Tests.Cases {
			if _, err := fmt.Fprintf(w, "    %s %s (expected %s, got %s)\n", testCase.Status, testCase.Name, testCase.Expected, testCase.Actual); err != nil {
				return err
			}
		}
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
