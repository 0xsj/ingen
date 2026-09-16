package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/sattler"
)

func TestBundleCompareCommandReportsJSONManifestErrors(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "comparison.json")
	if err := os.WriteFile(manifestPath, []byte(`{"schema":"ingen.sattler-comparison-input/v0","before":{"ci_result":"before.json"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	stderr := captureStderr(t, func() int {
		return bundleCompareCommand([]string{"compare", "--format", "json", manifestPath})
	})
	var document sattler.ErrorDocument
	if err := json.Unmarshal([]byte(stderr), &document); err != nil {
		t.Fatalf("stderr = %q, decode error = %v", stderr, err)
	}
	if document.Schema != sattler.ErrorSchema || document.Operation != "bundle compare" {
		t.Fatalf("error document = %+v, want Sattler error envelope", document)
	}
	if len(document.Errors) != 1 || document.Errors[0].Code != "incomplete-pair" {
		t.Fatalf("error issues = %+v, want incomplete-pair", document.Errors)
	}
}

func TestBundleCompareCommandSummaryOnly(t *testing.T) {
	root := t.TempDir()
	workflowHash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	run := `{"schema":"ingen.nublar-run/v1","run_id":"run-1","workflow":{"id":"workflow","file":{"path":"workflow.yaml","sha256":"` + workflowHash + `"}},"status":"passed","exit_code":0,"checks":[{"id":"check","tool":"sorna","required":true,"status":"passed"}]}`
	if err := os.WriteFile(filepath.Join(root, "before.json"), []byte(run), 0o644); err != nil {
		t.Fatal(err)
	}
	afterRun := strings.Replace(run, `"status":"passed"`, `"status":"failed"`, 1)
	if err := os.WriteFile(filepath.Join(root, "after.json"), []byte(afterRun), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema":"ingen.sattler-comparison-input/v0","before":{"nublar_run":"before.json"},"after":{"nublar_run":"after.json"}}`
	manifestPath := filepath.Join(root, "comparison.json")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout := captureStdout(t, func() int {
		return bundleCompareCommand([]string{"compare", "--summary-only", "--format", "json", "--change-id", "verdict.status", manifestPath})
	})
	var document sattler.BundleSummaryReport
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("stdout = %q, decode error = %v", stdout, err)
	}
	if document.Schema != "ingen.sattler-bundle-summary/v0" || document.Summary.ChangeSummary.Total != 1 || len(document.ChangeIDFilter) != 1 || document.ChangeIDFilter[0] != "verdict.status" {
		t.Fatalf("summary document = %+v, want filtered summary-only output", document)
	}
}

func TestSeriesCompareCommandSummaryOnly(t *testing.T) {
	root := t.TempDir()
	bundleRoot := filepath.Join(root, "bundle")
	if err := os.MkdirAll(bundleRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	workflowHash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	run := `{"schema":"ingen.nublar-run/v1","run_id":"run-1","workflow":{"id":"workflow","file":{"path":"workflow.yaml","sha256":"` + workflowHash + `"}},"status":"passed","exit_code":0,"checks":[{"id":"check","tool":"sorna","required":true,"status":"passed"}]}`
	if err := os.WriteFile(filepath.Join(bundleRoot, "before.json"), []byte(run), 0o644); err != nil {
		t.Fatal(err)
	}
	afterRun := strings.Replace(run, `"status":"passed"`, `"status":"failed"`, 1)
	if err := os.WriteFile(filepath.Join(bundleRoot, "after.json"), []byte(afterRun), 0o644); err != nil {
		t.Fatal(err)
	}
	comparison := `{"schema":"ingen.sattler-comparison-input/v0","before":{"nublar_run":"before.json"},"after":{"nublar_run":"after.json"}}`
	comparisonPath := filepath.Join(bundleRoot, "comparison.json")
	if err := os.WriteFile(comparisonPath, []byte(comparison), 0o644); err != nil {
		t.Fatal(err)
	}
	series := `{"schema":"ingen.sattler-comparison-series-input/v0","entries":[{"id":"one","manifest":"bundle/comparison.json"}]}`
	seriesPath := filepath.Join(root, "series.json")
	if err := os.WriteFile(seriesPath, []byte(series), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout := captureStdout(t, func() int {
		return seriesCompareCommand([]string{"compare", "--summary-only", "--format", "json", seriesPath})
	})
	var document sattler.SeriesSummaryReport
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("stdout = %q, decode error = %v", stdout, err)
	}
	if document.Schema != "ingen.sattler-comparison-series-summary/v0" || document.Summary.Entries != 1 || strings.Contains(stdout, `"entries": [`) {
		t.Fatalf("summary document = %+v, want one point-free series summary", document)
	}
}

func captureStderr(t *testing.T, run func() int) string {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stderr
	os.Stderr = write
	code := run()
	if closeErr := write.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	os.Stderr = previous
	contents, readErr := io.ReadAll(read)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if closeErr := read.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if code != 1 {
		t.Fatalf("command exit code = %d, want 1", code)
	}
	return string(contents)
}

func captureStdout(t *testing.T, run func() int) string {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = write
	code := run()
	if closeErr := write.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	os.Stdout = previous
	contents, readErr := io.ReadAll(read)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if closeErr := read.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if code != 0 {
		t.Fatalf("command exit code = %d, want 0", code)
	}
	return string(contents)
}
