package sattler

import (
	"bytes"
	"encoding/json"
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
	if len(series.Entries) != 2 || series.Entries[0].ID != "one" || series.Entries[0].Latest || series.Entries[1].Label != "second attempt" || !series.Entries[1].Latest {
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
	for _, fragment := range []string{"Sattler comparison series", "changes by ID:", "verdict.status: 2", "transitions by subsystem:", "nublar_run: unchanged=0, changed=2, incompatible=0", "first attempt", "two (second attempt) [latest]"} {
		if !strings.Contains(output.String(), fragment) {
			t.Fatalf("series text = %q, missing %q", output.String(), fragment)
		}
	}
}

func TestCompareSeriesManifestAggregatesTransitionsBySubsystem(t *testing.T) {
	points := []SeriesPoint{
		{Summary: BundleSummary{Subsystems: map[string]BundleSubsystemSummary{
			"ci_result":  {Transition: NewStateTransition("status", "passed", "passed", true)},
			"nublar_run": {Transition: NewStateTransition("status", "passed", "failed", true)},
		}}},
		{Summary: BundleSummary{Subsystems: map[string]BundleSubsystemSummary{
			"ci_result":  {Transition: NewStateTransition("status", "passed", "failed", false)},
			"nublar_run": {Transition: NewStateTransition("status", "failed", "failed", true)},
		}}},
	}

	transitions := summarizeSeriesTransitions(points)
	if transitions["ci_result"].Unchanged != 1 || transitions["ci_result"].Changed != 0 || transitions["ci_result"].Incompatible != 1 {
		t.Fatalf("CI transitions = %+v, want unchanged=1 and incompatible=1", transitions["ci_result"])
	}
	if transitions["nublar_run"].Unchanged != 1 || transitions["nublar_run"].Changed != 1 || transitions["nublar_run"].Incompatible != 0 {
		t.Fatalf("Nublar transitions = %+v, want unchanged=1 and changed=1", transitions["nublar_run"])
	}
}

func TestCompareSeriesManifestAggregatesWarningsByMessage(t *testing.T) {
	points := []SeriesPoint{
		{Summary: BundleSummary{Warnings: []string{"report unavailable", "report unavailable"}}},
		{Summary: BundleSummary{Warnings: []string{"report unavailable", "another warning"}}},
	}

	warnings := summarizeSeriesWarnings(points)
	if warnings["report unavailable"] != 3 || warnings["another warning"] != 1 {
		t.Fatalf("warnings by message = %+v, want report=3 and another=1", warnings)
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

func TestCompareSeriesManifestAggregatesMutationChanges(t *testing.T) {
	root := t.TempDir()
	bundlePath := writeMutationSeriesBundle(t, root, "mutations")
	manifest := ComparisonSeriesManifest{
		Schema: ComparisonSeriesManifestSchema,
		Entries: []ComparisonSeriesEntry{{
			ID:       "campaign-1",
			Manifest: filepath.Base(filepath.Dir(bundlePath)) + "/comparison.json",
		}},
	}
	seriesPath := filepath.Join(root, "series.json")
	writeSeriesFile(t, seriesPath, marshalJSON(t, manifest))

	series, err := CompareSeriesManifestFile(seriesPath)
	if err != nil {
		t.Fatal(err)
	}
	if series.Summary.MutationChangesByID["alpha"] != 1 || series.Summary.MutationChangesByID["beta"] != 0 {
		t.Fatalf("mutation change frequencies = %+v, want alpha only", series.Summary.MutationChangesByID)
	}
	if len(series.Entries[0].MutationChangeIDs) != 1 || series.Entries[0].MutationChangeIDs[0] != "alpha" {
		t.Fatalf("mutation change IDs = %+v, want alpha", series.Entries[0].MutationChangeIDs)
	}

	var output bytes.Buffer
	if err := WriteSeriesText(&output, series); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "mutation changes by ID:") || !strings.Contains(output.String(), "alpha: 1") || !strings.Contains(output.String(), "mutation changes=alpha") {
		t.Fatalf("series text = %q, want mutation frequency and point detail", output.String())
	}
}

func TestWriteSeriesTextIncludesPointCorrelations(t *testing.T) {
	series := ComparisonSeries{
		Manifest: "series.json",
		Summary:  SeriesSummary{Entries: 1, Compatible: 1},
		Entries: []SeriesPoint{{
			ID:      "attempt-1",
			Summary: BundleSummary{ChangeSummary: ChangeSummary{}},
			Correlations: []BundleCorrelation{{
				Kind:               BundleCorrelationKindNublarCustodySource,
				Side:               "after",
				Relation:           BundleCorrelationExactMatch,
				NublarRunID:        "run-1",
				CustodySourceRunID: "run-1",
			}},
		}},
	}

	var output bytes.Buffer
	if err := WriteSeriesText(&output, series); err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"correlations:", "nublar-run-to-custody-source after: exact-match", "Nublar run \"run-1\""} {
		if !strings.Contains(output.String(), fragment) {
			t.Fatalf("series text = %q, missing %q", output.String(), fragment)
		}
	}
}

func TestWriteSeriesSummaryOmitsPointEntries(t *testing.T) {
	series := ComparisonSeries{
		Manifest: "series.json",
		Summary: SeriesSummary{
			Entries:      2,
			Compatible:   1,
			Incompatible: 1,
			TotalChanges: 3,
			ChangesByID:  map[string]int{"verdict.exit_code": 1, "verdict.status": 2},
		},
		Entries: []SeriesPoint{{ID: "one"}, {ID: "two"}},
	}

	var jsonOutput bytes.Buffer
	if err := WriteSeriesSummaryJSON(&jsonOutput, series); err != nil {
		t.Fatal(err)
	}
	var document SeriesSummaryReport
	if err := json.Unmarshal(jsonOutput.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if document.Schema != comparisonSeriesSummarySchema || document.Summary.TotalChanges != 3 {
		t.Fatalf("summary document = %+v, want summary schema and totals", document)
	}
	if strings.Contains(jsonOutput.String(), `"entries": [`) {
		t.Fatalf("summary JSON = %q, unexpectedly contains point entries", jsonOutput.String())
	}

	var textOutput bytes.Buffer
	if err := WriteSeriesSummaryText(&textOutput, series); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(textOutput.String(), "  points:") || !strings.Contains(textOutput.String(), "Sattler comparison series summary") {
		t.Fatalf("summary text = %q, want summary heading without points", textOutput.String())
	}
}

func TestWriteSeriesLatestProjectsFinalPoint(t *testing.T) {
	series := ComparisonSeries{
		Manifest:       "series.json",
		ChangeIDFilter: []string{"verdict.status"},
		Entries: []SeriesPoint{
			{ID: "one", Summary: BundleSummary{Compatible: true}},
			{ID: "two", Label: "current", Latest: true, Manifest: "bundle/two.json", Summary: BundleSummary{
				Compatible:           false,
				CompatibilityReasons: []string{"workflow changed"},
				ChangeSummary:        ChangeSummary{Total: 1, ByCategory: map[string]int{"context": 1}},
				Subsystems: map[string]BundleSubsystemSummary{
					"nublar_run": {
						Compatible:    false,
						Transition:    NewStateTransition("status", "passed", "failed", false),
						ChangeSummary: ChangeSummary{Total: 1, ByCategory: map[string]int{"context": 1}},
					},
				},
			}},
		},
	}

	point, ok := LatestSeriesPoint(series)
	if !ok || point.ID != "two" || point.Label != "current" {
		t.Fatalf("latest point = %+v, ok=%t, want final point", point, ok)
	}

	var jsonOutput bytes.Buffer
	if err := WriteSeriesLatestJSON(&jsonOutput, series); err != nil {
		t.Fatal(err)
	}
	var document SeriesLatestReport
	if err := json.Unmarshal(jsonOutput.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if document.Schema != comparisonSeriesLatestSchema || document.Point.ID != "two" || len(document.ChangeIDFilter) != 1 {
		t.Fatalf("latest document = %+v, want final point projection", document)
	}
	if strings.Contains(jsonOutput.String(), `"entries": [`) {
		t.Fatalf("latest JSON = %q, unexpectedly contains series entries", jsonOutput.String())
	}

	var textOutput bytes.Buffer
	if err := WriteSeriesLatestText(&textOutput, series); err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"Sattler latest comparison point", "id: two", "label: current", "change ID filter: verdict.status"} {
		if !strings.Contains(textOutput.String(), fragment) {
			t.Fatalf("latest text = %q, missing %q", textOutput.String(), fragment)
		}
	}
}

func TestCompareSeriesManifestAggregatesCorrelationObservations(t *testing.T) {
	points := []SeriesPoint{{
		Correlations: []BundleCorrelation{
			{Kind: BundleCorrelationKindNublarCustodySource, Relation: BundleCorrelationExactMatch},
			{Kind: BundleCorrelationKindNublarCustodySource, Relation: BundleCorrelationMismatch},
			{Kind: BundleCorrelationKindNublarAmber, Relation: BundleCorrelationUnknown},
		},
	}}
	summary := summarizeSeriesCorrelations(points)

	if summary.Observations != 3 {
		t.Fatalf("correlation observations = %d, want 3", summary.Observations)
	}
	if summary.ByKind[BundleCorrelationKindNublarCustodySource] != 2 || summary.ByKind[BundleCorrelationKindNublarAmber] != 1 {
		t.Fatalf("correlations by kind = %+v, want custody=2 and Amber=1", summary.ByKind)
	}
	if summary.ByRelation[string(BundleCorrelationExactMatch)] != 1 || summary.ByRelation[string(BundleCorrelationMismatch)] != 1 || summary.ByRelation[string(BundleCorrelationUnknown)] != 1 {
		t.Fatalf("correlations by relation = %+v, want one of each", summary.ByRelation)
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

func writeMutationSeriesBundle(t *testing.T, root, directory string) string {
	t.Helper()
	directoryPath := filepath.Join(root, directory)
	if err := os.MkdirAll(directoryPath, 0o755); err != nil {
		t.Fatal(err)
	}
	before := validArtifact()
	before.Kind = "mutation-campaign"
	before.Report = json.RawMessage(`{"schema":"ingen.mutation-campaign-result/v1","status":"passed","plan":{"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"summary":{"total":2,"killed":2,"survived":0,"inconclusive":0,"other":0,"errors":0},"entries":[{"mutation_id":"alpha","status":"passed","outcome":"killed"},{"mutation_id":"beta","status":"passed","outcome":"killed"}]}`)
	after := validArtifact()
	after.Kind = "mutation-campaign"
	after.Status = "failed"
	after.ExitCode = 1
	after.Report = json.RawMessage(`{"schema":"ingen.mutation-campaign-result/v1","status":"failed","plan":{"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"summary":{"total":2,"killed":1,"survived":1,"inconclusive":0,"other":0,"errors":0},"entries":[{"mutation_id":"alpha","status":"failed","outcome":"survived"},{"mutation_id":"beta","status":"passed","outcome":"killed"}]}`)
	writeSeriesFile(t, filepath.Join(directoryPath, "before.json"), marshalJSON(t, before))
	writeSeriesFile(t, filepath.Join(directoryPath, "after.json"), marshalJSON(t, after))
	manifest := ComparisonManifest{
		Schema: ComparisonManifestSchema,
		Before: ComparisonInputs{CIResult: "before.json"},
		After:  ComparisonInputs{CIResult: "after.json"},
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
