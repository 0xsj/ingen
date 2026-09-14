package aggregate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/core/ciresult"
	"ingen/nublar/internal/workflow"
)

func TestComposeUsesEnvelopeStatusAndPreservesProducerArtifacts(t *testing.T) {
	results := []Input{
		{Path: "sorna.json", Result: testResult("sorna", "passed", 0)},
		{Path: "paddock.json", Result: testResult("paddock", "failed", 1)},
	}
	report, err := Compose(results)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "failed" || report.ExitCode != 1 || len(report.Results) != 2 {
		t.Fatalf("report = %+v, want failed aggregate with two inputs", report)
	}
	if report.Results[0].Result.Tool != "sorna" || report.Results[1].Result.Tool != "paddock" {
		t.Fatalf("report results = %+v, want original producer artifacts", report.Results)
	}
	if string(report.Results[0].Result.Report) != `{"producer":"sorna"}` {
		t.Fatalf("Sorna report = %s, want opaque producer JSON", report.Results[0].Result.Report)
	}
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestComposeEscalatesErrorsWithoutInspectingReports(t *testing.T) {
	report, err := Compose([]Input{
		{Path: "failed.json", Result: testResult("tool-a", "failed", 1)},
		{Path: "error.json", Result: ciresult.Artifact{
			Schema:    ciresult.Schema,
			Tool:      "tool-b",
			Kind:      "test",
			Status:    "error",
			ExitCode:  2,
			CreatedAt: "2026-09-14T12:00:00Z",
			Source:    ciresult.Source{Root: "."},
			Error:     "unavailable",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "error" || report.ExitCode != 2 {
		t.Fatalf("report = %+v, want error aggregate", report)
	}
}

func TestComposeRejectsDuplicateInputs(t *testing.T) {
	result := testResult("sorna", "passed", 0)
	if _, err := Compose([]Input{{Path: "same.json", Result: result}, {Path: "same.json", Result: result}}); err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Fatalf("Compose() = %v, want duplicate input error", err)
	}
}

func TestComposeWorkflowLoadsRequiredAndOptionalChecks(t *testing.T) {
	root := t.TempDir()
	writeResult(t, root, "sorna.json", testResult("sorna", "passed", 0))
	optional := false
	document := workflow.Document{
		Schema: workflow.Schema,
		ID:     "document-pipeline-ci",
		Checks: []workflow.Check{
			{ID: "behavior", Tool: "sorna", Result: "sorna.json"},
			{ID: "architecture", Tool: "paddock", Result: "paddock.json", Required: &optional},
		},
	}
	report := ComposeWorkflow(document, root)
	if report.WorkflowID != document.ID || report.Status != "passed" || report.ExitCode != 0 || len(report.Results) != 1 || len(report.Warnings) != 1 || len(report.Errors) != 0 {
		t.Fatalf("workflow report = %+v, want passing result with optional warning", report)
	}
	if report.Results[0].CheckID != "behavior" || report.Warnings[0].CheckID != "architecture" {
		t.Fatalf("workflow report details = %+v, want check identities", report)
	}
	if len(report.Results[0].SHA256) != 64 {
		t.Fatalf("workflow result hash = %q, want SHA-256", report.Results[0].SHA256)
	}
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestComposeFilesRecordsConsumedArtifactHash(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "result.json")
	writeResult(t, root, "result.json", testResult("sorna", "passed", 0))
	report, err := ComposeFiles([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 1 || len(report.Results[0].SHA256) != 64 {
		t.Fatalf("report input = %+v, want consumed artifact hash", report.Results)
	}
	expected, err := hashFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if report.Results[0].SHA256 != expected {
		t.Fatalf("report hash = %q, want %q", report.Results[0].SHA256, expected)
	}
}

func TestComposeWorkflowMakesMissingRequiredResultsAnError(t *testing.T) {
	report := ComposeWorkflow(workflow.Document{
		Schema: workflow.Schema,
		Checks: []workflow.Check{{ID: "behavior", Tool: "sorna", Result: "missing.json"}},
	}, t.TempDir())
	if report.Status != "error" || report.ExitCode != 2 || len(report.Errors) != 1 || len(report.Results) != 0 {
		t.Fatalf("workflow report = %+v, want missing-required error", report)
	}
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestComposeWorkflowFileRecordsWorkflowBytes(t *testing.T) {
	root := t.TempDir()
	writeResult(t, root, "sorna.json", testResult("sorna", "passed", 0))
	workflowPath := filepath.Join(root, "workflow.yaml")
	workflowContents := []byte(`schema: ingen.nublar-workflow/v1
id: test-workflow
checks:
  - id: behavior
    tool: sorna
    result: sorna.json
`)
	if err := os.WriteFile(workflowPath, workflowContents, 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := ComposeWorkflowFile(workflowPath, root)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := workflow.FileReference(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	if report.WorkflowID != "test-workflow" || report.Workflow == nil || *report.Workflow != reference {
		t.Fatalf("workflow reference = %+v, want %v", report.Workflow, reference)
	}
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestComposeWorkflowRejectsUnexpectedProducer(t *testing.T) {
	root := t.TempDir()
	writeResult(t, root, "result.json", testResult("paddock", "passed", 0))
	report := ComposeWorkflow(workflow.Document{
		Schema: workflow.Schema,
		Checks: []workflow.Check{{ID: "behavior", Tool: "sorna", Result: "result.json"}},
	}, root)
	if report.Status != "error" || report.ExitCode != 2 || len(report.Errors) != 1 || !strings.Contains(report.Errors[0].Reason, "workflow expects") {
		t.Fatalf("workflow report = %+v, want producer mismatch error", report)
	}
}

func testResult(tool, status string, exitCode int) ciresult.Artifact {
	return ciresult.Artifact{
		Schema:      ciresult.Schema,
		Tool:        tool,
		Kind:        "test",
		Status:      status,
		ExitCode:    exitCode,
		CreatedAt:   "2026-09-14T12:00:00Z",
		Source:      ciresult.Source{Root: "."},
		Report:      []byte(`{"producer":"` + tool + `"}`),
		Explanation: []byte(`{"producer":"` + tool + `"}`),
	}
}

func writeResult(t *testing.T, root, name string, result ciresult.Artifact) {
	t.Helper()
	contents, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, name), contents, 0o644); err != nil {
		t.Fatal(err)
	}
}
