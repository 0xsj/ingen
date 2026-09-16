package sattler

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareSeriesManifestAggregatesOrderedBundles(t *testing.T) {
	root := t.TempDir()
	first := writeSeriesBundle(t, root, "first", "failed")
	second := writeSeriesBundle(t, root, "second", "failed")
	manifest := ComparisonSeriesManifest{
		Schema: ComparisonSeriesManifestSchema,
		Entries: []ComparisonSeriesEntry{
			{ID: "one", Label: "first attempt", Manifest: filepath.Base(filepath.Dir(first)) + "/comparison.json"},
			{ID: "two", Label: "second attempt", Manifest: filepath.Base(filepath.Dir(second)) + "/comparison.json"},
		},
	}
	seriesPath := filepath.Join(root, "series.json")
	writeSeriesFile(t, seriesPath, marshalJSON(t, manifest))

	series, err := CompareSeriesManifestFile(seriesPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(series.Entries) != 2 || series.Entries[0].ID != "one" || series.Entries[1].Label != "second attempt" {
		t.Fatalf("series entries = %+v, want ordered entries", series.Entries)
	}
	if series.Summary.Entries != 2 || series.Summary.Compatible != 2 || series.Summary.Incompatible != 0 {
		t.Fatalf("series summary = %+v, want two compatible entries", series.Summary)
	}
	if series.Summary.TotalChanges != 4 || series.Summary.ChangesByID["verdict.status"] != 2 || series.Summary.ChangesByID["verdict.exit_code"] != 2 {
		t.Fatalf("series change totals = %+v, want two status and exit changes", series.Summary)
	}

	var output bytes.Buffer
	if err := WriteSeriesText(&output, series); err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"Sattler comparison series", "changes by ID:", "verdict.status: 2", "first attempt", "second attempt"} {
		if !strings.Contains(output.String(), fragment) {
			t.Fatalf("series text = %q, missing %q", output.String(), fragment)
		}
	}
}

func TestComparisonSeriesManifestValidationReportsStableIssues(t *testing.T) {
	manifest := ComparisonSeriesManifest{
		Schema: ComparisonSeriesManifestSchema,
		Entries: []ComparisonSeriesEntry{
			{ID: "same", Manifest: "one.json"},
			{ID: "same"},
		},
	}
	var validationErr *ManifestValidationError
	if err := manifest.Validate(); !errors.As(err, &validationErr) {
		t.Fatalf("series validation error = %T %v, want ManifestValidationError", err, err)
	}
	if len(validationErr.Issues) != 2 {
		t.Fatalf("series validation issues = %+v, want duplicate and missing-manifest", validationErr.Issues)
	}
	if validationErr.Issues[0].Code != "duplicate-entry-id" || validationErr.Issues[1].Code != "missing-entry-manifest" {
		t.Fatalf("series validation codes = %+v, want stable codes", validationErr.Issues)
	}
}

func TestCompareSeriesManifestFiltersChangeIDsBeforeAggregation(t *testing.T) {
	root := t.TempDir()
	bundlePath := writeSeriesBundle(t, root, "filtered", "failed")
	manifest := ComparisonSeriesManifest{
		Schema: ComparisonSeriesManifestSchema,
		Entries: []ComparisonSeriesEntry{{
			ID:       "one",
			Manifest: filepath.Base(filepath.Dir(bundlePath)) + "/comparison.json",
		}},
	}
	seriesPath := filepath.Join(root, "series.json")
	writeSeriesFile(t, seriesPath, marshalJSON(t, manifest))

	series, err := CompareSeriesManifestFileWithChanges(seriesPath, []string{"verdict.status"})
	if err != nil {
		t.Fatal(err)
	}
	if len(series.ChangeIDFilter) != 1 || series.ChangeIDFilter[0] != "verdict.status" {
		t.Fatalf("series change ID filter = %+v, want selected ID", series.ChangeIDFilter)
	}
	if series.Summary.TotalChanges != 1 || series.Summary.ChangesByID["verdict.status"] != 1 || series.Summary.ChangesByID["verdict.exit_code"] != 0 {
		t.Fatalf("filtered series summary = %+v, want status-only aggregation", series.Summary)
	}
	if series.Entries[0].Summary.ChangeSummary.Total != 1 {
		t.Fatalf("filtered series point = %+v, want one change", series.Entries[0])
	}
}

func writeSeriesBundle(t *testing.T, root, directory, afterStatus string) string {
	t.Helper()
	directoryPath := filepath.Join(root, directory)
	if err := os.MkdirAll(directoryPath, 0o755); err != nil {
		t.Fatal(err)
	}
	hash := strings.Repeat("a", 64)
	before := nublarFixture("before-"+directory, hash)
	after := strings.Replace(nublarFixture("after-"+directory, hash), `"status":"passed"`, `"status":"`+afterStatus+`"`, 1)
	if afterStatus == "failed" {
		after = strings.Replace(after, `"exit_code":0`, `"exit_code":1`, 1)
	}
	writeSeriesFile(t, filepath.Join(directoryPath, "before.json"), []byte(before))
	writeSeriesFile(t, filepath.Join(directoryPath, "after.json"), []byte(after))
	manifest := ComparisonManifest{
		Schema: ComparisonManifestSchema,
		Before: ComparisonInputs{NublarRun: "before.json"},
		After:  ComparisonInputs{NublarRun: "after.json"},
	}
	manifestPath := filepath.Join(directoryPath, "comparison.json")
	writeSeriesFile(t, manifestPath, marshalJSON(t, manifest))
	return manifestPath
}

func writeSeriesFile(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}
}
