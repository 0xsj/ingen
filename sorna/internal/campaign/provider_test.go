package campaign

import (
	"os"
	"path/filepath"
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

func mutationSpec(id string) mutation.Spec {
	return mutation.Spec{ID: id}
}
