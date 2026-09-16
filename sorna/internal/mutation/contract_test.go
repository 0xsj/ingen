package mutation

import (
	"strings"
	"testing"

	"ingen/sorna/internal/contract"
	"ingen/sorna/internal/oracle"
)

func TestApplyContractDoesNotModifyInputAndSupportsRuleMutations(t *testing.T) {
	document := contractMutationDocument()
	sealed, err := contract.Seal(document)
	if err != nil {
		t.Fatal(err)
	}

	mutated, err := ApplyContract(sealed.Document, Spec{
		ID:          "remove-name-rule",
		Plane:       "contract",
		Operator:    "contract.rule.expect.required.remove",
		Target:      "rule:document.create.accepted",
		Description: "remove a required response field",
		Change:      map[string]any{"field": "name"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sealed.Document.Contract["status"] != "sealed" {
		t.Fatalf("input status = %v, want sealed", sealed.Document.Contract["status"])
	}
	if mutated.Contract["status"] != "draft" {
		t.Fatalf("mutated status = %v, want draft before re-sealing", mutated.Contract["status"])
	}
	rules := mutated.Contract["rules"].([]any)
	first := rules[0].(map[string]any)
	body := first["expect"].(map[string]any)["body"].(map[string]any)
	if got := body["required"].([]any); len(got) != 1 || got[0] != "id" {
		t.Fatalf("required fields = %v, want only id", got)
	}
	originalRules := sealed.Document.Contract["rules"].([]any)
	originalBody := originalRules[0].(map[string]any)["expect"].(map[string]any)["body"].(map[string]any)
	if got := originalBody["required"].([]any); len(got) != 2 {
		t.Fatalf("input required fields = %v, want id and name", got)
	}
}

func TestApplyContractRejectsUnsafeTargetAndInvalidChange(t *testing.T) {
	document := contractMutationDocument()
	cases := []struct {
		name string
		spec Spec
		want string
	}{
		{
			name: "target must identify a rule",
			spec: Spec{ID: "bad-target", Plane: "contract", Operator: "contract.rule.remove", Target: "POST /documents", Change: map[string]any{"rule_id": "document.create.accepted"}},
			want: "target must use the form rule:<rule-id>",
		},
		{
			name: "from must match",
			spec: Spec{ID: "bad-from", Plane: "contract", Operator: "contract.rule.expect.status.replace", Target: "rule:document.create.accepted", Change: map[string]any{"from": 200, "to": 201}},
			want: "want declared from 200",
		},
		{
			name: "unsupported operator",
			spec: Spec{ID: "bad-operator", Plane: "contract", Operator: "contract.unknown", Target: "rule:document.create.accepted", Change: map[string]any{"value": true}},
			want: "unsupported contract mutation operator",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ApplyContract(document, test.spec); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ApplyContract() = %v, want error containing %q", err, test.want)
			}
		})
	}
}

func TestAnalyzeContractMutationsReportsVisibleEquivalentAndInvalid(t *testing.T) {
	sealed, err := contract.Seal(contractMutationDocument())
	if err != nil {
		t.Fatal(err)
	}
	originalOracle, err := oracle.Generate(sealed, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	report, err := AnalyzeContractMutations(sealed, originalOracle, []Spec{
		{
			ID:       "remove-rule",
			Plane:    "contract",
			Operator: "contract.rule.remove",
			Target:   "rule:document.create.accepted",
			Change:   map[string]any{"rule_id": "document.create.accepted"},
		},
		{
			ID:       "same-status",
			Plane:    "contract",
			Operator: "contract.rule.expect.status.replace",
			Target:   "rule:document.create.accepted",
			Change:   map[string]any{"from": 202, "to": 202},
		},
		{
			ID:       "change-status",
			Plane:    "contract",
			Operator: "contract.rule.expect.status.replace",
			Target:   "rule:document.create.accepted",
			Change:   map[string]any{"from": 202, "to": 201},
		},
		{
			ID:       "unsupported",
			Plane:    "contract",
			Operator: "contract.unknown",
			Target:   "rule:document.create.accepted",
			Change:   map[string]any{"value": true},
		},
		{
			ID:       "implementation-is-not-inspected",
			Plane:    "implementation",
			Operator: "response.status.replace",
			Target:   "POST /documents",
			Change:   map[string]any{"from": 202, "to": 200},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Schema != ContractMutationResultSchema || report.Status != "completed" {
		t.Fatalf("report identity = %+v", report)
	}
	if report.Summary.Analyzed != 4 || report.Summary.Visible != 2 || report.Summary.Equivalent != 1 || report.Summary.Invalid != 1 {
		t.Fatalf("summary = %+v, want 4 analyzed, 2 visible, 1 equivalent, 1 invalid", report.Summary)
	}
	if len(report.Entries) != 4 {
		t.Fatalf("entries = %d, want four contract entries", len(report.Entries))
	}
	if report.Entries[0].Outcome != "visible" || len(report.Entries[0].RemovedRuleIDs) != 1 {
		t.Fatalf("removed-rule entry = %+v, want visible with one removed rule", report.Entries[0])
	}
	if report.Entries[1].Outcome != "equivalent" {
		t.Fatalf("same-status entry = %+v, want equivalent", report.Entries[1])
	}
	if report.Entries[2].Outcome != "visible" || len(report.Entries[2].ChangedRuleIDs) != 1 {
		t.Fatalf("changed-status entry = %+v, want visible with one changed rule", report.Entries[2])
	}
	if report.Entries[3].Outcome != "invalid" || !strings.Contains(report.Entries[3].Reason, "unsupported contract mutation operator") {
		t.Fatalf("invalid entry = %+v, want unsupported operator diagnosis", report.Entries[3])
	}
	if _, err := CanonicalContractMutationReport(report); err != nil {
		t.Fatalf("CanonicalContractMutationReport() = %v", err)
	}
}

func contractMutationDocument() contract.Document {
	return contract.Document{Contract: map[string]any{
		"schema":    "ingen.contract/v1",
		"id":        "contract-mutation-test",
		"version":   int64(1),
		"status":    "draft",
		"interface": map[string]any{"kind": "http-json"},
		"rules": []any{
			map[string]any{
				"id":       "document.create.accepted",
				"strength": "must",
				"subject":  "POST /documents",
				"given":    map[string]any{"body": map[string]any{"name": "welcome.md"}},
				"expect": map[string]any{
					"status": int64(202),
					"body": map[string]any{
						"type":     "object",
						"required": []any{"id", "name"},
					},
				},
			},
			map[string]any{
				"id":       "document.health",
				"strength": "must",
				"subject":  "GET /healthz",
				"expect":   map[string]any{"status": int64(200)},
			},
		},
		"unspecified": []any{},
	}}
}
