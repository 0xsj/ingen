package integration

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"ingen/core/ciresult"
	nublarrun "ingen/nublar/internal/run"
	nublarstore "ingen/nublar/internal/storage/filesystem"
)

func TestMixedProducerWorkflowComposesSharedStatuses(t *testing.T) {
	workflowPath := filepath.Join("..", "testdata", "workflows", "mixed-producers.yaml")
	artifactRoot := filepath.Join("..", "testdata", "ci-results")
	record, err := nublarrun.CollectWorkflowFile(workflowPath, artifactRoot, "mixed-run-01")
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != "failed" || record.ExitCode != 1 || len(record.Checks) != 2 {
		t.Fatalf("mixed-producer run = %+v, want failed run with two checks", record)
	}
	if record.Checks[0].Tool != "sorna" || record.Checks[0].Status != "passed" {
		t.Fatalf("Sorna check = %+v, want passed Sorna result", record.Checks[0])
	}
	if record.Checks[1].Tool != "paddock" || record.Checks[1].Status != "failed" {
		t.Fatalf("Paddock check = %+v, want failed Paddock result", record.Checks[1])
	}
	if compactJSON(record.Checks[0].Result.Artifact.Report) != `{"schema":"test.sorna-report/v1","cases":7}` {
		t.Fatalf("Sorna report = %s, want opaque producer report", record.Checks[0].Result.Artifact.Report)
	}
	if compactJSON(record.Checks[1].Result.Artifact.Report) != `{"schema":"test.paddock-report/v1","violations":[{"rule":"example-boundary","from":"app.domain","to":"app.adapters"}]}` {
		t.Fatalf("Paddock report = %s, want opaque producer report", record.Checks[1].Result.Artifact.Report)
	}
	if len(record.Checks[0].Result.Ref.SHA256) != 64 || len(record.Checks[1].Result.Ref.SHA256) != 64 {
		t.Fatalf("result hashes = %q, %q, want SHA-256 references", record.Checks[0].Result.Ref.SHA256, record.Checks[1].Result.Ref.SHA256)
	}
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}

	store, err := nublarstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(record.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != "failed" || loaded.Checks[1].Result.Artifact.Tool != "paddock" {
		t.Fatalf("stored mixed-producer run = %+v, want preserved Paddock result", loaded)
	}
}

func TestSentinelFailedEnvelopeRemainsFailedAtNublarBoundary(t *testing.T) {
	workspace := t.TempDir()
	artifactRoot := filepath.Join(workspace, "artifacts")
	workflowPath := filepath.Join(workspace, "workflow.yaml")
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	workflowContents := []byte(`schema: ingen.nublar-workflow/v1
id: sentinel-webhook-verifier
checks:
  - id: verifier-lifecycle
    tool: sentinel
    result: sentinel-webhook-ci-result.json
`)
	if err := os.WriteFile(workflowPath, workflowContents, 0o644); err != nil {
		t.Fatal(err)
	}
	result := ciresult.Artifact{
		Schema:    ciresult.Schema,
		Tool:      "sentinel",
		Kind:      "orchestration-receipt",
		Status:    "failed",
		ExitCode:  1,
		CreatedAt: "2026-09-16T10:00:00Z",
		Source:    ciresult.Source{Root: ".", ModulePath: "webhook-validation"},
	}
	result.Report = []byte(`{"schema":"ingen.sentinel-run/v1","status":"failed"}`)
	result.Explanation = []byte(`{"schema":"ingen.sentinel-ci-explanation/v1","receipt_status":"failed","outcome":"Sentinel lifecycle failed","artifact_ids":[]}`)
	resultContents, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifactRoot, "sentinel-webhook-ci-result.json"), resultContents, 0o644); err != nil {
		t.Fatal(err)
	}

	record, err := nublarrun.CollectWorkflowFile(workflowPath, artifactRoot, "sentinel-failed-boundary-01")
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != "failed" || record.ExitCode != 1 || len(record.Checks) != 1 || record.Checks[0].Status != "failed" {
		t.Fatalf("collected Sentinel run = %+v, want failed run", record)
	}
	if record.Checks[0].Result == nil || record.Checks[0].Result.Artifact.Tool != "sentinel" || string(record.Checks[0].Result.Artifact.Report) != string(result.Report) {
		t.Fatalf("Sentinel result = %+v, want opaque failed producer result", record.Checks[0].Result)
	}
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSentinelBlockedEnvelopeBecomesNublarError(t *testing.T) {
	workspace := t.TempDir()
	artifactRoot := filepath.Join(workspace, "artifacts")
	workflowPath := filepath.Join(workspace, "workflow.yaml")
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	workflowContents := []byte(`schema: ingen.nublar-workflow/v1
id: sentinel-webhook-verifier
checks:
  - id: verifier-lifecycle
    tool: sentinel
    result: sentinel-webhook-ci-result.json
`)
	if err := os.WriteFile(workflowPath, workflowContents, 0o644); err != nil {
		t.Fatal(err)
	}
	result := ciresult.Artifact{
		Schema:      ciresult.Schema,
		Tool:        "sentinel",
		Kind:        "orchestration-receipt",
		Status:      "error",
		ExitCode:    2,
		CreatedAt:   "2026-09-16T10:00:00Z",
		Source:      ciresult.Source{Root: ".", ModulePath: "webhook-validation"},
		Report:      []byte(`{"schema":"ingen.sentinel-run/v1","status":"blocked"}`),
		Explanation: []byte(`{"schema":"ingen.sentinel-ci-explanation/v1","receipt_status":"blocked","outcome":"Sentinel lifecycle status \"blocked\" is not a terminal verifier outcome","artifact_ids":[]}`),
		Error:       "Sentinel receipt is not complete: status blocked",
	}
	resultContents, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifactRoot, "sentinel-webhook-ci-result.json"), resultContents, 0o644); err != nil {
		t.Fatal(err)
	}

	record, err := nublarrun.CollectWorkflowFile(workflowPath, artifactRoot, "sentinel-blocked-boundary-01")
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != "error" || record.ExitCode != 2 || len(record.Checks) != 1 || record.Checks[0].Status != "error" {
		t.Fatalf("collected blocked Sentinel run = %+v, want error run", record)
	}
	if record.Checks[0].Result == nil || record.Checks[0].Result.Artifact.Tool != "sentinel" || record.Checks[0].Result.Artifact.Error == "" {
		t.Fatalf("blocked Sentinel result = %+v, want preserved error envelope", record.Checks[0].Result)
	}
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}
}

func compactJSON(value []byte) string {
	var compact bytes.Buffer
	if err := json.Compact(&compact, value); err != nil {
		return string(value)
	}
	return compact.String()
}
