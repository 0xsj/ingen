package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"ingen/core/ciresult"
	nublardelivery "ingen/nublar/internal/delivery"
	nublarrun "ingen/nublar/internal/run"
	nublarstore "ingen/nublar/internal/storage/filesystem"
)

func TestWorkflowValidateCommandAcceptsDeclaration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workflow.yaml")
	contents := []byte(`schema: ingen.nublar-workflow/v1
id: cli-workflow
checks:
  - id: behavior
    tool: sorna
    result: sorna.json
`)
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}
	if exitCode := run([]string{"workflow", "validate", path}); exitCode != 0 {
		t.Fatalf("workflow validate = %d, want success", exitCode)
	}
}

func TestWorkflowValidateCommandRejectsInvalidDeclaration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workflow.yaml")
	contents := []byte(`schema: ingen.nublar-workflow/v1
id: cli-invalid-workflow
checks: []
`)
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}
	if exitCode := run([]string{"workflow", "validate", path}); exitCode != 1 {
		t.Fatalf("workflow validate = %d, want validation failure", exitCode)
	}
}

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

func TestRunCollectPersistsFailedRunBeforeReturningFailure(t *testing.T) {
	workspace := t.TempDir()
	artifactRoot := filepath.Join(workspace, "artifacts")
	storeRoot := filepath.Join(workspace, "runs")
	workflowPath := filepath.Join(workspace, "workflow.yaml")
	outputPath := filepath.Join(workspace, "failed-run.json")
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	workflowContents := []byte(`schema: ingen.nublar-workflow/v1
id: cli-failed-workflow
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
		Status:      "failed",
		ExitCode:    1,
		CreatedAt:   "2026-09-15T12:00:00Z",
		Source:      ciresult.Source{Root: "."},
		Report:      []byte(`{"ok":false}`),
		Explanation: []byte(`{"reason":"assertion failed"}`),
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
		"--run-id", "cli-failed-collect-01",
		"--store", storeRoot,
		"--output", outputPath,
	}
	if exitCode := run(args); exitCode != 1 {
		t.Fatalf("run(%v) = %d, want producer failure exit code 1", args, exitCode)
	}
	stored, err := loadStoredRun(storeRoot, "cli-failed-collect-01")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != "failed" || stored.ExitCode != 1 || stored.Checks[0].Status != "failed" {
		t.Fatalf("stored failed run = %+v, want failed decision and check", stored)
	}
	if _, err := nublarrun.LoadFile(outputPath); err != nil {
		t.Fatalf("failed run output was not written: %v", err)
	}
}

func TestRunCollectPersistsCollectionErrorBeforeReturningError(t *testing.T) {
	workspace := t.TempDir()
	artifactRoot := filepath.Join(workspace, "artifacts")
	storeRoot := filepath.Join(workspace, "runs")
	workflowPath := filepath.Join(workspace, "workflow.yaml")
	outputPath := filepath.Join(workspace, "error-run.json")
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	workflowContents := []byte(`schema: ingen.nublar-workflow/v1
id: cli-error-workflow
checks:
  - id: required-behavior
    tool: sorna
    result: missing.json
`)
	if err := os.WriteFile(workflowPath, workflowContents, 0o644); err != nil {
		t.Fatal(err)
	}

	args := []string{
		"run", "collect",
		"--workflow", workflowPath,
		"--root", artifactRoot,
		"--run-id", "cli-error-collect-01",
		"--store", storeRoot,
		"--output", outputPath,
	}
	if exitCode := run(args); exitCode != 2 {
		t.Fatalf("run(%v) = %d, want collection error exit code 2", args, exitCode)
	}
	stored, err := loadStoredRun(storeRoot, "cli-error-collect-01")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != "error" || stored.ExitCode != 2 || len(stored.Errors) != 1 {
		t.Fatalf("stored collection error = %+v, want error decision and one error", stored)
	}
	if _, err := nublarrun.LoadFile(outputPath); err != nil {
		t.Fatalf("collection error output was not written: %v", err)
	}
}

func TestRunCollectStoresOptionalExternalCorrelation(t *testing.T) {
	workspace := t.TempDir()
	artifactRoot := filepath.Join(workspace, "artifacts")
	storeRoot := filepath.Join(workspace, "runs")
	workflowPath := filepath.Join(workspace, "workflow.yaml")
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workflowPath, []byte(`schema: ingen.nublar-workflow/v1
id: cli-correlated-workflow
checks:
  - id: behavior
    tool: sorna
    result: sorna.json
`), 0o644); err != nil {
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
	contents, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifactRoot, "sorna.json"), contents, 0o644); err != nil {
		t.Fatal(err)
	}

	args := []string{
		"run", "collect",
		"--workflow", workflowPath,
		"--root", artifactRoot,
		"--run-id", "cli-correlated-01",
		"--store", storeRoot,
		"--external-system", "github-actions",
		"--external-id", "build-42",
		"--attempt", "3",
	}
	if exitCode := run(args); exitCode != 0 {
		t.Fatalf("run(%v) = %d, want correlated collection success", args, exitCode)
	}
	stored, err := loadStoredRun(storeRoot, "cli-correlated-01")
	if err != nil {
		t.Fatal(err)
	}
	want := &nublarrun.Correlation{System: "github-actions", ID: "build-42", Attempt: 3}
	if stored.Correlation == nil || *stored.Correlation != *want {
		t.Fatalf("stored correlation = %+v, want %+v", stored.Correlation, want)
	}
}

func TestParseCorrelationRequiresCompleteTuple(t *testing.T) {
	for name, values := range map[string]struct {
		system  string
		id      string
		attempt int64
	}{
		"missing system":  {id: "build-42", attempt: 1},
		"missing id":      {system: "github-actions", attempt: 1},
		"missing attempt": {system: "github-actions", id: "build-42"},
	} {
		t.Run(name, func(t *testing.T) {
			if correlation, err := parseCorrelation(values.system, values.id, values.attempt); err == nil || correlation != nil {
				t.Fatalf("parseCorrelation() = %+v, %v, want validation error", correlation, err)
			}
		})
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

func TestRunListFiltersByStatusAndWorkflow(t *testing.T) {
	storeRoot := filepath.Join(t.TempDir(), "runs")
	store, err := nublarstore.New(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	passed := cliTestRun("cli-filter-passed-01")
	passed.Workflow.ID = "document-pipeline-ci"
	failed := cliTestRun("cli-filter-failed-01")
	failed.Workflow.ID = "webhook-validation-ci"
	failed.Status = "failed"
	failed.ExitCode = 1
	failed.Checks[0].Status = "failed"
	failed.Checks[0].Result.Artifact.Status = "failed"
	failed.Checks[0].Result.Artifact.ExitCode = 1
	if err := store.Save(passed); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(failed); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(t.TempDir(), "filtered.json")
	args := []string{
		"run", "list",
		"--store", storeRoot,
		"--status", "failed",
		"--workflow", "webhook-validation-ci",
		"--output", output,
	}
	if exitCode := run(args); exitCode != 0 {
		t.Fatalf("run(%v) = %d, want filtered-list success", args, exitCode)
	}
	contents, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var records []nublarrun.Run
	if err := json.Unmarshal(contents, &records); err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].RunID != failed.RunID {
		t.Fatalf("filtered records = %+v, want only failed webhook run", records)
	}
}

func TestRunListFiltersByExternalCorrelation(t *testing.T) {
	storeRoot := filepath.Join(t.TempDir(), "runs")
	store, err := nublarstore.New(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	firstAttempt := cliTestRun("cli-external-build-01")
	firstAttempt.Correlation = &nublarrun.Correlation{System: "github-actions", ID: "build-42", Attempt: 1}
	secondAttempt := cliTestRun("cli-external-build-02")
	secondAttempt.Correlation = &nublarrun.Correlation{System: "github-actions", ID: "build-42", Attempt: 2}
	otherSystem := cliTestRun("cli-external-build-03")
	otherSystem.Correlation = &nublarrun.Correlation{System: "circleci", ID: "build-42", Attempt: 2}
	for _, record := range []nublarrun.Run{firstAttempt, secondAttempt, otherSystem} {
		if err := store.Save(record); err != nil {
			t.Fatal(err)
		}
	}

	output := filepath.Join(t.TempDir(), "filtered.json")
	args := []string{
		"run", "list",
		"--store", storeRoot,
		"--external-system", "github-actions",
		"--external-id", "build-42",
		"--attempt", "2",
		"--output", output,
	}
	if exitCode := run(args); exitCode != 0 {
		t.Fatalf("run(%v) = %d, want external-correlation filter success", args, exitCode)
	}
	contents, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var records []nublarrun.Run
	if err := json.Unmarshal(contents, &records); err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].RunID != secondAttempt.RunID {
		t.Fatalf("filtered records = %+v, want only second GitHub attempt", records)
	}
}

func TestRunListRejectsUnsupportedStatusFilter(t *testing.T) {
	storeRoot := filepath.Join(t.TempDir(), "runs")
	args := []string{"run", "list", "--store", storeRoot, "--status", "blocked"}
	if exitCode := run(args); exitCode != 2 {
		t.Fatalf("run(%v) = %d, want unsupported-status usage error", args, exitCode)
	}
}

func TestRunListRejectsNegativeAttemptFilter(t *testing.T) {
	storeRoot := filepath.Join(t.TempDir(), "runs")
	args := []string{"run", "list", "--store", storeRoot, "--attempt=-1"}
	if exitCode := run(args); exitCode != 2 {
		t.Fatalf("run(%v) = %d, want invalid-attempt usage error", args, exitCode)
	}
}

func TestRunShowReturnsStoredFailureCodeAndExportsRecord(t *testing.T) {
	storeRoot := filepath.Join(t.TempDir(), "runs")
	store, err := nublarstore.New(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	record := cliTestRun("cli-show-failed-01")
	record.Status = "failed"
	record.ExitCode = 1
	record.Checks[0].Status = "failed"
	record.Checks[0].Result.Artifact.Status = "failed"
	record.Checks[0].Result.Artifact.ExitCode = 1
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "shown-failed.json")
	args := []string{"run", "show", "--store", storeRoot, "--run-id", record.RunID, "--output", output}
	if exitCode := run(args); exitCode != 1 {
		t.Fatalf("run(%v) = %d, want stored failure code", args, exitCode)
	}
	shown, err := nublarrun.LoadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if shown.RunID != record.RunID || shown.Status != "failed" || shown.ExitCode != 1 {
		t.Fatalf("shown run = %+v, want failed stored record", shown)
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

func TestRunDecisionExportsFailedStatusWithReadSuccess(t *testing.T) {
	storeRoot := filepath.Join(t.TempDir(), "runs")
	store, err := nublarstore.New(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	record := cliTestRun("cli-decision-failed-01")
	record.Status = "failed"
	record.ExitCode = 1
	record.Checks[0].Status = "failed"
	record.Checks[0].Result.Artifact.Status = "failed"
	record.Checks[0].Result.Artifact.ExitCode = 1
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "decision-failed.json")
	args := []string{"run", "decision", "--store", storeRoot, "--run-id", record.RunID, "--output", output}
	if exitCode := run(args); exitCode != 0 {
		t.Fatalf("run(%v) = %d, want export success despite failed run", args, exitCode)
	}
	contents, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var decision nublardelivery.Decision
	if err := json.Unmarshal(contents, &decision); err != nil {
		t.Fatal(err)
	}
	if decision.RunID != record.RunID || decision.Status != "failed" || decision.ExitCode != 1 {
		t.Fatalf("decision = %+v, want failed provider-neutral projection", decision)
	}
}

func TestRunDeliverRejectsInvalidWebhookConfiguration(t *testing.T) {
	storeRoot := filepath.Join(t.TempDir(), "runs")
	store, err := nublarstore.New(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(cliTestRun("cli-deliver-01")); err != nil {
		t.Fatal(err)
	}
	args := []string{"run", "deliver", "--store", storeRoot, "--run-id", "cli-deliver-01", "--webhook", "file:///tmp/nublar", "--timeout", "1s"}
	if exitCode := run(args); exitCode != 2 {
		t.Fatalf("run(%v) = %d, want invalid-webhook configuration error", args, exitCode)
	}
}

func TestRunDeliverWritesAcceptedReceiptFromCLI(t *testing.T) {
	storeRoot := filepath.Join(t.TempDir(), "runs")
	store, err := nublarstore.New(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(cliTestRun("cli-deliver-accepted-01")); err != nil {
		t.Fatal(err)
	}
	var received nublardelivery.Decision
	publisher := publisherFunc(func(_ context.Context, decision nublardelivery.Decision) (nublardelivery.Receipt, error) {
		received = decision
		return nublardelivery.Receipt{
			Schema:      nublardelivery.ReceiptSchema,
			RunID:       decision.RunID,
			Transport:   "http-webhook",
			Status:      "accepted",
			HTTPStatus:  http.StatusNoContent,
			AttemptedAt: "2026-09-15T12:00:02Z",
		}, nil
	})

	receiptPath := filepath.Join(t.TempDir(), "accepted-receipt.json")
	args := []string{
		"run", "deliver",
		"--store", storeRoot,
		"--run-id", "cli-deliver-accepted-01",
		"--webhook", "https://example.test/nublar",
		"--timeout", "2s",
		"--receipt", receiptPath,
	}
	if exitCode := deliverCommandWithFactory(args[2:], func(_ string, _ []byte) (nublardelivery.Publisher, error) {
		return publisher, nil
	}); exitCode != 0 {
		t.Fatalf("run(%v) = %d, want accepted delivery", args, exitCode)
	}
	if received.RunID != "cli-deliver-accepted-01" || received.Status != "passed" {
		t.Fatalf("received decision = %+v, want passed decision for run", received)
	}
	contents, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	var receipt nublardelivery.Receipt
	if err := json.Unmarshal(contents, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "accepted" || receipt.HTTPStatus != http.StatusNoContent || receipt.RunID != "cli-deliver-accepted-01" {
		t.Fatalf("receipt = %+v, want accepted 204 receipt", receipt)
	}
}

func TestRunDeliverWritesFailedReceiptFromCLI(t *testing.T) {
	storeRoot := filepath.Join(t.TempDir(), "runs")
	store, err := nublarstore.New(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(cliTestRun("cli-deliver-failed-01")); err != nil {
		t.Fatal(err)
	}
	publisher := publisherFunc(func(_ context.Context, decision nublardelivery.Decision) (nublardelivery.Receipt, error) {
		return nublardelivery.Receipt{
			Schema:      nublardelivery.ReceiptSchema,
			RunID:       decision.RunID,
			Transport:   "http-webhook",
			Status:      "failed",
			HTTPStatus:  http.StatusBadGateway,
			AttemptedAt: "2026-09-15T12:00:02Z",
			Error:       "Nublar webhook returned HTTP 502 Bad Gateway",
		}, errors.New("Nublar webhook returned HTTP 502 Bad Gateway")
	})

	receiptPath := filepath.Join(t.TempDir(), "failed-receipt.json")
	args := []string{
		"run", "deliver",
		"--store", storeRoot,
		"--run-id", "cli-deliver-failed-01",
		"--webhook", "https://example.test/nublar",
		"--timeout", "2s",
		"--receipt", receiptPath,
	}
	if exitCode := deliverCommandWithFactory(args[2:], func(_ string, _ []byte) (nublardelivery.Publisher, error) {
		return publisher, nil
	}); exitCode != 2 {
		t.Fatalf("run(%v) = %d, want delivery failure", args, exitCode)
	}
	contents, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	var receipt nublardelivery.Receipt
	if err := json.Unmarshal(contents, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "failed" || receipt.HTTPStatus != http.StatusBadGateway || receipt.Error == "" || receipt.RunID != "cli-deliver-failed-01" {
		t.Fatalf("receipt = %+v, want failed 502 receipt", receipt)
	}
	stored, err := store.Load("cli-deliver-failed-01")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != "passed" || stored.ExitCode != 0 {
		t.Fatalf("stored run after failed delivery = %+v, want unchanged passed decision", stored)
	}
}

func TestSaveReceiptWritesVersionedReceipt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery-receipt.json")
	receipt := nublardelivery.Receipt{
		Schema:      nublardelivery.ReceiptSchema,
		RunID:       "cli-receipt-01",
		Transport:   "http-webhook",
		Status:      "accepted",
		HTTPStatus:  204,
		AttemptedAt: "2026-09-15T12:00:02Z",
	}
	if err := saveReceipt(path, receipt); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var loaded nublardelivery.Receipt
	if err := json.Unmarshal(contents, &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded != receipt {
		t.Fatalf("loaded receipt = %+v, want %+v", loaded, receipt)
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

func loadStoredRun(storeRoot, runID string) (nublarrun.Run, error) {
	store, err := nublarstore.New(storeRoot)
	if err != nil {
		return nublarrun.Run{}, err
	}
	return store.Load(runID)
}

type publisherFunc func(context.Context, nublardelivery.Decision) (nublardelivery.Receipt, error)

func (f publisherFunc) Publish(ctx context.Context, decision nublardelivery.Decision) (nublardelivery.Receipt, error) {
	return f(ctx, decision)
}
