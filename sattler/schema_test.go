package sattler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

const (
	sattlerInputSchemaID              = "https://ingen.example/spec/sattler-input-v0.schema.json"
	sattlerProjectionSchemaID         = "https://ingen.example/spec/sattler-projections-v0.schema.json"
	sattlerErrorSchemaID              = "https://ingen.example/spec/sattler-error-v0.schema.json"
	sattlerComparisonSchemaID         = "https://ingen.example/spec/sattler-comparison-v0.schema.json"
	sattlerBundleSummarySchemaID      = "https://ingen.example/spec/sattler-bundle-summary-v0.schema.json"
	sattlerBundleSchemaID             = "https://ingen.example/spec/sattler-bundle-comparison-v0.schema.json"
	sattlerSornaComparisonSchemaID    = "https://ingen.example/spec/sattler-sorna-run-comparison-v0.schema.json"
	sattlerNublarComparisonSchemaID   = "https://ingen.example/spec/sattler-nublar-run-comparison-v0.schema.json"
	sattlerLockwoodComparisonSchemaID = "https://ingen.example/spec/sattler-lockwood-custody-comparison-v0.schema.json"
	sattlerSeriesSummarySchemaID      = "https://ingen.example/spec/sattler-series-summary-v0.schema.json"
	sattlerSeriesLatestSchemaID       = "https://ingen.example/spec/sattler-series-latest-v0.schema.json"
	sattlerInvestigationSchemaID      = "https://ingen.example/spec/sattler-investigation-v0.schema.json"
	sattlerSeriesQuerySchemaID        = "https://ingen.example/spec/sattler-series-query-v0.schema.json"
	sattlerSeriesSchemaID             = "https://ingen.example/spec/sattler-series-v0.schema.json"
)

func TestSattlerInputSchemaValidatesCheckedInManifests(t *testing.T) {
	schema := loadSattlerSchema(t, filepath.Join("spec", "sattler-input-v0.schema.json"), sattlerInputSchemaID)
	for _, name := range []string{"comparison.json", "series.json"} {
		t.Run(name, func(t *testing.T) {
			data := readSattlerFixture(t, filepath.Join("testdata", "cross-artifact", name))
			var document any
			if err := json.Unmarshal(data, &document); err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(document); err != nil {
				t.Fatalf("%s rejected by Sattler input schema: %v", name, err)
			}
		})
	}
}

