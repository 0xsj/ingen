package explain_test

import (
	"bytes"
	"strings"
	"testing"

	"ingen/paddock/internal/explain"
	"ingen/paddock/internal/model"
)

func TestExplainIncludesEvidenceAndRemediation(t *testing.T) {
	result := &model.Result{
		Schema: "paddock.report/v1",
		Root:   "/service",
		Rules: []model.RuleSummary{
			{
				ID:      "domain-is-pure",
				Kind:    "allow-dependencies",
				Allow:   []string{"standard-library:errors", "{role: port}"},
				Message: "the domain depends on stable abstractions",
			},
		},
		Findings: []*model.Finding{
			{
				RuleID:        "domain-is-pure",
				Kind:          "allow-dependencies",
				Severity:      "error",
				From:          "internal/orders/domain",
				FromComponent: "domain",
				To:            "net/http",
				File:          "internal/orders/domain/order.go",
				Line:          3,
				Message:       "the domain depends on stable abstractions",
			},
		},
	}

	document := explain.Explain(result)
	if document.Status != "FAIL" || document.Triage.Outcome != "remediate" || len(document.Findings) != 1 || len(document.Summary) != 1 {
		t.Fatalf("unexpected explanation summary: %#v", document)
	}
	if summary := document.Summary[0]; summary.RuleID != "domain-is-pure" || summary.Findings != 1 || summary.Active != 1 || summary.Blocking != 1 {
		t.Fatalf("unexpected finding summary: %#v", summary)
	}
	finding := document.Findings[0]
	if !strings.Contains(finding.Observation, "order.go:3 imports net/http") {
		t.Fatalf("observation missing source evidence: %q", finding.Observation)
	}
	if finding.Status != "blocking" {
		t.Fatalf("finding status = %q, want blocking", finding.Status)
	}
	if len(finding.SuggestedActions) != 1 || !strings.Contains(finding.SuggestedActions[0], "standard-library:errors") {
		t.Fatalf("remediation missing allowed targets: %#v", finding.SuggestedActions)
	}

	var output bytes.Buffer
	if err := explain.Text(&output, document); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "SUMMARY 1 rules, 1 findings, 1 blocking") || !strings.Contains(output.String(), "Next:") {
		t.Fatalf("text explanation missing remediation:\n%s", output.String())
	}
}

func TestExplainSummarizesFindingStatuses(t *testing.T) {
	result := &model.Result{
		Schema: "paddock.report/v1",
		Rules: []model.RuleSummary{
			{ID: "layer-direction", Kind: "layer-direction", Severity: "error"},
			{ID: "complete-classification", Kind: "coverage", Severity: "warning"},
		},
		Findings: []*model.Finding{
			{RuleID: "layer-direction", Kind: "layer-direction", Severity: "error"},
			{RuleID: "layer-direction", Kind: "layer-direction", Severity: "error", Waived: true},
			{RuleID: "layer-direction", Kind: "layer-direction", Severity: "error", Baselined: true},
			{RuleID: "complete-classification", Kind: "coverage", Severity: "warning"},
			{RuleID: "unknown-rule", Kind: "custom", Severity: "error"},
		},
	}

	document := explain.Explain(result)
	if len(document.Summary) != 3 {
		t.Fatalf("summary entries = %d, want 3: %#v", len(document.Summary), document.Summary)
	}
	if got := document.Summary[0]; got.RuleID != "complete-classification" || got.Findings != 1 || got.Active != 1 || got.Blocking != 0 {
		t.Fatalf("unexpected first summary entry: %#v", got)
	}
	if got := document.Summary[1]; got.RuleID != "layer-direction" || got.Findings != 3 || got.Active != 1 || got.Blocking != 1 || got.Waived != 1 || got.Baselined != 1 {
		t.Fatalf("unexpected second summary entry: %#v", got)
	}
	if got := document.Summary[2]; got.RuleID != "unknown-rule" || got.Kind != "custom" || got.Severity != "error" || got.Blocking != 1 {
		t.Fatalf("unexpected unknown-rule summary entry: %#v", got)
	}
	if document.Triage.Outcome != "remediate" || document.Triage.Findings != 5 || document.Triage.Active != 3 || document.Triage.Blocking != 2 || document.Triage.Accepted != 2 {
		t.Fatalf("unexpected triage summary: %#v", document.Triage)
	}
	for _, finding := range document.Findings {
		if finding.Finding.RuleID == "complete-classification" && finding.Status != "advisory" {
			t.Fatalf("warning finding status = %q, want advisory", finding.Status)
		}
	}

	filtered, err := explain.ApplyFilter(document, "layer-direction", "blocking")
	if err != nil {
		t.Fatal(err)
	}
	if filtered.Status != document.Status || filtered.Filter == nil || filtered.Filter.RuleID != "layer-direction" || filtered.Filter.Status != "blocking" || len(filtered.Findings) != 1 {
		t.Fatalf("unexpected filtered explanation: %#v", filtered)
	}
	if len(filtered.Summary) != 1 || filtered.Summary[0].Blocking != 1 {
		t.Fatalf("unexpected filtered summary: %#v", filtered.Summary)
	}
	if filtered.Triage.Outcome != "remediate" || filtered.Triage.Findings != 1 || filtered.Triage.Blocking != 1 {
		t.Fatalf("unexpected filtered triage: %#v", filtered.Triage)
	}

	advisory, err := explain.ApplyFilter(document, "", "advisory")
	if err != nil {
		t.Fatal(err)
	}
	if advisory.Triage.Outcome != "review" || advisory.Triage.Findings != 1 || advisory.Triage.Blocking != 0 {
		t.Fatalf("unexpected advisory triage: %#v", advisory.Triage)
	}

	accepted, err := explain.ApplyFilter(document, "", "waived")
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Triage.Outcome != "accepted" || accepted.Triage.Findings != 1 || accepted.Triage.Accepted != 1 {
		t.Fatalf("unexpected accepted triage: %#v", accepted.Triage)
	}

	if _, err := explain.ApplyFilter(document, "", "unknown"); err == nil {
		t.Fatal("invalid finding status unexpectedly accepted")
	}
}

