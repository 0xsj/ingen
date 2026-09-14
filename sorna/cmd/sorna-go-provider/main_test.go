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
	if _, err := mutateDocumentPipeline(root, spec); err != nil {
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
	_, err := mutateDocumentPipeline(t.TempDir(), mutation.Spec{
		ID: "unsupported", Plane: "implementation", Operator: "response.status.replace", Target: "POST /documents",
		Change: map[string]any{"from": 201, "to": 200},
	})
	if err == nil || !strings.Contains(err.Error(), "only status 202 -> 200") {
		t.Fatalf("mutateDocumentPipeline() = %v, want unsupported change error", err)
	}
}

func TestMutateDocumentPipelineRemovesOnlyNamedResponseField(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "examples", "document-pipeline-lab", "subject")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `package documentpipeline

import "net/http"

func createDocument(w http.ResponseWriter) {
	writeJSON(w, http.StatusAccepted, map[string]string{"id": "doc-1", "name": "welcome.md", "status": "queued"})
}
`
	if err := os.WriteFile(filepath.Join(path, "server.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := mutateDocumentPipeline(root, mutation.Spec{
		ID: "remove-name-create", Plane: "implementation", Operator: "response.field.remove", Target: "POST /documents",
		Change: map[string]any{"field": "name"},
	})
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(path, "server.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), `"name":`) || !strings.Contains(string(contents), `"id":`) || !strings.Contains(string(contents), `"status":`) {
		t.Fatalf("mutated source = %s, want only name field removed", contents)
	}
}

func TestMutateDocumentPipelineRejectsAmbiguousStatusTarget(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "examples", "document-pipeline-lab", "subject")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `package documentpipeline

import "net/http"

func createDocument(w http.ResponseWriter) {
	writeJSON(w, http.StatusAccepted, map[string]string{"id": "one"})
	writeJSON(w, http.StatusAccepted, map[string]string{"id": "two"})
}
`
	if err := os.WriteFile(filepath.Join(path, "server.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := mutateDocumentPipeline(root, mutation.Spec{
		ID: "status-200-create", Plane: "implementation", Operator: "response.status.replace", Target: "POST /documents",
		Change: map[string]any{"from": 202, "to": 200},
	})
	if err == nil || !strings.Contains(err.Error(), "found 2") {
		t.Fatalf("mutateDocumentPipeline() = %v, want ambiguous-target error", err)
	}
	contents, readErr := os.ReadFile(filepath.Join(path, "server.go"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if strings.Contains(string(contents), "StatusOK") {
		t.Fatalf("ambiguous mutation changed source = %s, want no write", contents)
	}
}

func TestMutateDocumentPipelineRejectsMissingFieldTarget(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "examples", "document-pipeline-lab", "subject")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `package documentpipeline

import "net/http"

func createDocument(w http.ResponseWriter) {
	writeJSON(w, http.StatusAccepted, map[string]string{"id": "one"})
}
`
	if err := os.WriteFile(filepath.Join(path, "server.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := mutateDocumentPipeline(root, mutation.Spec{
		ID: "remove-name-create", Plane: "implementation", Operator: "response.field.remove", Target: "POST /documents",
		Change: map[string]any{"field": "name"},
	})
	if err == nil || !strings.Contains(err.Error(), "found 0") {
		t.Fatalf("mutateDocumentPipeline() = %v, want missing-target error", err)
	}
}

func TestMutateDocumentPipelineRejectsUnsupportedOperator(t *testing.T) {
	_, err := mutateDocumentPipeline(t.TempDir(), mutation.Spec{
		ID: "unsupported", Plane: "implementation", Operator: "response.field.type", Target: "POST /documents",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported document mutation") {
		t.Fatalf("mutateDocumentPipeline() = %v, want unsupported operator error", err)
	}
}
