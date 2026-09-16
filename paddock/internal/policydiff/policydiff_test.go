package policydiff_test

import (
	"strings"
	"testing"

	"ingen/paddock/internal/policy"
	"ingen/paddock/internal/policydiff"
)

func TestCompareReportsSemanticPolicyChanges(t *testing.T) {
	before := policy.Policy{
		Project: "orders",
		Source:  policy.Source{Language: "go", Unit: "package", Roots: []string{"internal", "cmd"}},
		Components: map[string]policy.Component{
			"domain": {Match: policy.Patterns{"internal/domain/**"}},
		},
		Rules: []policy.Rule{{ID: "domain-pure", Kind: "allow-dependencies", Severity: "error"}},
	}
	after := before
	after.Source.Roots = []string{"cmd", "internal"}
	after.Rules = append(after.Rules, policy.Rule{ID: "no-cycles", Kind: "no-cycles", Severity: "warning"})
	after.Waivers = []policy.Waiver{{Rule: "domain-pure", From: "internal/domain/legacy.go", Reason: "migration", Owner: "platform", Expires: "2099-01-01"}}

	document := policydiff.Compare(before, after)
	if document.Schema != "paddock.policy-diff/v1" || document.Status != "changed" {
		t.Fatalf("unexpected document identity: %#v", document)
	}
	if document.Summary.Total != 2 || document.Summary.Added != 2 || document.Summary.Changed != 0 {
		t.Fatalf("unexpected diff summary: %#v", document.Summary)
	}
	if len(document.Changes) != 2 || document.Changes[0].Path != "rules.no-cycles" || document.Changes[1].Path != "waivers[0]" {
		t.Fatalf("unexpected changes: %#v", document.Changes)
	}
}

func TestCompareIgnoresRootOrdering(t *testing.T) {
	before := policy.Policy{Project: "demo", Source: policy.Source{
		Language: "go", Roots: []string{"cmd", "internal"},
		Include: []string{"src/**/*.ts", "src/**/*.svelte"}, Exclude: []string{"src/generated/**", "src/examples/**"},
	}}
	after := policy.Policy{Project: "demo", Source: policy.Source{
		Language: "go", Roots: []string{"internal", "cmd"},
		Include: []string{"src/**/*.svelte", "src/**/*.ts"}, Exclude: []string{"src/examples/**", "src/generated/**"},
	}}
	document := policydiff.Compare(before, after)
	if document.Status != "unchanged" || len(document.Changes) != 0 {
		t.Fatalf("root ordering created a semantic diff: %#v", document)
	}
}

func TestTextIncludesInputsAndChangeSummary(t *testing.T) {
	document := policydiff.Compare(
		policy.Policy{Project: "before"},
		policy.Policy{Project: "after"},
	)
	document.Before.Path = "before.yaml"
	document.After.Path = "after.yaml"
	var output strings.Builder
	if err := policydiff.Text(&output, document); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "POLICY-DIFF changed") ||
		!strings.Contains(output.String(), "before.yaml") ||
		!strings.Contains(output.String(), "project") {
		t.Fatalf("text diff is incomplete:\n%s", output.String())
	}
}

func TestValidateRejectsInconsistentSummary(t *testing.T) {
	document := policydiff.Compare(
		policy.Policy{Project: "before"},
		policy.Policy{Project: "after"},
	)
	document.Before.Path = "before.yaml"
	document.After.Path = "after.yaml"
	document.Summary.Total = 2
	if err := document.Validate(); err == nil || !strings.Contains(err.Error(), "summary does not match") {
		t.Fatalf("inconsistent policy diff was accepted: %v", err)
	}
}
