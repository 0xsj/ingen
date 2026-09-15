package run

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/core/ciresult"
	"ingen/nublar/internal/workflow"
)

func TestValidateAcceptsPassedRunAndPreservesProducerArtifact(t *testing.T) {
	r := validRun()
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := WriteJSON(&output, r); err != nil {
		t.Fatal(err)
	}
	var decoded Run
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	var report map[string]string
	if err := json.Unmarshal(decoded.Checks[0].Result.Artifact.Report, &report); err != nil {
		t.Fatal(err)
	}
	if report["producer"] != "sorna" {
		t.Fatalf("producer report = %v, want original opaque report", report)
	}
}

func TestValidateComposesFailedProducerStatus(t *testing.T) {
	r := validRun()
	r.Status = "failed"
	r.ExitCode = 1
	r.Checks[0].Status = "failed"
	r.Checks[0].Result.Artifact.Status = "failed"
	r.Checks[0].Result.Artifact.ExitCode = 1
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsProducerIdentityMismatch(t *testing.T) {
	r := validRun()
	r.Checks[0].Result.Artifact.Tool = "paddock"
	if err := r.Validate(); err == nil || !strings.Contains(err.Error(), "producer tool") {
		t.Fatalf("Validate() = %v, want producer identity error", err)
	}
}

func TestValidateRejectsRequiredMissingCheck(t *testing.T) {
	r := validRun()
	r.Checks = []Check{
		{ID: "behavior", Tool: "sorna", Path: "missing.json", Required: true, Status: "missing", Reason: "result was not produced"},
	}
	r.Status = "error"
	r.ExitCode = 2
	if err := r.Validate(); err == nil || !strings.Contains(err.Error(), "required check cannot be missing") {
		t.Fatalf("Validate() = %v, want required-missing error", err)
	}
}

func TestValidateAcceptsOptionalMissingCheckWithPresentResult(t *testing.T) {
	r := validRun()
	r.Checks = append(r.Checks, Check{
		ID:       "architecture",
		Tool:     "paddock",
		Path:     "paddock.json",
		Required: false,
		Status:   "missing",
		Reason:   "optional result was not produced",
	})
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsDecisionThatDoesNotMatchChecks(t *testing.T) {
	r := validRun()
	r.Status = "failed"
	r.ExitCode = 1
	if err := r.Validate(); err == nil || !strings.Contains(err.Error(), "does not match check decisions") {
		t.Fatalf("Validate() = %v, want decision mismatch", err)
	}
}

func TestValidateRequiresResultHash(t *testing.T) {
	r := validRun()
	r.Workflow.File.SHA256 = ""
	if err := r.Validate(); err == nil || !strings.Contains(err.Error(), "sha256 is required") {
		t.Fatalf("Validate() = %v, want required workflow hash", err)
	}
}

func TestNewIDReturnsDistinctOpaqueIDs(t *testing.T) {
	first, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(first) == "" || strings.TrimSpace(second) == "" {
		t.Fatalf("generated IDs must not be blank: %q, %q", first, second)
	}
	if first == second {
		t.Fatalf("generated IDs must be distinct, got %q twice", first)
	}
}

func TestValidateAcceptsErrorRunWithOnlyOptionalMissingChecks(t *testing.T) {
	r := validRun()
	r.Checks = []Check{{
		ID:       "architecture",
		Tool:     "paddock",
		Path:     "paddock.json",
		Required: false,
		Status:   "missing",
		Reason:   "optional result was not produced",
	}}
	r.Status = "error"
	r.ExitCode = 2
	r.Errors = []Issue{{CheckID: "architecture", Tool: "paddock", Path: "paddock.json", Reason: "workflow produced no result artifacts"}}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsDuplicateNormalizedResultPaths(t *testing.T) {
	r := validRun()
	r.Checks = append(r.Checks, Check{
		ID:       "same-result",
		Tool:     "sorna",
		Path:     "nested/../sorna.json",
		Required: false,
		Status:   "missing",
		Reason:   "optional result was not produced",
	})
	if err := r.Validate(); err == nil || !strings.Contains(err.Error(), "result path") {
		t.Fatalf("Validate() = %v, want duplicate result path error", err)
	}
}

func TestCollectWorkflowRecordsMissingOptionalAndMalformedRequiredResults(t *testing.T) {
	root := t.TempDir()
	writeArtifact(t, root, "sorna.json", validArtifact("sorna", "passed", 0))
	document := workflow.Document{
		Schema: workflow.Schema,
		ID:     "test-workflow",
		Checks: []workflow.Check{
			{ID: "behavior", Tool: "sorna", Result: "sorna.json"},
			{ID: "optional", Tool: "paddock", Result: "missing.json", Required: boolPtr(false)},
			{ID: "required", Tool: "sorna", Result: "malformed.json"},
		},
	}
	if err := os.WriteFile(filepath.Join(root, "malformed.json"), []byte("not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := CollectWorkflow(document, fileRef("workflow.yaml"), root, "run-01")
	if r.Status != "error" || r.ExitCode != 2 || len(r.Checks) != 3 || len(r.Warnings) != 1 || len(r.Errors) != 1 {
		t.Fatalf("collected run = %+v, want one warning and one error", r)
	}
	if r.Checks[1].Status != "missing" || r.Checks[1].Path != "missing.json" {
		t.Fatalf("optional check = %+v, want missing path-preserving check", r.Checks[1])
	}
	if r.Checks[2].Status != "error" || r.Checks[2].Path != "malformed.json" {
		t.Fatalf("malformed check = %+v, want blocking error", r.Checks[2])
	}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
}

func validRun() Run {
	return Run{
		Schema:      Schema,
		RunID:       "run-01",
		Workflow:    Workflow{ID: "document-pipeline-ci", File: fileRef("workflow.yaml")},
		Status:      "passed",
		ExitCode:    0,
		CreatedAt:   "2026-09-15T12:00:00Z",
		CompletedAt: "2026-09-15T12:00:01Z",
		Checks: []Check{{
			ID:       "behavioral-verification",
			Tool:     "sorna",
			Path:     "sorna.json",
			Required: true,
			Status:   "passed",
			Result: &Result{
				Ref:      fileRef("sorna.json"),
				Artifact: validArtifact("sorna", "passed", 0),
			},
		}},
	}
}

func validArtifact(tool, status string, exitCode int) ciresult.Artifact {
	return ciresult.Artifact{
		Schema:      ciresult.Schema,
		Tool:        tool,
		Kind:        "test",
		Status:      status,
		ExitCode:    exitCode,
		CreatedAt:   "2026-09-15T12:00:00Z",
		Source:      ciresult.Source{Root: "."},
		Report:      []byte(`{"producer":"` + tool + `"}`),
		Explanation: []byte(`{"explanation":"ok"}`),
	}
}

func writeArtifact(t *testing.T, root, name string, artifact ciresult.Artifact) {
	t.Helper()
	contents, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, name), contents, 0o644); err != nil {
		t.Fatal(err)
	}
}

func boolPtr(value bool) *bool {
	return &value
}

func fileRef(path string) ciresult.FileRef {
	return ciresult.FileRef{Path: path, SHA256: strings.Repeat("a", 64)}
}
