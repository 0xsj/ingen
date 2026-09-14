package ciresult

import (
	"bytes"
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
