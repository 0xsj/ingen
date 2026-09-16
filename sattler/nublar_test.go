package sattler

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareNublarRunsReportsWorkflowAndCheckChanges(t *testing.T) {
	beforePath := writeNublarFixture(t, `{
        "schema":"ingen.nublar-run/v1",
        "run_id":"run-before",
        "workflow":{"id":"document-pipeline","file":{"path":"workflow.yaml","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
        "status":"passed",
        "exit_code":0,
        "checks":[
            {"id":"campaign","tool":"sorna","required":true,"status":"passed"},
            {"id":"provider","tool":"sorna","required":true,"status":"passed"}
        ]
    }`)
	afterPath := writeNublarFixture(t, `{
        "schema":"ingen.nublar-run/v1",
        "run_id":"run-after",
        "workflow":{"id":"document-pipeline","file":{"path":"workflow.yaml","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
        "status":"failed",
        "exit_code":1,
        "checks":[
            {"id":"campaign","tool":"sorna","required":true,"status":"failed","reason":"surviving mutation"},
            {"id":"provider","tool":"sorna","required":true,"status":"passed"},
            {"id":"sentinel","tool":"sentinel","required":false,"status":"missing","reason":"not configured"}
        ]
    }`)

	report, err := CompareNublarRunFiles(beforePath, afterPath)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Compatible {
		t.Fatal("runs with the same workflow identity were marked incompatible")
	}
	if got, want := len(report.Changes), 4; got != want {
		t.Fatalf("change count = %d, want %d: %+v", got, want, report.Changes)
	}
	assertChange(t, report.Changes[0], "verdict", "status")
	assertChange(t, report.Changes[1], "verdict", "exit_code")
	assertChange(t, report.Changes[2], "check", "checks.campaign")
	assertChange(t, report.Changes[3], "check", "checks.sentinel")
	if _, ok := report.After.Checks["sentinel"]; !ok {
		t.Fatal("after summary omitted newly introduced check")
	}
}

func TestCompareNublarRunsMarksWorkflowDrift(t *testing.T) {
	beforePath := writeNublarFixture(t, nublarFixture("run-before", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
	afterPath := writeNublarFixture(t, nublarFixture("run-after", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"))

	report, err := CompareNublarRunFiles(beforePath, afterPath)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Compatible {
		t.Fatal("runs with the same workflow ID were marked incompatible due to workflow file drift")
	}
	if len(report.Changes) != 1 || report.Changes[0].Field != "workflow.file" {
		t.Fatalf("workflow changes = %+v, want only workflow.file", report.Changes)
	}
}

func TestCompareNublarRunsExplainsWorkflowIdentityChange(t *testing.T) {
	beforePath := writeNublarFixture(t, nublarFixtureWithWorkflow("run-before", "document-pipeline", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
	afterPath := writeNublarFixture(t, nublarFixtureWithWorkflow("run-after", "webhook-validation", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))

	report, err := CompareNublarRunFiles(beforePath, afterPath)
	if err != nil {
		t.Fatal(err)
	}
	if report.Compatible {
		t.Fatal("runs with different workflow IDs were marked compatible")
	}
	if len(report.CompatibilityReasons) != 1 || !strings.Contains(report.CompatibilityReasons[0], "workflow id changed") {
		t.Fatalf("compatibility reasons = %+v, want workflow-ID explanation", report.CompatibilityReasons)
	}
}

func TestCompareNublarRunsRejectsWrongSchema(t *testing.T) {
	path := writeNublarFixture(t, `{"schema":"ingen.ci-result/v1"}`)
	if _, err := CompareNublarRunFiles(path, path); err == nil || !strings.Contains(err.Error(), "Nublar run schema") {
		t.Fatalf("error = %v, want schema error", err)
	}
}

func TestWriteNublarTextNamesCoordinatorBoundary(t *testing.T) {
	path := writeNublarFixture(t, nublarFixture("run", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
	report, err := CompareNublarRunFiles(path, path)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := WriteNublarText(&output, report); err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"Sattler Nublar run comparison", "compatible: true", "none observable at the Nublar boundary"} {
		if !strings.Contains(output.String(), fragment) {
			t.Fatalf("text output = %q, missing %q", output.String(), fragment)
		}
	}
}

func writeNublarFixture(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "run.json")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func nublarFixture(runID, workflowHash string) string {
	return nublarFixtureWithWorkflow(runID, "document-pipeline", workflowHash)
}

func nublarFixtureWithWorkflow(runID, workflowID, workflowHash string) string {
	return `{"schema":"ingen.nublar-run/v1","run_id":"` + runID + `","workflow":{"id":"` + workflowID + `","file":{"path":"workflow.yaml","sha256":"` + workflowHash + `"}},"status":"passed","exit_code":0,"created_at":"2026-09-16T10:00:00Z","completed_at":"2026-09-16T10:00:02Z","checks":[{"id":"campaign","tool":"sorna","required":true,"status":"passed"}]}`
}
