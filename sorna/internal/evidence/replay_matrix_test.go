package evidence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ingen/core/ciresult"
	"ingen/sorna/internal/runner"
)

func TestBuildReplayMatrixCIResultAggregatesExpectedClassifications(t *testing.T) {
	root := t.TempDir()
	matchedPath := writeReplayMatrixInput(t, root, "passed", validReplayReport())
	drifted := validReplayReport()
	drifted.Status = "drifted"
	drifted.Behavior.Status = "drifted"
	drifted.Rules[0].Status = "outcome-drift"
	drifted.Rules[0].Differences = []string{"response status changed"}
	drifted.ReplayVerdict = runner.ContractVerdict{Status: "fail", Reason: "response drift"}
	driftedPath := writeReplayMatrixInput(t, root, "failed", drifted)
	inconclusive := validReplayReport()
	inconclusive.Status = "inconclusive"
	inconclusive.Behavior.Status = "inconclusive"
	inconclusive.Behavior.ObservationStatus = "unavailable"
	inconclusive.ReplayVerdict = runner.ContractVerdict{Status: "fail", Reason: "stateful replay was incomplete"}
	inconclusivePath := writeReplayMatrixInput(t, root, "error", inconclusive)

	artifact, err := BuildReplayMatrixCIResult([]ReplayMatrixCase{
		{ID: "baseline", Path: matchedPath, ExpectedCIStatus: "passed", ExpectedReplayState: "matched"},
		{ID: "complete-defect", Path: driftedPath, ExpectedCIStatus: "failed", ExpectedReplayState: "drifted"},
		{ID: "stateful-defect", Path: inconclusivePath, ExpectedCIStatus: "error", ExpectedReplayState: "inconclusive"},
	}, ".")
	if err != nil {
		t.Fatal(err)
	}
	if err := artifact.Validate(); err != nil {
		t.Fatalf("shared CI result validation = %v", err)
	}
	if artifact.Status != "passed" || artifact.ExitCode != 0 || artifact.Kind != "behavioral-replay-matrix" {
		t.Fatalf("artifact = %+v, want passed replay matrix", artifact)
	}
	if len(artifact.Inputs) != 3 || artifact.Inputs["replay:baseline"].SHA256 == "" {
		t.Fatalf("artifact inputs = %+v, want all replay input hashes", artifact.Inputs)
	}

	var report ReplayMatrixReport
	if err := json.Unmarshal(artifact.Report, &report); err != nil {
		t.Fatal(err)
	}
	if err := ValidateReplayMatrixReport(report); err != nil {
		t.Fatalf("matrix report validation = %v", err)
	}
	if report.Status != "passed" || report.Total != 3 || report.Matched != 3 || report.Mismatched != 0 {
		t.Fatalf("matrix report = %+v, want all cases matched", report)
	}

	var explanation ReplayMatrixExplanation
	if err := json.Unmarshal(artifact.Explanation, &explanation); err != nil {
		t.Fatal(err)
	}
	if explanation.Schema != replayMatrixExplanationSchema || explanation.Status != "passed" || len(explanation.Failures) != 0 {
		t.Fatalf("matrix explanation = %+v, want passed explanation", explanation)
	}
}

func TestBuildReplayMatrixCIResultFailsOnUnexpectedClassification(t *testing.T) {
	root := t.TempDir()
	path := writeReplayMatrixInput(t, root, "failed", func() ReplayResult {
		result := validReplayReport()
		result.Status = "drifted"
		result.Behavior.Status = "drifted"
		result.Rules[0].Status = "outcome-drift"
		result.Rules[0].Differences = []string{"response status changed"}
		result.ReplayVerdict = runner.ContractVerdict{Status: "fail", Reason: "response drift"}
		return result
	}())

	artifact, err := BuildReplayMatrixCIResult([]ReplayMatrixCase{
		{ID: "unexpected", Path: path, ExpectedCIStatus: "passed", ExpectedReplayState: "matched"},
	}, ".")
	if err != nil {
		t.Fatal(err)
	}
	if err := artifact.Validate(); err != nil {
		t.Fatalf("shared CI result validation = %v", err)
	}
	if artifact.Status != "failed" || artifact.ExitCode != 1 {
		t.Fatalf("artifact = %+v, want failed matrix", artifact)
	}
	var report ReplayMatrixReport
	if err := json.Unmarshal(artifact.Report, &report); err != nil {
		t.Fatal(err)
	}
	if report.Mismatched != 1 || len(report.Entries) != 1 || report.Entries[0].Status != "mismatched" || len(report.Entries[0].Differences) != 2 {
		t.Fatalf("matrix report = %+v, want one classification mismatch", report)
	}
}

