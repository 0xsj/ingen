package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/sorna/internal/mutation"
)

func TestMutateDocumentPipelineChangesOnlyTargetStatus(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "examples", "document-pipeline-lab", "subject")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `package documentpipeline

import "net/http"

func createDocument(w http.ResponseWriter) {
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
}
`
	if err := os.WriteFile(filepath.Join(path, "server.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := mutation.Spec{
		ID: "status-200-create", Plane: "implementation", Operator: "response.status.replace", Target: "POST /documents",
		Description: "Return 200 instead of 202", Change: map[string]any{"from": 202, "to": 200}, ExpectedRuleIDs: []string{"rule"}, Status: "candidate",
	}
	if err := mutateDocumentPipeline(root, spec); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(path, "server.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "http.StatusOK") || strings.Contains(string(contents), "http.StatusAccepted") {
		t.Fatalf("mutated source = %s, want only accepted status replaced", contents)
	}
}

func TestMutateDocumentPipelineRejectsUnsupportedChange(t *testing.T) {
	err := mutateDocumentPipeline(t.TempDir(), mutation.Spec{
		ID: "unsupported", Plane: "implementation", Operator: "response.status.replace", Target: "POST /documents",
		Change: map[string]any{"from": 201, "to": 200},
	})
	if err == nil || !strings.Contains(err.Error(), "only status 202 -> 200") {
		t.Fatalf("mutateDocumentPipeline() = %v, want unsupported change error", err)
	}
}
