package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFileAcceptsYAMLAndDefaultsChecksToRequired(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workflow.yaml")
	contents := `schema: ingen.nublar-workflow/v1
id: document-pipeline-ci
checks:
  - id: behavior
    tool: sorna
    result: artifacts/sorna.json
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	document, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if document.ID != "document-pipeline-ci" || len(document.Checks) != 1 || !document.Checks[0].IsRequired() {
		t.Fatalf("document = %+v, want one required behavior check", document)
	}
}

func TestValidateAcceptsExplicitOptionalCheck(t *testing.T) {
	optional := false
	document := Document{
		Schema: Schema,
		ID:     "optional-check-workflow",
		Checks: []Check{{ID: "architecture", Tool: "paddock", Result: "paddock.json", Required: &optional}},
	}
	if err := Validate(document); err != nil {
		t.Fatal(err)
	}
	if document.Checks[0].IsRequired() {
		t.Fatal("optional check was treated as required")
	}
}

func TestValidateRejectsUnsafeOrDuplicateChecks(t *testing.T) {
	tests := []Document{
		{Schema: Schema, Checks: []Check{{ID: "same", Tool: "sorna", Result: "one.json"}, {ID: "same", Tool: "paddock", Result: "two.json"}}},
		{Schema: Schema, Checks: []Check{{ID: "unsafe", Tool: "sorna", Result: "../sorna.json"}}},
		{Schema: Schema, Checks: []Check{{ID: "one", Tool: "sorna", Result: "nested/../same.json"}, {ID: "two", Tool: "sorna", Result: "same.json"}}},
	}
	for _, document := range tests {
		if err := Validate(document); err == nil || !strings.Contains(err.Error(), "workflow") {
			t.Fatalf("Validate(%+v) = %v, want workflow validation error", document, err)
		}
	}
}

func TestLoadFileRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workflow.yaml")
	contents := `schema: ingen.nublar-workflow/v1
checks:
  - id: behavior
    tool: sorna
    result: sorna.json
    requierd: true
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path); err == nil || !strings.Contains(err.Error(), "field requierd not found") {
		t.Fatalf("LoadFile() = %v, want unknown-field error", err)
	}
}

func TestLoadFileRejectsExecutionFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workflow.yaml")
	contents := `schema: ingen.nublar-workflow/v1
id: execution-attempt
checks:
  - id: behavior
    tool: sorna
    result: sorna.json
    command: go test ./...
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path); err == nil || !strings.Contains(err.Error(), "field command not found") {
		t.Fatalf("LoadFile() = %v, want execution-field rejection", err)
	}
}
