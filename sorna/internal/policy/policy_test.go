package policy

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAndSealDocumentPipelinePolicy(t *testing.T) {
	path := filepath.Join("..", "..", "..", "examples", "document-pipeline-lab", "policy", "isolation.yaml")
	document, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := Seal(document)
	if err != nil {
		t.Fatal(err)
	}
	if sealed.SHA256 == "" || sealed.Reference().ID != "document-pipeline-oracle" || sealed.Reference().Version != 1 {
		t.Fatalf("sealed policy reference = %+v, want stable identity and hash", sealed.Reference())
	}
	if subjectID, err := SubjectID(sealed.Document); err != nil || subjectID != "document-pipeline" {
		t.Fatalf("sealed policy subject ID = %q, err = %v; want document-pipeline", subjectID, err)
	}
	if sealed.Document.Policy["status"] != "sealed" {
		t.Fatalf("sealed status = %v, want sealed", sealed.Document.Policy["status"])
	}
	if document.Policy["status"] != "draft" {
		t.Fatalf("input status = %v, want draft after immutable seal", document.Policy["status"])
	}
}

func TestValidateReportsOverlappingFilesystemPathsAndNetworkErrors(t *testing.T) {
	document := Document{Policy: map[string]any{
		"schema":      "ingen.policy/v1",
		"id":          "invalid-policy",
		"version":     int64(1),
		"status":      "draft",
		"purpose":     "test",
		"enforcement": "declared-only",
		"filesystem": map[string]any{
			"read":  []any{map[string]any{"path": "contract", "reason": "input"}},
			"write": []any{map[string]any{"path": "contract", "reason": "output"}},
			"deny":  []any{map[string]any{"path": "private", "reason": "blocked"}},
		},
		"network": map[string]any{
			"mode": "allowlist",
			"allow": []any{map[string]any{
				"host":    "127.0.0.1",
				"purpose": "test",
				"ports":   []any{int64(0), int64(70000)},
			}},
		},
		"process": map[string]any{"subject_id": "test-subject", "can_invoke_subject": "no"},
	}}
	problems := Validate(document)
	joined := strings.Join(problems, "\n")
	for _, expected := range []string{
		"policy.filesystem.write[0].path overlaps",
		"policy.network.allow[0].ports[0] must be between 1 and 65535",
		"policy.network.allow[0].ports[1] must be between 1 and 65535",
		"policy.process.can_invoke_subject must be a boolean",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("validation problems = %v, want %q", problems, expected)
		}
	}
}

func TestValidationRejectsUnknownSchema(t *testing.T) {
	document := validPolicy()
	document.Policy["schema"] = "sorna.policy/v1"

	problems := strings.Join(Validate(document), "\n")
	if !strings.Contains(problems, "policy.schema must be ingen.policy/v1") {
		t.Fatalf("validation problems = %s, want policy schema identity error", problems)
	}
}

func TestSealRejectsNonDraftPolicy(t *testing.T) {
	document := validPolicy()
	document.Policy["status"] = "sealed"
	if _, err := Seal(document); err == nil || !strings.Contains(err.Error(), "only draft policies") {
		t.Fatalf("Seal() error = %v, want non-draft error", err)
	}
}

func TestValidateRequiresProcessSubjectIdentity(t *testing.T) {
	document := validPolicy()
	delete(document.Policy["process"].(map[string]any), "subject_id")
	problems := strings.Join(Validate(document), "\n")
	if !strings.Contains(problems, "policy.process.subject_id must be a non-empty string") {
		t.Fatalf("validation problems = %s, want missing subject identity", problems)
	}
}

func validPolicy() Document {
	return Document{Policy: map[string]any{
		"schema":      "ingen.policy/v1",
		"id":          "policy-test",
		"version":     int64(1),
		"status":      "draft",
		"purpose":     "test",
		"enforcement": "declared-only",
		"filesystem": map[string]any{
			"read":  []any{map[string]any{"path": "contract", "reason": "input"}},
			"write": []any{map[string]any{"path": "output", "reason": "output"}},
			"deny":  []any{map[string]any{"path": "implementation", "reason": "blocked"}},
		},
		"network": map[string]any{"mode": "disabled"},
		"process": map[string]any{"subject_id": "test-subject", "can_invoke_subject": false},
	}}
}
