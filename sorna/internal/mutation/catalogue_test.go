package mutation

import (
	"os"
	"path/filepath"
	"testing"

	"ingen/sorna/internal/contract"
)

func TestLoadFileValidatesCatalogueAndFindsMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalogue.yaml")
	contents := []byte(`mutation_catalogue:
  schema: ingen.mutation-catalogue/v1
  id: example
  version: 1
  contract_id: document-pipeline
  contract_version: 1
  mutations:
    - id: status-200-create
      plane: implementation
      operator: response.status.replace
      target: POST /documents
      description: Return the wrong success status.
      change:
        from: 202
        to: 200
      expected_rule_ids: [document.create.valid.accepted]
      status: candidate
`)
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}

	catalogue, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	spec, ok := catalogue.Find("status-200-create")
	if !ok || spec.Operator != "response.status.replace" || spec.Change["to"] != 200 {
		t.Fatalf("found mutation = %+v, ok = %v", spec, ok)
	}
}

func TestValidateRejectsDuplicateIDsAndMissingExperimentMetadata(t *testing.T) {
	problems := Validate(Catalogue{
		Schema:          Schema,
		ID:              "example",
		Version:         1,
		ContractID:      "document-pipeline",
		ContractVersion: 1,
		Mutations: []Spec{
			{ID: "same", Plane: "implementation", Status: "candidate"},
			{ID: "same", Plane: "other", Status: "unknown"},
		},
	})
	if len(problems) < 7 {
		t.Fatalf("problems = %v, want duplicate and required-field errors", problems)
	}
}

func TestValidateAgainstContractBindsIdentityAndExpectedRules(t *testing.T) {
	catalogue := Catalogue{
		Schema:          Schema,
		ID:              "example",
		Version:         1,
		ContractID:      "document-pipeline",
		ContractVersion: 1,
		Mutations: []Spec{{
			ID:              "status-200-create",
			Plane:           "implementation",
			Operator:        "response.status.replace",
			Target:          "POST /documents",
			Description:     "Return the wrong success status.",
			Change:          map[string]any{"from": int64(202), "to": int64(200)},
			ExpectedRuleIDs: []string{"document.create.valid.accepted"},
			Status:          "candidate",
		}},
	}
	document := contract.Document{Contract: map[string]any{
		"id":      "document-pipeline",
		"version": int64(1),
		"rules":   []any{map[string]any{"id": "document.create.valid.accepted"}},
	}}
	if problems := ValidateAgainstContract(catalogue, document); len(problems) != 0 {
		t.Fatalf("binding problems = %v", problems)
	}

	catalogue.ContractID = "other-contract"
	catalogue.Mutations[0].ExpectedRuleIDs = []string{"missing-rule"}
	problems := ValidateAgainstContract(catalogue, document)
	if len(problems) != 2 {
		t.Fatalf("binding problems = %v, want contract ID and rule errors", problems)
	}
}