func TestSattlerProjectionSchemaValidatesEveryEmittedJSONSurface(t *testing.T) {
	schema := loadSattlerSchema(t, filepath.Join("spec", "sattler-projections-v0.schema.json"), sattlerProjectionSchemaID)
	fixture := func(name string) string {
		return filepath.Join("testdata", "cross-artifact", name)
	}
	ci, err := CompareFiles(fixture("before-ci.json"), fixture("after-ci.json"))
	if err != nil {
		t.Fatal(err)
	}
	sorna, err := CompareSornaRunFiles(fixture("before-sorna-run.json"), fixture("after-sorna-run.json"))
	if err != nil {
		t.Fatal(err)
	}
	nublar, err := CompareNublarRunFiles(fixture("before-nublar.json"), fixture("after-nublar.json"))
	if err != nil {
		t.Fatal(err)
	}
	custody, err := CompareLockwoodCustodyFiles(fixture("before-custody.json"), fixture("after-custody.json"))
	if err != nil {
		t.Fatal(err)
	}
	provenance, err := CompareAmberProvenanceFiles(fixture("before-provenance.json"), fixture("after-provenance.json"))
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := CompareBundleFile(fixture("comparison.json"))
	if err != nil {
		t.Fatal(err)
	}
	series, err := CompareSeriesManifestFile(fixture("series.json"))
	if err != nil {
		t.Fatal(err)
	}
	query, err := QuerySeriesManifestFile(fixture("series.json"), SeriesQuerySelector{Kind: SeriesQueryRule, Value: "document.create.accepted"})
	if err != nil {
		t.Fatal(err)
	}
	errorDocument := ErrorDocumentFor("schema test", errors.New("fixture failure"))

	tests := []struct {
		name  string
		write func(io.Writer) error
	}{
		{name: "comparison", write: func(w io.Writer) error { return WriteJSON(w, ci) }},
		{name: "sorna", write: func(w io.Writer) error { return WriteSornaJSON(w, sorna) }},
		{name: "nublar", write: func(w io.Writer) error { return WriteNublarJSON(w, nublar) }},
		{name: "custody", write: func(w io.Writer) error { return WriteLockwoodJSON(w, custody) }},
		{name: "provenance", write: func(w io.Writer) error { return WriteAmberProvenanceJSON(w, provenance) }},
		{name: "bundle", write: func(w io.Writer) error { return WriteBundleJSON(w, bundle) }},
		{name: "bundle-summary", write: func(w io.Writer) error { return WriteBundleSummaryJSON(w, bundle) }},
		{name: "series", write: func(w io.Writer) error { return WriteSeriesJSON(w, series) }},
		{name: "series-summary", write: func(w io.Writer) error { return WriteSeriesSummaryJSON(w, series) }},
		{name: "series-latest", write: func(w io.Writer) error { return WriteSeriesLatestJSON(w, series) }},
		{name: "investigation", write: func(w io.Writer) error { return WriteInvestigationJSON(w, NewInvestigationReport(bundle)) }},
		{name: "series-query", write: func(w io.Writer) error { return WriteSeriesQueryJSON(w, query) }},
		{name: "error", write: func(w io.Writer) error {
			return WriteErrorJSON(w, errorDocument.Operation, errors.New(errorDocument.Errors[0].Message))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := test.write(&output); err != nil {
				t.Fatal(err)
			}
			var document any
			if err := json.Unmarshal(output.Bytes(), &document); err != nil {
				t.Fatalf("%s output is not JSON: %v", test.name, err)
			}
			if err := schema.Validate(document); err != nil {
				t.Fatalf("%s output rejected by Sattler projection schema: %v\n%s", test.name, err, output.String())
			}
		})
	}
}

func TestPromotedErrorSchemaIsStrictAndRuntimeValidated(t *testing.T) {
	schema := loadSattlerSchema(t, filepath.Join("spec", "sattler-error-v0.schema.json"), sattlerErrorSchemaID)
	var output bytes.Buffer
	if err := WriteErrorJSON(&output, "bundle compare", &ManifestValidationError{Issues: []ErrorIssue{{
		Code:    "incomplete-pair",
		Path:    "ci_result",
		Message: "comparison manifest ci_result needs both before and after paths",
	}}}); err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatalf("promoted error output rejected by its schema: %v", err)
	}

	invalid := map[string]any{
		"schema":    ErrorSchema,
		"operation": "bundle compare",
		"errors":    []any{map[string]any{"code": "failure", "message": "broken"}},
		"extra":     true,
	}
	if err := schema.Validate(invalid); err == nil {
		t.Fatal("promoted error schema accepted an unknown top-level field")
	}
	if err := (ErrorDocument{Schema: ErrorSchema, Operation: "bundle compare"}).Validate(); err == nil {
		t.Fatal("incomplete error document passed runtime validation")
	}
}

func TestPromotedComparisonSchemaIsStrictAndRuntimeValidated(t *testing.T) {
	schema := loadSattlerSchema(t, filepath.Join("spec", "sattler-comparison-v0.schema.json"), sattlerComparisonSchemaID)
	beforePath := filepath.Join("testdata", "cross-artifact", "before-ci.json")
	afterPath := filepath.Join("testdata", "cross-artifact", "after-ci.json")
	report, err := CompareFiles(beforePath, afterPath)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := WriteJSON(&output, report); err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatalf("promoted comparison output rejected by its schema: %v", err)
	}

	invalid := document.(map[string]any)
	invalid["extra"] = true
	if err := schema.Validate(invalid); err == nil {
		t.Fatal("promoted comparison schema accepted an unknown top-level field")
	}
	if err := (Comparison{Schema: Schema}).Validate(); err == nil {
		t.Fatal("incomplete comparison passed runtime validation")
	}
}

