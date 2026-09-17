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
		Schema:      Schema,
		Tool:        "sorna",
		ToolVersion: "1.4.0",
		Kind:        "behavioral-verification",
		Status:      "passed",
		ExitCode:    0,
		CreatedAt:   "2026-09-14T12:00:00Z",
		Source:      Source{Root: ".artifacts/run", ModulePath: "document-pipeline"},
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
	if !strings.Contains(output.String(), `"schema": "ingen.ci-result/v1"`) || !strings.Contains(output.String(), `"tool_version": "1.4.0"`) {
		t.Fatalf("output = %s, want shared schema", output.String())
	}
}

func TestArtifactRejectsBlankToolVersion(t *testing.T) {
	artifact := Artifact{
		Schema:      Schema,
		Tool:        "sorna",
		ToolVersion: " ",
		Kind:        "behavioral-verification",
		Status:      "error",
		ExitCode:    2,
		CreatedAt:   "2026-09-14T12:00:00Z",
		Source:      Source{Root: ".artifacts/run"},
		Error:       "input unavailable",
	}
	if err := artifact.Validate(); err == nil || !strings.Contains(err.Error(), "tool_version") {
		t.Fatalf("Validate() = %v, want blank tool_version error", err)
	}
}

func TestArtifactAcceptsOptionalSourceVCS(t *testing.T) {
	artifact := Artifact{
		Schema:    Schema,
		Tool:      "sorna",
		Kind:      "behavioral-verification",
		Status:    "error",
		ExitCode:  2,
		CreatedAt: "2026-09-14T12:00:00Z",
		Source: Source{
			Root:       ".artifacts/run",
			ModulePath: "document-pipeline",
			VCS:        &VCS{System: "git", Revision: "0123456789abcdef", Dirty: true},
		},
		Error: "input unavailable",
	}
	if err := artifact.Validate(); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := WriteJSON(&output, artifact); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"vcs"`) || !strings.Contains(output.String(), `"dirty": true`) {
		t.Fatalf("output omitted source VCS provenance: %s", output.String())
	}
}

func TestArtifactRejectsIncompleteSourceVCS(t *testing.T) {
	artifact := Artifact{
		Schema:    Schema,
		Tool:      "sorna",
		Kind:      "behavioral-verification",
		Status:    "error",
		ExitCode:  2,
		CreatedAt: "2026-09-14T12:00:00Z",
		Source:    Source{Root: ".artifacts/run", VCS: &VCS{System: "git", Dirty: false}},
		Error:     "input unavailable",
	}
	if err := artifact.Validate(); err == nil || !strings.Contains(err.Error(), "source.vcs.revision") {
		t.Fatalf("Validate() = %v, want incomplete source VCS error", err)
	}
}

func TestArtifactRejectsInvalidSourceVCSChangesHash(t *testing.T) {
	artifact := Artifact{
		Schema:    Schema,
		Tool:      "sorna",
		Kind:      "behavioral-verification",
		Status:    "error",
		ExitCode:  2,
		CreatedAt: "2026-09-14T12:00:00Z",
		Source:    Source{Root: ".artifacts/run", VCS: &VCS{System: "git", Revision: "0123456789abcdef", Dirty: true, ChangesSHA256: "not-a-hash"}},
		Error:     "input unavailable",
	}
	if err := artifact.Validate(); err == nil || !strings.Contains(err.Error(), "changes_sha256") {
		t.Fatalf("Validate() = %v, want invalid source VCS changes hash error", err)
	}
}

func TestExitCodeForStatusDefinesSharedContract(t *testing.T) {
	for _, test := range []struct {
		status string
		code   int
	}{
		{status: "passed", code: 0},
		{status: "failed", code: 1},
		{status: "error", code: 2},
	} {
		t.Run(test.status, func(t *testing.T) {
			code, err := ExitCodeForStatus(test.status)
			if err != nil || code != test.code {
				t.Fatalf("ExitCodeForStatus(%q) = %d, %v; want %d, nil", test.status, code, err, test.code)
			}
		})
	}
}

func TestExitCodeForStatusRejectsUnknownStatus(t *testing.T) {
	if _, err := ExitCodeForStatus("blocked"); err == nil || !strings.Contains(err.Error(), "unsupported status") {
		t.Fatalf("ExitCodeForStatus(blocked) = %v, want unsupported-status error", err)
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

func TestSaveFileWritesValidatedEnvelope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result.json")
	artifact := Artifact{
		Schema:      Schema,
		Tool:        "test-tool",
		Kind:        "test",
		Status:      "passed",
		ExitCode:    0,
		CreatedAt:   "2026-09-14T12:00:00Z",
		Source:      Source{Root: "."},
		Report:      []byte(`{"schema":"example.report/v1"}`),
		Explanation: []byte(`{"schema":"example.explanation/v1"}`),
	}
	if err := SaveFile(path, artifact); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Tool != artifact.Tool || loaded.ToolVersion != artifact.ToolVersion || loaded.Status != artifact.Status {
		t.Fatalf("loaded artifact = %+v, want %+v", loaded, artifact)
	}
}

func TestLoadExternalProducerFixtures(t *testing.T) {
	fixtures := []struct {
		name   string
		file   string
		status string
		code   int
	}{
		{name: "failed architecture", file: "failed-architecture-result.json", status: "failed", code: 1},
		{name: "error architecture", file: "error-architecture-result.json", status: "error", code: 2},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			path := filepath.Join("testdata", fixture.file)
			artifact, err := LoadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if artifact.Tool != "example-python-architecture" || artifact.Status != fixture.status || artifact.ExitCode != fixture.code {
				t.Fatalf("loaded external artifact = %+v, want tool/status/code %s/%s/%d", artifact, "example-python-architecture", fixture.status, fixture.code)
			}
		})
	}
}
