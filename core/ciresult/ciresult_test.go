package ciresult

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArtifactValidatesOpaqueProducerReport(t *testing.T) {
	artifact := Artifact{
		Schema:    Schema,
		Tool:      "sorna",
		Kind:      "behavioral-verification",
		Status:    "passed",
		ExitCode:  0,
		CreatedAt: "2026-09-14T12:00:00Z",
		Source:    Source{Root: ".artifacts/run", ModulePath: "document-pipeline"},
		Inputs: map[string]FileRef{
			"manifest": {Path: "manifest.json", SHA256: strings.Repeat("a", 64)},
		},
		Report:      []byte(`{"schema":"ingen.gate/v1"}`),
		Explanation: []byte(`{"schema":"sorna.gate-explanation/v1"}`),
	}

	var output bytes.Buffer
	if err := WriteJSON(&output, artifact); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"schema": "ingen.ci-result/v1"`) {
		t.Fatalf("output = %s, want shared schema", output.String())
	}
}

func TestArtifactRejectsStatusExitCodeMismatch(t *testing.T) {
	artifact := Artifact{
		Schema:      Schema,
		Tool:        "sorna",
		Kind:        "behavioral-verification",
		Status:      "failed",
		ExitCode:    0,
		CreatedAt:   "2026-09-14T12:00:00Z",
		Source:      Source{Root: ".artifacts/run"},
		Report:      []byte(`{}`),
		Explanation: []byte(`{}`),
	}
	if err := artifact.Validate(); err == nil || !strings.Contains(err.Error(), "exit_code 1") {
		t.Fatalf("Validate() = %v, want failed-status exit-code error", err)
	}
}

func TestLoadFileValidatesTheSharedEnvelope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result.json")
	contents := `{
  "schema": "ingen.ci-result/v1",
  "tool": "test-tool",
  "kind": "test",
  "status": "error",
  "exit_code": 2,
  "created_at": "2026-09-14T12:00:00Z",
  "source": {"root": "."},
  "error": "input unavailable"
}
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	artifact, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Status != "error" || artifact.ExitCode != 2 {
		t.Fatalf("loaded artifact = %+v, want shared error result", artifact)
	}
}