func TestPromotedSornaComparisonSchemaIsStrictAndRuntimeValidated(t *testing.T) {
	schema := loadSattlerSchema(t, filepath.Join("spec", "sattler-sorna-run-comparison-v0.schema.json"), sattlerSornaComparisonSchemaID)
	sorna, err := CompareSornaRunFiles(filepath.Join("testdata", "cross-artifact", "before-sorna-run.json"), filepath.Join("testdata", "cross-artifact", "after-sorna-run.json"))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := WriteSornaJSON(&output, sorna); err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatalf("promoted Sorna comparison output rejected by its schema: %v", err)
	}

	invalid := document.(map[string]any)
	invalid["extra"] = true
	if err := schema.Validate(invalid); err == nil {
		t.Fatal("promoted Sorna comparison schema accepted an unknown top-level field")
	}
	if err := (SornaRunComparison{Schema: sornaRunComparisonSchema}).Validate(); err == nil {
		t.Fatal("incomplete Sorna comparison passed runtime validation")
	}
	badRules := sorna
	badRules.ChangedRules = append([]SornaRuleStatusChange(nil), sorna.ChangedRules...)
	badRules.ChangedRules = append(badRules.ChangedRules, SornaRuleStatusChange{RuleID: "missing-rule", Before: "pass", After: "fail"})
	if err := WriteSornaJSON(&bytes.Buffer{}, badRules); err == nil {
		t.Fatal("Sorna writer accepted a changed rule without a matching change")
	}
}

func TestPromotedNublarComparisonSchemaIsStrictAndRuntimeValidated(t *testing.T) {
	schema := loadSattlerSchema(t, filepath.Join("spec", "sattler-nublar-run-comparison-v0.schema.json"), sattlerNublarComparisonSchemaID)
	nublar, err := CompareNublarRunFiles(filepath.Join("testdata", "cross-artifact", "before-nublar.json"), filepath.Join("testdata", "cross-artifact", "after-nublar.json"))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := WriteNublarJSON(&output, nublar); err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatalf("promoted Nublar comparison output rejected by its schema: %v", err)
	}

	invalid := document.(map[string]any)
	invalid["extra"] = true
	if err := schema.Validate(invalid); err == nil {
		t.Fatal("promoted Nublar comparison schema accepted an unknown top-level field")
	}
	if err := (NublarRunComparison{Schema: nublarRunComparisonSchema}).Validate(); err == nil {
		t.Fatal("incomplete Nublar comparison passed runtime validation")
	}
	badTransition := nublar
	badTransition.Transition.Field = "workflow.id"
	if err := WriteNublarJSON(&bytes.Buffer{}, badTransition); err == nil {
		t.Fatal("Nublar writer accepted a non-status transition")
	}
}

func TestPromotedLockwoodComparisonSchemaIsStrictAndRuntimeValidated(t *testing.T) {
	schema := loadSattlerSchema(t, filepath.Join("spec", "sattler-lockwood-custody-comparison-v0.schema.json"), sattlerLockwoodComparisonSchemaID)
	custody, err := CompareLockwoodCustodyFiles(filepath.Join("testdata", "cross-artifact", "before-custody.json"), filepath.Join("testdata", "cross-artifact", "after-custody.json"))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := WriteLockwoodJSON(&output, custody); err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatalf("promoted Lockwood comparison output rejected by its schema: %v", err)
	}

	invalid := document.(map[string]any)
	invalid["extra"] = true
	if err := schema.Validate(invalid); err == nil {
		t.Fatal("promoted Lockwood comparison schema accepted an unknown top-level field")
	}
	if err := (LockwoodCustodyComparison{Schema: lockwoodCustodyComparisonSchema}).Validate(); err == nil {
		t.Fatal("incomplete Lockwood comparison passed runtime validation")
	}
	badDigest := custody
	badDigest.Before.ArtifactDigest = "not-a-digest"
	if err := WriteLockwoodJSON(&bytes.Buffer{}, badDigest); err == nil {
		t.Fatal("Lockwood writer accepted an invalid artifact digest")
	}
}