func TestBuildReplayMatrixCIResultRejectsInvalidMember(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "invalid.json")
	if err := os.WriteFile(path, []byte(`{"not":"a ci result"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := BuildReplayMatrixCIResult([]ReplayMatrixCase{
		{ID: "invalid", Path: path, ExpectedCIStatus: "passed", ExpectedReplayState: "matched"},
	}, ".")
	if err == nil || !strings.Contains(err.Error(), "load replay matrix case invalid") {
		t.Fatalf("BuildReplayMatrixCIResult() = %v, want member load error", err)
	}

	errorArtifact, err := BuildReplayMatrixCIErrorResult([]ReplayMatrixCase{
		{ID: "invalid", Path: path, ExpectedCIStatus: "passed", ExpectedReplayState: "matched"},
	}, ".", err)
	if err != nil {
		t.Fatal(err)
	}
	if errorArtifact.Status != "error" || errorArtifact.ExitCode != 2 || errorArtifact.Error == "" {
		t.Fatalf("error artifact = %+v, want collector-friendly error", errorArtifact)
	}
}

func TestValidateReplayMatrixReportRejectsInconsistentCounts(t *testing.T) {
	report := ReplayMatrixReport{
		Schema:     replayMatrixSchema,
		Status:     "passed",
		Total:      1,
		Matched:    0,
		Mismatched: 0,
		Entries: []ReplayMatrixEntry{{
			ID:                  "case",
			Path:                "case.json",
			InputSHA256:         strings.Repeat("a", 64),
			ExpectedCIStatus:    "passed",
			ActualCIStatus:      "passed",
			ExpectedReplayState: "matched",
			ActualReplayState:   "matched",
			Status:              "matched",
		}},
	}
	if err := ValidateReplayMatrixReport(report); err == nil || !strings.Contains(err.Error(), "counts are inconsistent") {
		t.Fatalf("ValidateReplayMatrixReport() = %v, want count error", err)
	}
}

func TestVerifyReplayMatrixCIResultRechecksMembersAndLineage(t *testing.T) {
	root := t.TempDir()
	memberPath := writeReplayMatrixInput(t, root, "passed", validReplayReport())
	artifact, err := BuildReplayMatrixCIResult([]ReplayMatrixCase{{
		ID:                  "baseline",
		Path:                memberPath,
		ExpectedCIStatus:    "passed",
		ExpectedReplayState: "matched",
	}}, ".")
	if err != nil {
		t.Fatal(err)
	}
	matrixPath := filepath.Join(root, "matrix.json")
	if err := ciresult.SaveFile(matrixPath, artifact); err != nil {
		t.Fatal(err)
	}
	if err := VerifyReplayMatrixCIResult(matrixPath, "."); err != nil {
		t.Fatalf("VerifyReplayMatrixCIResult() = %v, want valid matrix", err)
	}

	memberBytes, err := os.ReadFile(memberPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(memberPath, append(memberBytes, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyReplayMatrixCIResult(matrixPath, "."); err == nil || !strings.Contains(err.Error(), "hash changed") {
		t.Fatalf("VerifyReplayMatrixCIResult() after member drift = %v, want hash error", err)
	}
	if err := os.WriteFile(memberPath, memberBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	var report ReplayMatrixReport
	if err := json.Unmarshal(artifact.Report, &report); err != nil {
		t.Fatal(err)
	}
	report.Entries[0].ContractID = "tampered-contract"
	artifact.Report, err = json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := ciresult.SaveFile(matrixPath, artifact); err != nil {
		t.Fatal(err)
	}
	if err := VerifyReplayMatrixCIResult(matrixPath, "."); err == nil || !strings.Contains(err.Error(), "lineage changed") {
		t.Fatalf("VerifyReplayMatrixCIResult() after matrix drift = %v, want lineage error", err)
	}
}

func TestVerifyReplayMatrixCIErrorResultChecksAvailableInputs(t *testing.T) {
	root := t.TempDir()
	memberPath := writeReplayMatrixInput(t, root, "passed", validReplayReport())
	artifact, err := BuildReplayMatrixCIErrorResult([]ReplayMatrixCase{{
		ID:                  "baseline",
		Path:                memberPath,
		ExpectedCIStatus:    "passed",
		ExpectedReplayState: "matched",
	}}, ".", os.ErrInvalid)
	if err != nil {
		t.Fatal(err)
	}
	matrixPath := filepath.Join(root, "matrix-error.json")
	if err := ciresult.SaveFile(matrixPath, artifact); err != nil {
		t.Fatal(err)
	}
	if err := VerifyReplayMatrixCIResult(matrixPath, "."); err != nil {
		t.Fatalf("VerifyReplayMatrixCIResult() for error artifact = %v, want valid error envelope", err)
	}
	if err := os.WriteFile(memberPath, []byte("tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyReplayMatrixCIResult(matrixPath, "."); err == nil || !strings.Contains(err.Error(), "hash changed") {
		t.Fatalf("VerifyReplayMatrixCIResult() after error-input drift = %v, want hash error", err)
	}
}

func writeReplayMatrixInput(t *testing.T, root, status string, replay ReplayResult) string {
	t.Helper()
	report, err := json.Marshal(replay)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, status+"-"+replay.Status+".json")
	artifact := ciresult.Artifact{
		Schema:      ciresult.Schema,
		Tool:        "sorna",
		Kind:        "behavioral-replay",
		Status:      status,
		ExitCode:    map[string]int{"passed": 0, "failed": 1, "error": 2}[status],
		CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Source:      ciresult.Source{Root: "."},
		Report:      report,
		Explanation: json.RawMessage(`{"schema":"sorna.replay-explanation/v1"}`),
	}
	if status == "error" {
		artifact.Error = "replay was incomplete"
	}
	if err := ciresult.SaveFile(path, artifact); err != nil {
		t.Fatal(err)
	}
	return path
}
