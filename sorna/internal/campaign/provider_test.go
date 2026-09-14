package campaign

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/sorna/internal/mutation"
)

func TestLoadProviderAndResolveRuntimeTokens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider.yaml")
	contents := []byte(`mutation_provider:
  schema: ingen.mutation-provider/v1
  id: fixtures
  version: 1
  plan_schema: ingen.mutation-plan/v1
  capabilities:
    - plane: implementation
      operator: response.status.replace
      target: POST /documents
  entries:
    - mutation_id: status-200-create
      command: .artifacts/document-pipeline-subject/document-pipeline-defect
      args: [-addr, "${SORA_ADDR}"]
      subject_root: .
      variant: status-200-create
`)
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}
	provider, err := LoadProviderFile(path)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := provider.Resolve("status-200-create", "127.0.0.1:8081", "http://127.0.0.1:8081")
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Command) != 3 || prepared.Command[2] != "127.0.0.1:8081" || prepared.Variant != "status-200-create" {
		t.Fatalf("prepared = %+v, want expanded argv", prepared)
	}
}

func TestValidateProviderForPlanRequiresEveryMutation(t *testing.T) {
	provider := ProviderManifest{
		Schema:     ProviderSchema,
		ID:         "fixtures",
		Version:    1,
		PlanSchema: Schema,
		Capabilities: []ProviderCapability{{
			Plane: "implementation", Operator: "response.status.replace", Target: "POST /documents",
		}},
		Entries: []ProviderEntry{{
			MutationID: "other",
			Command:    "subject",
		}},
	}
	plan := Plan{Mutations: []MutationEntry{{Sequence: 1, Spec: mutationSpec("status-200-create")}}}
	problems := provider.ValidateForPlan(plan)
	if len(problems) != 1 {
		t.Fatalf("problems = %v, want missing provider entry", problems)
	}
}

func TestValidateProviderForPlanRequiresDeclaredCapability(t *testing.T) {
	provider := ProviderManifest{
		Schema:     ProviderSchema,
		ID:         "fixtures",
		Version:    1,
		PlanSchema: Schema,
		Capabilities: []ProviderCapability{{
			Plane: "implementation", Operator: "response.status.replace", Target: "POST /documents",
		}},
		Entries: []ProviderEntry{{MutationID: "m1", Command: "subject"}},
	}
	plan := Plan{Mutations: []MutationEntry{{Sequence: 1, Spec: mutation.Spec{
		ID: "m1", Plane: "implementation", Operator: "response.field.remove", Target: "POST /documents",
	}}}}
	problems := provider.ValidateForPlan(plan)
	if len(problems) != 1 || !strings.Contains(problems[0], "undeclared provider capability") {
		t.Fatalf("problems = %v, want undeclared capability", problems)
	}
}

func TestVerifyPreparedSubjectRejectsSourceOrBinaryDrift(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "source"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin", "subject"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	sourceHash, err := HashTree(filepath.Join(root, "source"))
	if err != nil {
		t.Fatal(err)
	}
	binaryHash, err := HashFile(filepath.Join(root, "bin", "subject"))
	if err != nil {
		t.Fatal(err)
	}
	prepared := PreparedSubject{
		Command:     []string{"bin/subject"},
		SubjectRoot: root,
		Provenance: &ProviderProvenance{
			SourceDir:    "source",
			SourceSHA256: sourceHash,
			BinarySHA256: binaryHash,
			Location:     "source/main.go",
			Before:       "old",
			After:        "new",
		},
	}
	if err := VerifyPreparedSubject(prepared); err != nil {
		t.Fatalf("VerifyPreparedSubject() = %v, want success", err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin", "subject"), []byte("drifted"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := VerifyPreparedSubject(prepared); err == nil || !strings.Contains(err.Error(), "binary hash") {
		t.Fatalf("VerifyPreparedSubject() = %v, want binary hash mismatch", err)
	}
}

func mutationSpec(id string) mutation.Spec {
	return mutation.Spec{ID: id, Plane: "implementation", Operator: "response.status.replace", Target: "POST /documents"}
}
