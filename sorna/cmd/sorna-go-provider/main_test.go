package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/sorna/internal/campaign"
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
	provenance, err := mutateDocumentPipeline(root, spec)
	if err != nil {
		t.Fatal(err)
	}
	if provenance.TargetResolution == nil || provenance.TargetResolution.CandidateCount != 1 || provenance.TargetResolution.AppliedCount != 1 {
		t.Fatalf("target resolution = %+v, want exactly one candidate and applied target", provenance.TargetResolution)
	}
	contents, err := os.ReadFile(filepath.Join(path, "server.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "http.StatusOK") || strings.Contains(string(contents), "http.StatusAccepted") {
		t.Fatalf("mutated source = %s, want only accepted status replaced", contents)
	}
}

func TestMutateDocumentPipelineSupportsEventAwareCreateResponse(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "examples", "document-pipeline-lab", "subject")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `package documentpipeline

import "net/http"

func createDocument(w http.ResponseWriter) {
	writeJSONWithEvents(w, http.StatusAccepted, map[string]string{"id": "doc-1", "name": "welcome.md", "status": "queued"}, "document.accepted")
}
`
	if err := os.WriteFile(filepath.Join(path, "server.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	statusProvenance, err := mutateDocumentPipeline(root, mutation.Spec{
		ID: "status-200-create", Plane: "implementation", Operator: "response.status.replace", Target: "POST /documents",
		Change: map[string]any{"from": 202, "to": 200},
	})
	if err != nil {
		t.Fatal(err)
	}
	if statusProvenance.TargetResolution == nil || statusProvenance.TargetResolution.CandidateCount != 1 || statusProvenance.TargetResolution.AppliedCount != 1 {
		t.Fatalf("status target resolution = %+v, want exactly one candidate and applied target", statusProvenance.TargetResolution)
	}
	if !strings.Contains(statusProvenance.Location, "writeJSONWithEvents.status") {
		t.Fatalf("status provenance location = %q, want event-aware writer", statusProvenance.Location)
	}

	contents, err := os.ReadFile(filepath.Join(path, "server.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "writeJSONWithEvents(w, http.StatusOK") || strings.Contains(string(contents), "http.StatusAccepted") {
		t.Fatalf("mutated event-aware source = %s, want accepted status replaced", contents)
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
	provenance, err := mutateDocumentPipeline(root, mutation.Spec{
		ID: "remove-name-create", Plane: "implementation", Operator: "response.field.remove", Target: "POST /documents",
		Change: map[string]any{"field": "name"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if provenance.TargetResolution == nil || provenance.TargetResolution.CandidateCount != 1 || provenance.TargetResolution.AppliedCount != 1 {
		t.Fatalf("target resolution = %+v, want exactly one candidate and applied target", provenance.TargetResolution)
	}
	contents, err := os.ReadFile(filepath.Join(path, "server.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), `"name":`) || !strings.Contains(string(contents), `"id":`) || !strings.Contains(string(contents), `"status":`) {
		t.Fatalf("mutated source = %s, want only name field removed", contents)
	}
}

func TestMutateDocumentPipelineAddsOnlyNamedResponseField(t *testing.T) {
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
	provenance, err := mutateDocumentPipeline(root, mutation.Spec{
		ID: "add-debug-create", Plane: "implementation", Operator: "response.field.add", Target: "POST /documents",
		Change: map[string]any{"field": "debug", "value": "mutation"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if provenance.TargetResolution == nil || provenance.TargetResolution.CandidateCount != 1 || provenance.TargetResolution.AppliedCount != 1 {
		t.Fatalf("target resolution = %+v, want exactly one candidate and applied target", provenance.TargetResolution)
	}
	contents, err := os.ReadFile(filepath.Join(path, "server.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), `"debug": "mutation"`) || !strings.Contains(string(contents), `"name": "welcome.md"`) {
		t.Fatalf("mutated source = %s, want debug field added without removing name", contents)
	}
}

func TestMutateDocumentPipelineChangesOnlyNamedErrorStatus(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "examples", "document-pipeline-lab", "subject")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `package documentpipeline

import "net/http"

func createDocument(w http.ResponseWriter) {
	writeError(w, http.StatusBadRequest, "invalid_json")
	writeError(w, http.StatusBadRequest, "unsupported_document_type")
}
`
	if err := os.WriteFile(filepath.Join(path, "server.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	provenance, err := mutateDocumentPipeline(root, mutation.Spec{
		ID: "unsupported-type-500", Plane: "implementation", Operator: "response.error.status.replace", Target: "POST /documents",
		Change: map[string]any{"code": "unsupported_document_type", "from": 400, "to": 500},
	})
	if err != nil {
		t.Fatal(err)
	}
	if provenance.TargetResolution == nil || provenance.TargetResolution.CandidateCount != 1 || provenance.TargetResolution.AppliedCount != 1 {
		t.Fatalf("target resolution = %+v, want exactly one candidate and applied target", provenance.TargetResolution)
	}
	contents, err := os.ReadFile(filepath.Join(path, "server.go"))
	if err != nil {
		t.Fatal(err)
	}
	mutated := string(contents)
	if !strings.Contains(mutated, `writeError(w, http.StatusBadRequest, "invalid_json")`) || !strings.Contains(mutated, `writeError(w, http.StatusInternalServerError, "unsupported_document_type")`) {
		t.Fatalf("mutated source = %s, want only named error status replaced", contents)
	}
}

func TestMutateDocumentPipelineChangesOnlyNamedStateTransition(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "examples", "document-pipeline-lab", "subject")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `package documentpipeline

func processDocument(doc *document) {
	doc.Status = "queued"
	doc.Status = "completed"
}
`
	if err := os.WriteFile(filepath.Join(path, "server.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	provenance, err := mutateDocumentPipeline(root, mutation.Spec{
		ID: "process-stays-queued", Plane: "implementation", Operator: "state.transition.replace", Target: "POST /documents/{id}/process",
		Change: map[string]any{"from": "completed", "to": "queued"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if provenance.TargetResolution == nil || provenance.TargetResolution.CandidateCount != 1 || provenance.TargetResolution.AppliedCount != 1 {
		t.Fatalf("target resolution = %+v, want exactly one candidate and applied target", provenance.TargetResolution)
	}
	contents, err := os.ReadFile(filepath.Join(path, "server.go"))
	if err != nil {
		t.Fatal(err)
	}
	mutated := string(contents)
	if !strings.Contains(mutated, `doc.Status = "queued"`) || strings.Count(mutated, `doc.Status = "queued"`) != 2 || strings.Contains(mutated, `doc.Status = "completed"`) {
		t.Fatalf("mutated source = %s, want only completed transition replaced", contents)
	}
}

func TestMutateDocumentPipelineChangesOnlyNamedPersistenceKey(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "examples", "document-pipeline-lab", "subject")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `package documentpipeline

func createDocument(h *handler, id string) {
	h.store.docs[id] = &document{ID: id}
	h.store.docs["other"] = &document{ID: "other"}
}
`
	if err := os.WriteFile(filepath.Join(path, "server.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	provenance, err := mutateDocumentPipeline(root, mutation.Spec{
		ID: "persistence-under-wrong-key", Plane: "implementation", Operator: "state.persistence.key.replace", Target: "POST /documents",
		Change: map[string]any{"from": "id", "to": "mutation-discarded"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if provenance.TargetResolution == nil || provenance.TargetResolution.CandidateCount != 1 || provenance.TargetResolution.AppliedCount != 1 {
		t.Fatalf("target resolution = %+v, want exactly one candidate and applied target", provenance.TargetResolution)
	}
	contents, err := os.ReadFile(filepath.Join(path, "server.go"))
	if err != nil {
		t.Fatal(err)
	}
	mutated := string(contents)
	if !strings.Contains(mutated, `h.store.docs["mutation-discarded"] = &document{ID: id}`) || !strings.Contains(mutated, `h.store.docs["other"] = &document{ID: "other"}`) || strings.Contains(mutated, "h.store.docs[id]") {
		t.Fatalf("mutated source = %s, want only accepted document key replaced", contents)
	}
}

func TestMutateDocumentPipelineChangesOnlyNamedValidationSuffix(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "examples", "document-pipeline-lab", "subject")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `package documentpipeline

func createDocument(input createRequest) bool {
	if input.Name == "" || (extension != ".md" && extension != ".txt") {
		return false
	}
	use(extension, ".txt")
	return false
}
`
	if err := os.WriteFile(filepath.Join(path, "server.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	provenance, err := mutateDocumentPipeline(root, mutation.Spec{
		ID: "accepts-png", Plane: "implementation", Operator: "input.validation.suffix.add", Target: "POST /documents",
		Change: map[string]any{"suffix": ".png"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if provenance.TargetResolution == nil || provenance.TargetResolution.CandidateCount != 1 || provenance.TargetResolution.AppliedCount != 1 {
		t.Fatalf("target resolution = %+v, want exactly one candidate and applied target", provenance.TargetResolution)
	}
	contents, err := os.ReadFile(filepath.Join(path, "server.go"))
	if err != nil {
		t.Fatal(err)
	}
	mutated := string(contents)
	if !strings.Contains(mutated, `extension != ".md" && extension != ".txt" && extension != ".png"`) || !strings.Contains(mutated, `use(extension, ".txt")`) || strings.Count(mutated, `extension != ".png"`) != 1 || !strings.Contains(mutated, `extension != ".txt"`) {
		t.Fatalf("mutated source = %s, want PNG added without removing TXT", contents)
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
	spec := mutation.Spec{
		ID: "status-200-create", Plane: "implementation", Operator: "response.status.replace", Target: "POST /documents",
		Change: map[string]any{"from": 202, "to": 200},
	}
	_, err := mutateDocumentPipeline(root, spec)
	var resolutionErr *campaign.TargetResolutionError
	if err == nil || !errors.As(err, &resolutionErr) {
		t.Fatalf("mutateDocumentPipeline() = %v, want target-resolution error", err)
	}
	if resolutionErr.Resolution.CandidateCount != 2 || resolutionErr.Resolution.AppliedCount != 0 {
		t.Fatalf("resolution error = %+v, want two candidates and no applied targets", resolutionErr)
	}
	encoded, marshalErr := json.Marshal(resolutionErr)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	var decoded campaign.TargetResolutionError
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Schema != campaign.TargetResolutionErrorSchema || decoded.Status != "blocked" || decoded.Resolution.CandidateCount != 2 {
		t.Fatalf("decoded resolution error = %+v, want versioned blocked diagnostic", decoded)
	}
	if !strings.Contains(err.Error(), "found 2") {
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
	spec := mutation.Spec{
		ID: "remove-name-create", Plane: "implementation", Operator: "response.field.remove", Target: "POST /documents",
		Change: map[string]any{"field": "name"},
	}
	_, err := mutateDocumentPipeline(root, spec)
	var resolutionErr *campaign.TargetResolutionError
	if err == nil || !errors.As(err, &resolutionErr) {
		t.Fatalf("mutateDocumentPipeline() = %v, want target-resolution error", err)
	}
	if resolutionErr.Resolution.CandidateCount != 0 || resolutionErr.Resolution.AppliedCount != 0 {
		t.Fatalf("resolution error = %+v, want zero candidates and no applied targets", resolutionErr)
	}
	if !strings.Contains(err.Error(), "found 0") {
		t.Fatalf("mutateDocumentPipeline() = %v, want missing-target error", err)
	}
}

func TestMutateDocumentPipelineRejectsAmbiguousStateTransitionTarget(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "examples", "document-pipeline-lab", "subject")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `package documentpipeline

func processDocument(doc *document) {
	doc.Status = "completed"
	doc.Status = "completed"
}
`
	serverPath := filepath.Join(path, "server.go")
	if err := os.WriteFile(serverPath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := mutateDocumentPipeline(root, mutation.Spec{
		ID: "process-stays-queued", Plane: "implementation", Operator: "state.transition.replace", Target: "POST /documents/{id}/process",
		Change: map[string]any{"from": "completed", "to": "queued"},
	})
	var resolutionErr *campaign.TargetResolutionError
	if err == nil || !errors.As(err, &resolutionErr) {
		t.Fatalf("mutateDocumentPipeline() = %v, want target-resolution error", err)
	}
	if resolutionErr.Resolution.CandidateCount != 2 || resolutionErr.Resolution.AppliedCount != 0 {
		t.Fatalf("resolution error = %+v, want two candidates and no applied targets", resolutionErr)
	}
	contents, readErr := os.ReadFile(serverPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(contents) != source {
		t.Fatalf("ambiguous mutation changed source = %s, want no write", contents)
	}
}

func TestMutateDocumentPipelineRejectsMissingPersistenceKeyTarget(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "examples", "document-pipeline-lab", "subject")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `package documentpipeline

func createDocument(h *handler, id string) {
	h.store.docs["other"] = &document{ID: "other"}
}
`
	serverPath := filepath.Join(path, "server.go")
	if err := os.WriteFile(serverPath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := mutateDocumentPipeline(root, mutation.Spec{
		ID: "persistence-under-wrong-key", Plane: "implementation", Operator: "state.persistence.key.replace", Target: "POST /documents",
		Change: map[string]any{"from": "id", "to": "mutation-discarded"},
	})
	var resolutionErr *campaign.TargetResolutionError
	if err == nil || !errors.As(err, &resolutionErr) {
		t.Fatalf("mutateDocumentPipeline() = %v, want target-resolution error", err)
	}
	if resolutionErr.Resolution.CandidateCount != 0 || resolutionErr.Resolution.AppliedCount != 0 {
		t.Fatalf("resolution error = %+v, want zero candidates and no applied targets", resolutionErr)
	}
	contents, readErr := os.ReadFile(serverPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(contents) != source {
		t.Fatalf("missing-target mutation changed source = %s, want no write", contents)
	}
}

func TestMutateDocumentPipelineRejectsUnsupportedValidationSuffix(t *testing.T) {
	_, err := mutateDocumentPipeline(t.TempDir(), mutation.Spec{
		ID: "accepts-gif", Plane: "implementation", Operator: "input.validation.suffix.add", Target: "POST /documents",
		Change: map[string]any{"suffix": ".gif"},
	})
	if err == nil || !strings.Contains(err.Error(), "only adding validation suffix .png") {
		t.Fatalf("mutateDocumentPipeline() = %v, want unsupported suffix error", err)
	}
}

func TestMutateDocumentPipelineRejectsMissingValidationSuffix(t *testing.T) {
	_, err := mutateDocumentPipeline(t.TempDir(), mutation.Spec{
		ID: "accepts-png", Plane: "implementation", Operator: "input.validation.suffix.add", Target: "POST /documents",
	})
	if err == nil || !strings.Contains(err.Error(), "missing change.suffix") {
		t.Fatalf("mutateDocumentPipeline() = %v, want missing suffix error", err)
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

func TestMutateWebhookValidationDisablesOnlyDuplicateGuard(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "examples", "webhook-validation-lab", "subject")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `package webhookvalidation

func receiveEvent() {
	if _, exists := accepted["event"]; exists {
		return
	}
	accepted["event"] = struct{}{}
}
`
	serverPath := filepath.Join(path, "server.go")
	if err := os.WriteFile(serverPath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	provenance, err := mutateWebhookValidation(root, mutation.Spec{
		ID: "accepts-duplicate", Plane: "implementation", Operator: "state.idempotency.disable", Target: "POST /webhooks/events",
		Change: map[string]any{"from": "reject-duplicate", "to": "accept-duplicate"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if provenance.TargetResolution == nil || provenance.TargetResolution.CandidateCount != 1 || provenance.TargetResolution.AppliedCount != 1 {
		t.Fatalf("target resolution = %+v, want exactly one candidate and applied target", provenance.TargetResolution)
	}
	contents, err := os.ReadFile(serverPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "if _, exists := accepted[\"event\"]; exists && false") || !strings.Contains(string(contents), `accepted["event"] = struct{}{}`) {
		t.Fatalf("mutated source = %s, want only duplicate guard disabled", contents)
	}
}

func TestMutateWebhookValidationRejectsAmbiguousDuplicateGuard(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "examples", "webhook-validation-lab", "subject")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `package webhookvalidation

func receiveEvent() {
	if _, exists := accepted["first"]; exists {
		return
	}
	if _, exists := accepted["second"]; exists {
		return
	}
}
`
	serverPath := filepath.Join(path, "server.go")
	if err := os.WriteFile(serverPath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := mutateWebhookValidation(root, mutation.Spec{
		ID: "accepts-duplicate", Plane: "implementation", Operator: "state.idempotency.disable", Target: "POST /webhooks/events",
		Change: map[string]any{"from": "reject-duplicate", "to": "accept-duplicate"},
	})
	var resolutionErr *campaign.TargetResolutionError
	if err == nil || !errors.As(err, &resolutionErr) {
		t.Fatalf("mutateWebhookValidation() = %v, want target-resolution error", err)
	}
	if resolutionErr.Resolution.CandidateCount != 2 || resolutionErr.Resolution.AppliedCount != 0 {
		t.Fatalf("resolution error = %+v, want two candidates and no applied targets", resolutionErr)
	}
	contents, readErr := os.ReadFile(serverPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(contents) != source {
		t.Fatalf("ambiguous mutation changed source = %s, want no write", contents)
	}
}