func TestDenyExplanationIncludesDeniedTargets(t *testing.T) {
	result := &model.Result{
		Schema: "paddock.report/v1",
		Rules: []model.RuleSummary{
			{
				ID:      "config-is-portable",
				Kind:    "deny-dependencies",
				Deny:    []string{"path: src/server/**", "external: $env/**"},
				Message: "config must remain portable",
			},
		},
		Findings: []*model.Finding{
			{
				RuleID:   "config-is-portable",
				Kind:     "deny-dependencies",
				Severity: "error",
				From:     "src/config/runtime.ts",
				To:       "$env/static/private",
				Message:  "config must remain portable",
			},
		},
	}

	document := explain.Explain(result)
	if len(document.Findings) != 1 || len(document.Findings[0].SuggestedActions) != 1 {
		t.Fatalf("unexpected deny explanation: %#v", document)
	}
	action := document.Findings[0].SuggestedActions[0]
	if !strings.Contains(action, "path: src/server/**") || !strings.Contains(action, "external: $env/**") {
		t.Fatalf("deny explanation omitted target constraints: %q", action)
	}
}

func TestExplainRelatesRulesForTheSameDependencyEdge(t *testing.T) {
	result := &model.Result{
		Schema: "paddock.report/v1",
		Rules: []model.RuleSummary{
			{ID: "application-not-infrastructure", Kind: "deny-dependencies", Severity: "error"},
			{ID: "layers-point-inward", Kind: "layer-direction", Severity: "error"},
		},
		Findings: []*model.Finding{
			{
				RuleID: "application-not-infrastructure",
				Kind:   "deny-dependencies",
				From:   "internal/audit/app/query",
				To:     "internal/audit/infra/postgres",
				File:   "internal/audit/app/query/ledger.go",
				Line:   9,
			},
			{
				RuleID: "layers-point-inward",
				Kind:   "layer-direction",
				From:   "internal/audit/app/query",
				To:     "internal/audit/infra/postgres",
				File:   "internal/audit/app/query/ledger.go",
				Line:   9,
			},
		},
	}

	document := explain.Explain(result)
	if len(document.Findings) != 2 {
		t.Fatalf("finding count = %d, want 2", len(document.Findings))
	}
	if got := document.Findings[0].RelatedRules; len(got) != 1 || got[0] != "layers-point-inward" {
		t.Fatalf("first related rules = %#v", got)
	}
	if got := document.Findings[1].RelatedRules; len(got) != 1 || got[0] != "application-not-infrastructure" {
		t.Fatalf("second related rules = %#v", got)
	}

	var output bytes.Buffer
	if err := explain.Text(&output, document); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Related: also reported by") {
		t.Fatalf("text explanation omitted related-rule hint:\n%s", output.String())
	}
}
