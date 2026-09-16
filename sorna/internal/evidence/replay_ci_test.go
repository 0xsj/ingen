package evidence

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"ingen/sorna/internal/oracle"
	"ingen/sorna/internal/runner"
)

func TestBuildReplayCIResultBindsReplayInputs(t *testing.T) {
	artifact := replayArtifact()
	root := t.TempDir()
	oraclePath := filepath.Join(root, "oracle.json")
	if _, err := oracle.WriteFile(oraclePath, artifact); err != nil {
		t.Fatal(err)
	}
	recorded, err := runner.ExecuteOracle(context.Background(), artifact, runner.Config{BaseURL: "http://recorded.invalid", Client: replayClient(200, `{}`)})
	if err != nil {
		t.Fatal(err)
	}
	evidenceDir := filepath.Join(root, "evidence")
	if _, err := WriteBundle(evidenceDir, recorded, nil); err != nil {
		t.Fatal(err)
	}
	result, err := buildReplayCIResult(evidenceDir, oraclePath, ".", runner.Config{BaseURL: "http://replay.invalid", Client: replayClient(200, `{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("shared CI result validation = %v", err)
	}
	if result.Status != "passed" || result.ExitCode != 0 || result.Kind != "behavioral-replay" {
		t.Fatalf("CI result = %+v, want passed behavioral replay", result)
	}
	for _, name := range []string{"oracle", "evidence_manifest", "evidence_checksums", "evidence:run.json", "evidence:events/lifecycle.jsonl"} {
		if result.Inputs[name].SHA256 == "" {
			t.Fatalf("CI inputs = %+v, want hash for %s", result.Inputs, name)
		}
	}
	var report ReplayResult
	if err := json.Unmarshal(result.Report, &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "matched" || report.Integrity.Status != "verified" || report.Behavior.Status != "matched" {
		t.Fatalf("replay report = %+v, want matching report", report)
	}
	var explanation ReplayCIExplanation
	if err := json.Unmarshal(result.Explanation, &explanation); err != nil {
		t.Fatal(err)
	}
	if explanation.Schema != replayExplanationSchema || explanation.Status != "passed" || explanation.ReplayStatus != "matched" || explanation.RequestStatus != "same" || explanation.RequestChanges != 0 {
		t.Fatalf("replay explanation = %+v, want passed matching explanation", explanation)
	}
}

func TestBuildReplayCIResultMapsOutcomeDriftToFailed(t *testing.T) {
	artifact := replayArtifact()
	root := t.TempDir()
	oraclePath := filepath.Join(root, "oracle.json")
	if _, err := oracle.WriteFile(oraclePath, artifact); err != nil {
		t.Fatal(err)
	}
	recorded, err := runner.ExecuteOracle(context.Background(), artifact, runner.Config{BaseURL: "http://recorded.invalid", Client: replayClient(200, `{}`)})
	if err != nil {
		t.Fatal(err)
	}
	evidenceDir := filepath.Join(root, "evidence")
	if _, err := WriteBundle(evidenceDir, recorded, nil); err != nil {
		t.Fatal(err)
	}
	result, err := buildReplayCIResult(evidenceDir, oraclePath, ".", runner.Config{BaseURL: "http://drifted.invalid", Client: replayClient(500, `{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "failed" || result.ExitCode != 1 {
		t.Fatalf("CI result = %+v, want failed outcome drift", result)
	}
	var report ReplayResult
	if err := json.Unmarshal(result.Report, &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "drifted" || report.Behavior.Status != "drifted" {
		t.Fatalf("replay report = %+v, want drifted report", report)
	}
}

func TestBuildReplayCIResultMapsExecutionErrorToError(t *testing.T) {
	artifact := replayArtifact()
	root := t.TempDir()
	oraclePath := filepath.Join(root, "oracle.json")
	if _, err := oracle.WriteFile(oraclePath, artifact); err != nil {
		t.Fatal(err)
	}
	recorded, err := runner.ExecuteOracle(context.Background(), artifact, runner.Config{BaseURL: "http://recorded.invalid", Client: replayClient(200, `{}`)})
	if err != nil {
		t.Fatal(err)
	}
	evidenceDir := filepath.Join(root, "evidence")
	if _, err := WriteBundle(evidenceDir, recorded, nil); err != nil {
		t.Fatal(err)
	}
	result, err := buildReplayCIResult(evidenceDir, oraclePath, ".", runner.Config{BaseURL: "http://unreachable.invalid", Client: replayErrorClient()})
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("shared CI result validation = %v", err)
	}
	if result.Status != "error" || result.ExitCode != 2 || result.Error == "" {
		t.Fatalf("CI result = %+v, want error with explanation", result)
	}
	var report ReplayResult
	if err := json.Unmarshal(result.Report, &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "error" || report.Behavior.Status != "error" {
		t.Fatalf("replay report = %+v, want execution error", report)
	}
}

func TestBuildReplayCIErrorResultIsCollectorFriendly(t *testing.T) {
	oraclePath := filepath.Join(t.TempDir(), "oracle.json")
	if err := os.WriteFile(oraclePath, []byte("not-json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := BuildReplayCIErrorResult("missing-evidence", oraclePath, ".", os.ErrInvalid)
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("shared CI error validation = %v", err)
	}
	if result.Status != "error" || result.ExitCode != 2 || result.Kind != "behavioral-replay" || result.Error != os.ErrInvalid.Error() {
		t.Fatalf("CI error result = %+v, want collector-friendly replay error", result)
	}
	if result.Inputs["oracle"].SHA256 == "" {
		t.Fatalf("CI error inputs = %+v, want available oracle hash", result.Inputs)
	}
}
