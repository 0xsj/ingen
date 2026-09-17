package sattler

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"ingen/core/ciresult"
)

func TestQuerySeriesManifestPreservesOrderAndRuleSources(t *testing.T) {
	fixtureManifest, err := filepath.Abs(filepath.Join("testdata", "cross-artifact", "comparison.json"))
	if err != nil {
		t.Fatal(err)
	}
	seriesPath := filepath.Join(t.TempDir(), "series.json")
	writeSeriesFile(t, seriesPath, marshalJSON(t, ComparisonSeriesManifest{
		Schema: ComparisonSeriesManifestSchema,
		Entries: []ComparisonSeriesEntry{
			{ID: "first", Label: "first observation", Manifest: fixtureManifest},
			{ID: "second", Label: "second observation", Manifest: fixtureManifest},
		},
	}))

	report, err := QuerySeriesManifestFile(seriesPath, SeriesQuerySelector{Kind: SeriesQueryRule, Value: "document.create.accepted"})
	if err != nil {
		t.Fatal(err)
	}
	if report.Schema != SeriesQuerySchema || report.Query.Kind != SeriesQueryRule || report.Summary.Points != 2 || report.Summary.Matches != 4 {
		t.Fatalf("query report = %+v, want two ordered points and four side matches", report)
	}
	if len(report.Entries) != 2 || report.Entries[0].Point.ID != "first" || report.Entries[0].Point.Latest || report.Entries[1].Point.ID != "second" || !report.Entries[1].Point.Latest {
		t.Fatalf("query entries = %+v, want preserved order and latest marker", report.Entries)
	}
	for _, entry := range report.Entries {
		if len(entry.Matches) != 2 || entry.Matches[0].Side != "after" || entry.Matches[1].Side != "before" {
			t.Fatalf("query matches = %+v, want deterministic side ordering", entry.Matches)
		}
		for _, match := range entry.Matches {
			if match.Source.Path == "" || match.Field != "rules.document.create.accepted.status" || match.Value == "" {
				t.Fatalf("query match = %+v, want source-preserving rule observation", match)
			}
		}
	}

	var first, second bytes.Buffer
	if err := WriteSeriesQueryJSON(&first, report); err != nil {
		t.Fatal(err)
	}
	if err := WriteSeriesQueryJSON(&second, report); err != nil {
		t.Fatal(err)
	}
	if first.String() != second.String() {
		t.Fatal("query JSON is not deterministic")
	}
	var document SeriesQueryReport
	if err := json.Unmarshal(first.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Entries) != 2 || document.Entries[1].Point.Manifest != fixtureManifest {
		t.Fatalf("decoded query document = %+v, want original bundle manifest references", document)
	}

	var textOutput bytes.Buffer
	if err := WriteSeriesQueryText(&textOutput, report); err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"Sattler series query", "query: rule=\"document.create.accepted\"", "matching points: 2", "second (second observation) [latest]", "sorna_run before rules.document.create.accepted.status"} {
		if !strings.Contains(textOutput.String(), fragment) {
			t.Fatalf("query text = %q, missing %q", textOutput.String(), fragment)
		}
	}
}

func TestQuerySeriesManifestSupportsMutationContractAndWorkflow(t *testing.T) {
	fixtureManifest, err := filepath.Abs(filepath.Join("testdata", "cross-artifact", "comparison.json"))
	if err != nil {
		t.Fatal(err)
	}
	seriesPath := filepath.Join(t.TempDir(), "series.json")
	writeSeriesFile(t, seriesPath, []byte(`{"schema":"ingen.sattler-comparison-series-input/v0","entries":[{"id":"one","manifest":"`+fixtureManifest+`"}]}`))

	tests := []struct {
		name        string
		selector    SeriesQuerySelector
		wantPoints  int
		wantMatches int
	}{
		{name: "mutation", selector: SeriesQuerySelector{Kind: SeriesQueryMutation, Value: "mut-alpha"}, wantPoints: 1, wantMatches: 2},
		{name: "contract id", selector: SeriesQuerySelector{Kind: SeriesQueryContract, Value: "document-pipeline"}, wantPoints: 1, wantMatches: 2},
		{name: "workflow id", selector: SeriesQuerySelector{Kind: SeriesQueryWorkflow, Value: "document-pipeline"}, wantPoints: 1, wantMatches: 2},
		{name: "workflow file", selector: SeriesQuerySelector{Kind: SeriesQueryWorkflow, Value: "workflow.yaml"}, wantPoints: 1, wantMatches: 2},
		{name: "no provider", selector: SeriesQuerySelector{Kind: SeriesQueryProvider, Value: "provider.yaml"}, wantPoints: 0, wantMatches: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report, err := QuerySeriesManifestFile(seriesPath, test.selector)
			if err != nil {
				t.Fatal(err)
			}
			if report.Summary.Points != test.wantPoints || report.Summary.Matches != test.wantMatches || len(report.Entries) != test.wantPoints {
				t.Fatalf("query summary = %+v, entries=%+v, want points=%d matches=%d", report.Summary, report.Entries, test.wantPoints, test.wantMatches)
			}
		})
	}
}

func TestSeriesQuerySelectorValidation(t *testing.T) {
	for _, selector := range []SeriesQuerySelector{
		{Kind: SeriesQueryRule},
		{Kind: SeriesQueryKind("unknown"), Value: "value"},
	} {
		if err := selector.Validate(); err == nil {
			t.Fatalf("selector %+v validated, want error", selector)
		}
	}
}

func TestMatchSeriesQuerySupportsProviderReferences(t *testing.T) {
	bundle := BundleComparison{
		CIResult: &Comparison{
			Before: ArtifactSummary{Inputs: map[string]ciresult.FileRef{"provider": {Path: "provider.yaml", SHA256: strings.Repeat("a", 64)}}},
			After:  ArtifactSummary{Inputs: map[string]ciresult.FileRef{"provider": {Path: "provider-v2.yaml", SHA256: strings.Repeat("b", 64)}}},
		},
	}
	matches := matchSeriesQuery(bundle, SeriesQuerySelector{Kind: SeriesQueryProvider, Value: "provider-v2.yaml"})
	if len(matches) != 1 || matches[0].Side != "after" || matches[0].Field != "inputs.provider" {
		t.Fatalf("provider matches = %+v, want one after-side provider observation", matches)
	}
}