func TestPromotedBundleSummarySchemaIsStrictAndRuntimeValidated(t *testing.T) {
	schema := loadSattlerSchema(t, filepath.Join("spec", "sattler-bundle-summary-v0.schema.json"), sattlerBundleSummarySchemaID)
	report, err := CompareBundleFile(filepath.Join("testdata", "cross-artifact", "comparison.json"))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := WriteBundleSummaryJSON(&output, report); err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatalf("promoted bundle summary output rejected by its schema: %v", err)
	}

	invalid := document.(map[string]any)
	invalid["extra"] = true
	if err := schema.Validate(invalid); err == nil {
		t.Fatal("promoted bundle summary schema accepted an unknown top-level field")
	}
	if err := (BundleSummaryReport{Schema: bundleSummarySchema}).Validate(); err == nil {
		t.Fatal("incomplete bundle summary passed runtime validation")
	}
}

func TestPromotedBundleSchemaIsStrictAndRuntimeValidated(t *testing.T) {
	schema := loadSattlerSchemaResources(t,
		sattlerSchemaResource{path: filepath.Join("spec", "sattler-bundle-comparison-v0.schema.json"), id: sattlerBundleSchemaID},
		sattlerSchemaResource{path: filepath.Join("spec", "sattler-bundle-summary-v0.schema.json"), id: sattlerBundleSummarySchemaID},
	)
	bundle, err := CompareBundleFile(filepath.Join("testdata", "cross-artifact", "comparison.json"))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := WriteBundleJSON(&output, bundle); err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatalf("promoted bundle output rejected by its schema: %v", err)
	}

	invalid := document.(map[string]any)
	invalid["extra"] = true
	if err := schema.Validate(invalid); err == nil {
		t.Fatal("promoted bundle schema accepted an unknown top-level field")
	}
	if err := (BundleComparison{Schema: bundleComparisonSchema}).Validate(); err == nil {
		t.Fatal("incomplete bundle comparison passed runtime validation")
	}
	badAdapter := bundle
	ci := *bundle.CIResult
	ci.Schema = "ingen.invalid-comparison/v0"
	badAdapter.CIResult = &ci
	if err := WriteBundleJSON(&bytes.Buffer{}, badAdapter); err == nil {
		t.Fatal("bundle writer accepted an adapter with an unsupported schema")
	}
}

func TestPromotedSeriesSummarySchemaIsStrictAndRuntimeValidated(t *testing.T) {
	schema := loadSattlerSchema(t, filepath.Join("spec", "sattler-series-summary-v0.schema.json"), sattlerSeriesSummarySchemaID)
	series, err := CompareSeriesManifestFile(filepath.Join("testdata", "cross-artifact", "series.json"))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := WriteSeriesSummaryJSON(&output, series); err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatalf("promoted series summary output rejected by its schema: %v", err)
	}

	invalid := document.(map[string]any)
	invalid["extra"] = true
	if err := schema.Validate(invalid); err == nil {
		t.Fatal("promoted series summary schema accepted an unknown top-level field")
	}
	if err := (SeriesSummaryReport{Schema: comparisonSeriesSummarySchema}).Validate(); err == nil {
		t.Fatal("incomplete series summary passed runtime validation")
	}
}

