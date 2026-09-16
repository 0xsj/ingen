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

func TestCompareBundleFileCombinesDeclaredArtifactPairs(t *testing.T) {
	root := t.TempDir()

	beforeCI := validArtifact()
	afterCI := validArtifact()
	afterCI.Status = "failed"
	afterCI.ExitCode = 1
	writeBundleArtifact(t, root, "before-ci.json", marshalJSON(t, beforeCI))
	writeBundleArtifact(t, root, "after-ci.json", marshalJSON(t, afterCI))
	writeBundleArtifact(t, root, "before-run.json", []byte(nublarFixture("run-before", strings.Repeat("a", 64))))
	writeBundleArtifact(t, root, "after-run.json", []byte(nublarFixture("run-after", strings.Repeat("a", 64))))
	writeBundleArtifact(t, root, "before-custody.json", []byte(custodyFixture("custody-before", "accepted", "sha256:"+strings.Repeat("a", 64), "run-before")))
	writeBundleArtifact(t, root, "after-custody.json", []byte(custodyFixture("custody-after", "accepted", "sha256:"+strings.Repeat("a", 64), "run-after")))
	writeBundleArtifact(t, root, "before-provenance.json", []byte(amberFixture("00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002", "00000000-0000-4000-8000-000000000003", "local", 0, 1, "normal", "")))
	writeBundleArtifact(t, root, "after-provenance.json", []byte(amberFixture("00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000004", "00000000-0000-4000-8000-000000000003", "retry", 0, 2, "retry", "00000000-0000-4000-8000-000000000002")))

	manifest := ComparisonManifest{
		Schema: ComparisonManifestSchema,
		Before: ComparisonInputs{CIResult: "before-ci.json", NublarRun: "before-run.json", Custody: "before-custody.json", Provenance: "before-provenance.json"},
		After:  ComparisonInputs{CIResult: "after-ci.json", NublarRun: "after-run.json", Custody: "after-custody.json", Provenance: "after-provenance.json"},
	}
	manifestPath := filepath.Join(root, "comparison.json")
	writeBundleArtifact(t, root, "comparison.json", marshalJSON(t, manifest))

	report, err := CompareBundleFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if report.CIResult == nil || report.NublarRun == nil || report.Custody == nil || report.Provenance == nil {
		t.Fatalf("bundle report omitted a declared comparison: %+v", report)
	}
	if report.CIResult.ChangeSummary.Total != 2 {
		t.Fatalf("CI change summary = %+v, want two verdict changes", report.CIResult.ChangeSummary)
	}
	if !report.Summary.Compatible {
		t.Fatalf("bundle summary marked compatible comparisons incompatible: %+v", report.Summary)
	}
	if report.Summary.ChangeSummary.Total != report.CIResult.ChangeSummary.Total+
		report.NublarRun.ChangeSummary.Total+report.Custody.ChangeSummary.Total+report.Provenance.ChangeSummary.Total {
		t.Fatalf("bundle change summary = %+v, want sum of subsystem changes", report.Summary.ChangeSummary)
	}
	if report.Summary.Subsystems["ci_result"].ChangeSummary.Total != 2 {
		t.Fatalf("CI subsystem summary = %+v, want two changes", report.Summary.Subsystems["ci_result"])
	}

	var output bytes.Buffer
	if err := WriteBundleText(&output, report); err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"Sattler bundle comparison", "ci result:", "nublar run:", "custody:", "provenance:"} {
		if !strings.Contains(output.String(), fragment) {
			t.Fatalf("bundle text = %q, missing %q", output.String(), fragment)
		}
	}
}

func TestSummarizeBundlePrefixesCompatibilityReasons(t *testing.T) {
	report := BundleComparison{
		NublarRun: &NublarRunComparison{
			Compatible:           false,
			CompatibilityReasons: []string{"workflow id changed"},
			Changes:              []Change{{Category: "context", Field: "workflow.id"}},
		},
		Provenance: &AmberProvenanceComparison{
			Compatible: true,
			Changes:    []Change{{Category: "identity", Field: "execution_id"}},
		},
	}

	summary := SummarizeBundle(report)
	if summary.Compatible {
		t.Fatal("bundle summary marked an incompatible subsystem compatible")
	}
	if len(summary.CompatibilityReasons) != 1 || summary.CompatibilityReasons[0] != "nublar_run: workflow id changed" {
		t.Fatalf("compatibility reasons = %+v, want subsystem-qualified reason", summary.CompatibilityReasons)
	}
	if summary.ChangeSummary.Total != 2 {
		t.Fatalf("bundle total = %+v, want two changes", summary.ChangeSummary)
	}
	if len(summary.Subsystems) != 2 {
		t.Fatalf("subsystem summaries = %+v, want two entries", summary.Subsystems)
	}
}

func TestCompareBundleFileRequiresCompletePairs(t *testing.T) {
	root := t.TempDir()
	manifest := ComparisonManifest{
		Schema: ComparisonManifestSchema,
		Before: ComparisonInputs{CIResult: "before.json"},
	}
	path := filepath.Join(root, "comparison.json")
	writeBundleArtifact(t, root, "comparison.json", marshalJSON(t, manifest))
	if _, err := CompareBundleFile(path); err == nil || !strings.Contains(err.Error(), "ci_result needs both") {
		t.Fatalf("error = %v, want incomplete-pair error", err)
	}
}

func TestComparisonManifestValidateReportsStableIssues(t *testing.T) {
	manifest := ComparisonManifest{
		Schema: ComparisonManifestSchema + ".unknown",
		Before: ComparisonInputs{CIResult: "before.json"},
	}
	var validationErr *ManifestValidationError
	if err := manifest.Validate(); !errors.As(err, &validationErr) {
		t.Fatalf("manifest validation error = %T %v, want ManifestValidationError", err, err)
	}
	if len(validationErr.Issues) != 2 {
		t.Fatalf("validation issues = %+v, want schema and pair issues", validationErr.Issues)
	}
	if validationErr.Issues[0].Code != "invalid-schema" || validationErr.Issues[1].Code != "incomplete-pair" {
		t.Fatalf("validation issue codes = %+v, want invalid-schema then incomplete-pair", validationErr.Issues)
	}

	if err := (ComparisonManifest{Schema: ComparisonManifestSchema}).Validate(); err == nil {
		t.Fatal("empty manifest validated, want no-artifact-pairs error")
	} else if !strings.Contains(err.Error(), "declares no artifact pairs") {
		t.Fatalf("empty manifest error = %v", err)
	}
}

func TestWriteErrorJSONUsesStableEnvelope(t *testing.T) {
	err := &ManifestValidationError{Issues: []ErrorIssue{{
		Code:    "incomplete-pair",
		Path:    "ci_result",
		Message: "comparison manifest ci_result needs both before and after paths",
	}}}
	var output bytes.Buffer
	if writeErr := WriteErrorJSON(&output, "bundle compare", err); writeErr != nil {
		t.Fatal(writeErr)
	}
	var document ErrorDocument
	if decodeErr := json.Unmarshal(output.Bytes(), &document); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if document.Schema != ErrorSchema || document.Operation != "bundle compare" {
		t.Fatalf("error document = %+v, want stable schema and operation", document)
	}
	if len(document.Errors) != 1 || document.Errors[0].Code != "incomplete-pair" {
		t.Fatalf("error document issues = %+v, want incomplete-pair", document.Errors)
	}
}

func writeBundleArtifact(t *testing.T, root, name string, contents []byte) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func marshalJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
