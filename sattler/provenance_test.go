package sattler

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestCompareAmberProvenanceReportsRetryTransition(t *testing.T) {
	beforePath := writeProvenanceFixture(t, amberFixture(
		"00000000-0000-4000-8000-000000000001",
		"00000000-0000-4000-8000-000000000002",
		"00000000-0000-4000-8000-000000000003",
		"local", 0, 1, "normal", "",
	))
	afterPath := writeProvenanceFixture(t, amberFixture(
		"00000000-0000-4000-8000-000000000001",
		"00000000-0000-4000-8000-000000000004",
		"00000000-0000-4000-8000-000000000003",
		"retry", 0, 2, "retry", "00000000-0000-4000-8000-000000000002",
	))

	report, err := CompareAmberProvenanceFiles(beforePath, afterPath)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Compatible {
		t.Fatal("retry of the same logical work was marked incompatible")
	}
	if got, want := len(report.Changes), 5; got != want {
		t.Fatalf("change count = %d, want %d: %+v", got, want, report.Changes)
	}
	assertChange(t, report.Changes[0], "identity", "execution_id")
	assertChange(t, report.Changes[1], "context", "origin")
	assertChange(t, report.Changes[2], "context", "attempt")
	assertChange(t, report.Changes[3], "context", "mode.kind")
	assertChange(t, report.Changes[4], "context", "mode.of_execution_id")
}

func TestCompareAmberProvenanceExplainsWorkDrift(t *testing.T) {
	beforePath := writeProvenanceFixture(t, amberFixture(
		"00000000-0000-4000-8000-000000000001",
		"00000000-0000-4000-8000-000000000002",
		"00000000-0000-4000-8000-000000000003",
		"local", 0, 1, "normal", "",
	))
	afterPath := writeProvenanceFixture(t, amberFixture(
		"00000000-0000-4000-8000-000000000005",
		"00000000-0000-4000-8000-000000000006",
		"00000000-0000-4000-8000-000000000003",
		"local", 0, 1, "normal", "",
	))

	report, err := CompareAmberProvenanceFiles(beforePath, afterPath)
	if err != nil {
		t.Fatal(err)
	}
	if report.Compatible {
		t.Fatal("different logical work IDs were marked compatible")
	}
	if len(report.CompatibilityReasons) != 1 || !strings.Contains(report.CompatibilityReasons[0], "work id changed") {
		t.Fatalf("compatibility reasons = %+v, want work-ID explanation", report.CompatibilityReasons)
	}
}

func TestCompareAmberProvenanceRejectsWrongVersion(t *testing.T) {
	path := writeProvenanceFixture(t, `{"version":2}`)
	if _, err := CompareAmberProvenanceFiles(path, path); err == nil || !strings.Contains(err.Error(), "Amber provenance version") {
		t.Fatalf("error = %v, want version error", err)
	}
}

func TestWriteAmberProvenanceTextNamesIdentityBoundary(t *testing.T) {
	path := writeProvenanceFixture(t, amberFixture(
		"00000000-0000-4000-8000-000000000001",
		"00000000-0000-4000-8000-000000000002",
		"00000000-0000-4000-8000-000000000003",
		"local", 0, 1, "normal", "",
	))
	report, err := CompareAmberProvenanceFiles(path, path)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := WriteAmberProvenanceText(&output, report); err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"Sattler Amber provenance comparison", "compatible: true", "none observable at the Amber boundary"} {
		if !strings.Contains(output.String(), fragment) {
			t.Fatalf("text output = %q, missing %q", output.String(), fragment)
		}
	}
}

func writeProvenanceFixture(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "provenance.json")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func amberFixture(workID, executionID, correlationID, origin string, depth, attempt uint64, modeKind, ofExecutionID string) string {
	return `{"version":1,"work_id":"` + workID + `","execution_id":"` + executionID + `","correlation_id":"` + correlationID + `","origin":"` + origin + `","depth":` + fmtUint(depth) + `,"attempt":` + fmtUint(attempt) + `,"mode":{"kind":"` + modeKind + `"` + modeSource(ofExecutionID) + `}}`
}

func fmtUint(value uint64) string {
	return strconv.FormatUint(value, 10)
}

func modeSource(executionID string) string {
	if executionID == "" {
		return ""
	}
	return `,"of_execution_id":"` + executionID + `"`
}
