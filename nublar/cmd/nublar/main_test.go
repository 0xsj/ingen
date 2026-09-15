package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"ingen/core/ciresult"
	nublarrun "ingen/nublar/internal/run"
	nublarstore "ingen/nublar/internal/storage/filesystem"
)

func TestRunCollectStoresAndRunShowLoadsTheSameRecord(t *testing.T) {
	workspace := t.TempDir()
	artifactRoot := filepath.Join(workspace, "artifacts")
	storeRoot := filepath.Join(workspace, "runs")
	workflowPath := filepath.Join(workspace, "workflow.yaml")
	collectOutput := filepath.Join(workspace, "collected.json")
	showOutput := filepath.Join(workspace, "shown.json")
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	workflowContents := []byte(`schema: ingen.nublar-workflow/v1
id: cli-test-workflow
checks:
  - id: behavior
    tool: sorna
    result: sorna.json
`)
	if err := os.WriteFile(workflowPath, workflowContents, 0o644); err != nil {
		t.Fatal(err)
	}
	result := ciresult.Artifact{
		Schema:      ciresult.Schema,
		Tool:        "sorna",
		Kind:        "test",
		Status:      "passed",
		ExitCode:    0,
		CreatedAt:   "2026-09-15T12:00:00Z",
		Source:      ciresult.Source{Root: "."},
		Report:      []byte(`{"ok":true}`),
		Explanation: []byte(`{"ok":true}`),
	}
	resultContents, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifactRoot, "sorna.json"), resultContents, 0o644); err != nil {
		t.Fatal(err)
	}

	args := []string{
		"run", "collect",
		"--workflow", workflowPath,
		"--root", artifactRoot,
		"--run-id", "cli-run-01",
		"--store", storeRoot,
		"--output", collectOutput,
	}
	if exitCode := run(args); exitCode != 0 {
		t.Fatalf("run(%v) = %d, want 0", args, exitCode)
	}
	collected, err := nublarrun.LoadFile(collectOutput)
	if err != nil {
		t.Fatal(err)
	}
	store, err := nublarstore.New(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.Load("cli-run-01")
	if err != nil {
		t.Fatal(err)
	}
	if stored.RunID != collected.RunID || stored.Workflow.ID != collected.Workflow.ID {
		t.Fatalf("stored run = %+v, collected run = %+v", stored, collected)
	}

	showArgs := []string{"run", "show", "--store", storeRoot, "--run-id", "cli-run-01", "--output", showOutput}
	if exitCode := run(showArgs); exitCode != 0 {
		t.Fatalf("run(%v) = %d, want 0", showArgs, exitCode)
	}
	shown, err := nublarrun.LoadFile(showOutput)
	if err != nil {
		t.Fatal(err)
	}
	if shown.RunID != collected.RunID || shown.Checks[0].Result.Artifact.Tool != "sorna" {
		t.Fatalf("shown run = %+v, want collected run", shown)
	}
}

func TestRunListSucceedsForFailedStoredRun(t *testing.T) {
	storeRoot := filepath.Join(t.TempDir(), "runs")
	store, err := nublarstore.New(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	record := cliTestRun("cli-failed-01")
	record.Status = "failed"
	record.ExitCode = 1
	record.Checks[0].Status = "failed"
	record.Checks[0].Result.Artifact.Status = "failed"
	record.Checks[0].Result.Artifact.ExitCode = 1
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "list.json")
	args := []string{"run", "list", "--store", storeRoot, "--output", output}
	if exitCode := run(args); exitCode != 0 {
		t.Fatalf("run(%v) = %d, want read success", args, exitCode)
	}
	contents, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var records []nublarrun.Run
	if err := json.Unmarshal(contents, &records); err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].RunID != record.RunID || records[0].Status != "failed" {
		t.Fatalf("listed records = %+v, want stored failed run", records)
	}
}

func TestRunDecisionExportsCompactVersionedProjection(t *testing.T) {
	storeRoot := filepath.Join(t.TempDir(), "runs")
	store, err := nublarstore.New(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(cliTestRun("cli-decision-01")); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "decision.json")
	args := []string{"run", "decision", "--store", storeRoot, "--run-id", "cli-decision-01", "--output", output}
	if exitCode := run(args); exitCode != 0 {
		t.Fatalf("run(%v) = %d, want projection success", args, exitCode)
	}
	contents, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var decision map[string]json.RawMessage
	if err := json.Unmarshal(contents, &decision); err != nil {
		t.Fatal(err)
	}
	var schema string
	if err := json.Unmarshal(decision["schema"], &schema); err != nil {
		t.Fatal(err)
	}
	if schema != "ingen.nublar-decision/v1" {
		t.Fatalf("decision schema = %q, want versioned projection", schema)
	}
	if _, ok := decision["artifact"]; ok || string(contents) == "" {
		t.Fatalf("decision = %s, want compact projection without producer artifact", contents)
	}
}

func cliTestRun(runID string) nublarrun.Run {
	return nublarrun.Run{
		Schema:      nublarrun.Schema,
		RunID:       runID,
		Workflow:    nublarrun.Workflow{ID: "cli-test-workflow", File: ciresult.FileRef{Path: "workflow.yaml", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
		Status:      "passed",
		ExitCode:    0,
		CreatedAt:   "2026-09-15T12:00:00Z",
		CompletedAt: "2026-09-15T12:00:01Z",
		Checks: []nublarrun.Check{{
			ID:       "behavior",
			Tool:     "sorna",
			Path:     "sorna.json",
			Required: true,
			Status:   "passed",
			Result: &nublarrun.Result{
				Ref: ciresult.FileRef{Path: "sorna.json", SHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
				Artifact: ciresult.Artifact{
					Schema:      ciresult.Schema,
					Tool:        "sorna",
					Kind:        "test",
					Status:      "passed",
					ExitCode:    0,
					CreatedAt:   "2026-09-15T12:00:00Z",
					Source:      ciresult.Source{Root: "."},
					Report:      []byte(`{"ok":true}`),
					Explanation: []byte(`{"ok":true}`),
				},
			},
		}},
	}
}
