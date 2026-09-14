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
	if document.Status != "FAIL" || len(document.Findings) != 1 {
		t.Fatalf("unexpected explanation summary: %#v", document)
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
	if !strings.Contains(output.String(), "Next:") {
		t.Fatalf("text explanation missing remediation:\n%s", output.String())
	}
}
