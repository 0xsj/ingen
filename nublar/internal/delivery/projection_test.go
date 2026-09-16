package delivery

import (
	"encoding/json"
	"strings"
	"testing"

	"ingen/core/ciresult"
	"ingen/nublar/internal/run"
)

func TestProjectPreservesDecisionAndReferencesWithoutProducerReport(t *testing.T) {
	record := deliveryTestRun()
	decision, err := Project(record)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Schema != Schema || decision.RunID != record.RunID || decision.Workflow.ID != record.Workflow.ID || decision.Status != "passed" || decision.ExitCode != 0 {
		t.Fatalf("decision = %+v, want run decision metadata", decision)
	}
	if len(decision.Checks) != 1 || decision.Checks[0].Result == nil || decision.Checks[0].Result.SHA256 != strings.Repeat("b", 64) {
		t.Fatalf("decision checks = %+v, want result reference", decision.Checks)
	}
	encoded, err := json.Marshal(decision)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "producer report") || strings.Contains(string(encoded), "artifact") {
		t.Fatalf("delivery projection contains producer-owned data: %s", encoded)
	}
}

func TestProjectRejectsInvalidRun(t *testing.T) {
	if _, err := Project(run.Run{}); err == nil {
		t.Fatal("Project() succeeded, want invalid-run error")
	}
}

func TestReceiptValidation(t *testing.T) {
	accepted := Receipt{
		Schema:      ReceiptSchema,
		RunID:       "run-receipt-01",
		Transport:   "http-webhook",
		Status:      "accepted",
		HTTPStatus:  204,
		AttemptedAt: "2026-09-15T12:00:02Z",
	}
	if err := accepted.Validate(); err != nil {
		t.Fatal(err)
	}
	failed := accepted
	failed.Status = "failed"
	failed.HTTPStatus = 502
	failed.Error = "Nublar webhook returned HTTP 502 Bad Gateway"
	if err := failed.Validate(); err != nil {
		t.Fatal(err)
	}
	failed.Error = ""
	if err := failed.Validate(); err == nil {
		t.Fatal("failed receipt validated without an error")
	}
}

func deliveryTestRun() run.Run {
	return run.Run{
		Schema:      run.Schema,
		RunID:       "run-delivery-01",
		Workflow:    run.Workflow{ID: "delivery-workflow", File: ciresult.FileRef{Path: "workflow.yaml", SHA256: strings.Repeat("a", 64)}},
		Status:      "passed",
		ExitCode:    0,
		CreatedAt:   "2026-09-15T12:00:00Z",
		CompletedAt: "2026-09-15T12:00:01Z",
		Checks: []run.Check{{
			ID:       "behavior",
			Tool:     "sorna",
			Path:     "sorna.json",
			Required: true,
			Status:   "passed",
			Result: &run.Result{
				Ref: ciresult.FileRef{Path: "sorna.json", SHA256: strings.Repeat("b", 64)},
				Artifact: ciresult.Artifact{
					Schema:      ciresult.Schema,
					Tool:        "sorna",
					Kind:        "test",
					Status:      "passed",
					ExitCode:    0,
					CreatedAt:   "2026-09-15T12:00:00Z",
					Source:      ciresult.Source{Root: "."},
					Report:      []byte(`{"producer report":true}`),
					Explanation: []byte(`{"explanation":"ok"}`),
				},
			},
		}},
	}
}