func TestPromotedSeriesLatestSchemaIsStrictAndRuntimeValidated(t *testing.T) {
	schema := loadSattlerSchemaResources(t,
		sattlerSchemaResource{path: filepath.Join("spec", "sattler-series-latest-v0.schema.json"), id: sattlerSeriesLatestSchemaID},
		sattlerSchemaResource{path: filepath.Join("spec", "sattler-bundle-summary-v0.schema.json"), id: sattlerBundleSummarySchemaID},
	)
	series, err := CompareSeriesManifestFile(filepath.Join("testdata", "cross-artifact", "series.json"))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := WriteSeriesLatestJSON(&output, series); err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatalf("promoted series latest output rejected by its schema: %v", err)
	}

	invalid := document.(map[string]any)
	invalid["extra"] = true
	if err := schema.Validate(invalid); err == nil {
		t.Fatal("promoted series latest schema accepted an unknown top-level field")
	}
	if err := (SeriesLatestReport{Schema: comparisonSeriesLatestSchema}).Validate(); err == nil {
		t.Fatal("incomplete series latest report passed runtime validation")
	}
}

func TestPromotedInvestigationSchemaIsStrictAndRuntimeValidated(t *testing.T) {
	schema := loadSattlerSchema(t, filepath.Join("spec", "sattler-investigation-v0.schema.json"), sattlerInvestigationSchemaID)
	bundle, err := CompareBundleFile(filepath.Join("testdata", "cross-artifact", "comparison.json"))
	if err != nil {
		t.Fatal(err)
	}
	report := NewInvestigationReport(bundle)
	var output bytes.Buffer
	if err := WriteInvestigationJSON(&output, report); err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatalf("promoted investigation output rejected by its schema: %v", err)
	}

	invalid := document.(map[string]any)
	invalid["extra"] = true
	if err := schema.Validate(invalid); err == nil {
		t.Fatal("promoted investigation schema accepted an unknown top-level field")
	}
	if err := (InvestigationReport{Schema: InvestigationSchema, Compatible: true}).Validate(); err == nil {
		t.Fatal("incomplete investigation report passed runtime validation")
	}
	badNotice := report
	badNotice.Notice = "findings are context"
	if err := WriteInvestigationJSON(&bytes.Buffer{}, badNotice); err == nil {
		t.Fatal("investigation writer accepted a non-neutral notice")
	}
}

func TestPromotedSeriesQuerySchemaIsStrictAndRuntimeValidated(t *testing.T) {
	schema := loadSattlerSchemaResources(t,
		sattlerSchemaResource{path: filepath.Join("spec", "sattler-series-query-v0.schema.json"), id: sattlerSeriesQuerySchemaID},
		sattlerSchemaResource{path: filepath.Join("spec", "sattler-bundle-summary-v0.schema.json"), id: sattlerBundleSummarySchemaID},
	)
	query, err := QuerySeriesManifestFile(filepath.Join("testdata", "cross-artifact", "series.json"), SeriesQuerySelector{
		Kind:  SeriesQueryRule,
		Value: "document.create.accepted",
	})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := WriteSeriesQueryJSON(&output, query); err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatalf("promoted series query output rejected by its schema: %v", err)
	}

	invalid := document.(map[string]any)
	invalid["extra"] = true
	if err := schema.Validate(invalid); err == nil {
		t.Fatal("promoted series query schema accepted an unknown top-level field")
	}
	if err := (SeriesQueryReport{Schema: SeriesQuerySchema}).Validate(); err == nil {
		t.Fatal("incomplete series query report passed runtime validation")
	}
	badSummary := query
	badSummary.Summary.Matches++
	if err := WriteSeriesQueryJSON(&bytes.Buffer{}, badSummary); err == nil {
		t.Fatal("series query writer accepted inconsistent summary counts")
	}
}

func TestPromotedSeriesSchemaIsStrictAndRuntimeValidated(t *testing.T) {
	schema := loadSattlerSchemaResources(t,
		sattlerSchemaResource{path: filepath.Join("spec", "sattler-series-v0.schema.json"), id: sattlerSeriesSchemaID},
		sattlerSchemaResource{path: filepath.Join("spec", "sattler-series-summary-v0.schema.json"), id: sattlerSeriesSummarySchemaID},
		sattlerSchemaResource{path: filepath.Join("spec", "sattler-bundle-summary-v0.schema.json"), id: sattlerBundleSummarySchemaID},
	)
	series, err := CompareSeriesManifestFile(filepath.Join("testdata", "cross-artifact", "series.json"))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := WriteSeriesJSON(&output, series); err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatalf("promoted series output rejected by its schema: %v", err)
	}

	invalid := document.(map[string]any)
	invalid["extra"] = true
	if err := schema.Validate(invalid); err == nil {
		t.Fatal("promoted series schema accepted an unknown top-level field")
	}
	if err := (ComparisonSeries{Schema: comparisonSeriesSchema}).Validate(); err == nil {
		t.Fatal("incomplete comparison series passed runtime validation")
	}
	badLatest := series
	badLatest.Entries = append([]SeriesPoint(nil), series.Entries...)
	badLatest.Entries[len(badLatest.Entries)-1].Latest = false
	if err := WriteSeriesJSON(&bytes.Buffer{}, badLatest); err == nil {
		t.Fatal("series writer accepted an unmarked final point")
	}
}

func TestSattlerManifestBoundaryRejectsMalformedAndUnknownInput(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name string
		data string
		code string
	}{
		{
			name: "malformed comparison JSON",
			data: `{"schema":"ingen.sattler-comparison-input/v0"`,
			code: "parse-manifest",
		},
		{
			name: "unknown comparison field",
			data: `{"schema":"ingen.sattler-comparison-input/v0","before":{"ci_result":"before.json"},"after":{"ci_result":"after.json"},"extra":true}`,
			code: "parse-manifest",
		},
		{
			name: "trailing comparison document",
			data: `{"schema":"ingen.sattler-comparison-input/v0","before":{"ci_result":"before.json"},"after":{"ci_result":"after.json"}}
{"schema":"extra"}`,
			code: "parse-manifest",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(root, strings.ReplaceAll(test.name, " ", "-")+".json")
			if err := os.WriteFile(path, []byte(test.data), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := CompareBundleFile(path)
			var validationErr *ManifestValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error = %T %v, want ManifestValidationError", err, err)
			}
			if len(validationErr.Issues) != 1 || validationErr.Issues[0].Code != test.code {
				t.Fatalf("validation issues = %+v, want code %q", validationErr.Issues, test.code)
			}
		})
	}
}

func TestSattlerManifestBoundaryRejectsPartialSeriesEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "series.json")
	contents := `{"schema":"ingen.sattler-comparison-series-input/v0","entries":[{"id":"attempt-1"}]}`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := CompareSeriesManifestFile(path)
	var validationErr *ManifestValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T %v, want ManifestValidationError", err, err)
	}
	if len(validationErr.Issues) != 1 || validationErr.Issues[0].Code != "missing-entry-manifest" {
		t.Fatalf("validation issues = %+v, want missing-entry-manifest", validationErr.Issues)
	}
}

func TestMixedProducerCIEnvelopesRemainExplicitlyIncompatible(t *testing.T) {
	before := validArtifact()
	after := validArtifact()
	after.Tool = "paddock"
	report := Compare(before, after)
	if report.Compatible {
		t.Fatal("mixed-producer comparison was marked compatible")
	}
	if len(report.CompatibilityReasons) != 1 || !strings.Contains(report.CompatibilityReasons[0], "tool changed") {
		t.Fatalf("compatibility reasons = %+v, want explicit producer mismatch", report.CompatibilityReasons)
	}
}

func TestWrongProducerForAnAdapterRemainsAnExplicitError(t *testing.T) {
	root := t.TempDir()
	beforePath := filepath.Join(root, "before.json")
	afterPath := filepath.Join(root, "after.json")
	if err := os.WriteFile(beforePath, []byte(nublarFixture("before", strings.Repeat("a", 64))), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(afterPath, []byte(nublarFixture("after", strings.Repeat("a", 64))), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CompareSornaRunFiles(beforePath, afterPath); err == nil || !strings.Contains(err.Error(), "schema must be ingen.run/v1") {
		t.Fatalf("wrong-producer error = %v, want Sorna schema error", err)
	}
}

func TestComparisonTextOutputGolden(t *testing.T) {
	report := Comparison{
		Schema:        Schema,
		Before:        ArtifactSummary{Path: "before.json", Tool: "sorna", Kind: "behavioral-verification", Status: "passed", CreatedAt: "2026-09-17T12:00:00Z"},
		After:         ArtifactSummary{Path: "after.json", Tool: "sorna", Kind: "behavioral-verification", Status: "failed", CreatedAt: "2026-09-17T12:01:00Z"},
		Compatible:    true,
		Transition:    NewStateTransition("status", "passed", "failed", true),
		Changes:       []Change{NewChange("verdict", "status", "passed", "failed")},
		ChangeSummary: ChangeSummary{Total: 1, ByCategory: map[string]int{"verdict": 1}},
	}
	var output bytes.Buffer
	if err := WriteText(&output, report); err != nil {
		t.Fatal(err)
	}
	want := "Sattler comparison\n" +
		"  before: before.json (sorna/behavioral-verification, passed)\n" +
		"  after:  after.json (sorna/behavioral-verification, failed)\n" +
		"  compatible: true\n" +
		"  transition: status \"passed\" -> \"failed\" [changed]\n" +
		"  created: 2026-09-17T12:00:00Z -> 2026-09-17T12:01:00Z\n" +
		"  change summary: total 1 (verdict=1)\n" +
		"  changes:\n" +
		"    - verdict status (id=verdict.status): \"passed\" -> \"failed\"\n"
	if output.String() != want {
		t.Fatalf("comparison text = %q, want %q", output.String(), want)
	}
}

func TestSeriesQueryJSONOrderingGolden(t *testing.T) {
	report := SeriesQueryReport{
		Schema:   SeriesQuerySchema,
		Manifest: "series.json",
		Query:    SeriesQuerySelector{Kind: SeriesQueryWorkflow, Value: "workflow-1"},
		Summary:  SeriesQuerySummary{},
		Entries:  []SeriesQueryPoint{},
	}
	var output bytes.Buffer
	if err := WriteSeriesQueryJSON(&output, report); err != nil {
		t.Fatal(err)
	}
	want := "{\n" +
		"  \"schema\": \"ingen.sattler-series-query/v0\",\n" +
		"  \"manifest\": \"series.json\",\n" +
		"  \"query\": {\n" +
		"    \"kind\": \"workflow\",\n" +
		"    \"value\": \"workflow-1\"\n" +
		"  },\n" +
		"  \"summary\": {\n" +
		"    \"points\": 0,\n" +
		"    \"matches\": 0\n" +
		"  },\n" +
		"  \"entries\": []\n" +
		"}\n"
	if output.String() != want {
		t.Fatalf("series query JSON = %q, want %q", output.String(), want)
	}
}

func loadSattlerSchema(t *testing.T, relativePath, id string) *jsonschema.Schema {
	return loadSattlerSchemaResources(t, sattlerSchemaResource{path: relativePath, id: id})
}

type sattlerSchemaResource struct {
	path string
	id   string
}

func loadSattlerSchemaResources(t *testing.T, resources ...sattlerSchemaResource) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	for _, resource := range resources {
		data := readSattlerFixture(t, resource.path)
		if err := compiler.AddResource(resource.id, bytes.NewReader(data)); err != nil {
			t.Fatalf("add Sattler schema %s: %v", resource.path, err)
		}
	}
	target := resources[0]
	schema, err := compiler.Compile(target.id)
	if err != nil {
		t.Fatalf("compile Sattler schema %s: %v", target.path, err)
	}
	return schema
}

func readSattlerFixture(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
