package campaign

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/sorna/internal/contract"
	"ingen/sorna/internal/mutation"
	"ingen/sorna/internal/oracle"
	"ingen/sorna/internal/runner"
)

func TestBuildWritesAndReloadsCanonicalPlan(t *testing.T) {
	contractHash := strings.Repeat("a", 64)
	policyHash := strings.Repeat("b", 64)
	oracleArtifact := oracle.Artifact{
		Schema:       oracle.Schema,
		Status:       "frozen",
		Contract:     oracle.ContractReference{ID: "document-pipeline", Version: 1, SHA256: contractHash},
		PolicySHA256: policyHash,
		Cases: []oracle.Case{{
			CaseID:   "case-0001",
			RuleID:   "document.create.valid.accepted",
			Strength: "must",
		}},
	}
	oracleHash, err := oracle.Hash(oracleArtifact)
	if err != nil {
		t.Fatal(err)
	}
	oracleReference := &runner.OracleReference{Schema: oracle.Schema, SHA256: oracleHash}
	request := BuildRequest{
		CataloguePath:   "examples/document-pipeline-lab/mutations/catalogue.yaml",
		CatalogueSHA256: strings.Repeat("c", 64),
		Catalogue: mutation.Catalogue{
			Schema:          mutation.Schema,
			ID:              "document-pipeline-mutations",
			Version:         1,
			ContractID:      "document-pipeline",
			ContractVersion: 1,
			Mutations: []mutation.Spec{{
				ID:              "status-200-create",
				Plane:           "implementation",
				Operator:        "response.status.replace",
				Target:          "POST /documents",
				Description:     "Return the wrong success status.",
				Change:          map[string]any{"from": int64(202), "to": int64(200)},
				ExpectedRuleIDs: []string{"document.create.valid.accepted"},
				Status:          "candidate",
			}},
		},
		Contract: contract.Document{Contract: map[string]any{
			"id":      "document-pipeline",
			"version": int64(1),
			"rules":   []any{map[string]any{"id": "document.create.valid.accepted"}},
		}},
		ContractSHA256: contractHash,
		Oracle:         oracleArtifact,
		Baseline: runner.BaselineReference{
			EvidencePath: ".artifacts/document-pipeline-run",
			RunID:        "run-clean",
			Contract:     runner.ContractReference{ID: "document-pipeline", Version: 1, SHA256: contractHash},
			Oracle:       oracleReference,
		},
		SubjectPolicySHA256: strings.Repeat("d", 64),
	}

	plan, err := Build(request)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != "ready" || len(plan.Mutations) != 1 || plan.Mutations[0].Sequence != 1 {
		t.Fatalf("plan = %+v, want ready plan with one ordered mutation", plan)
	}

	path := filepath.Join(t.TempDir(), "plan.json")
	if _, err := WriteFile(path, plan); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Catalogue.SHA256 != plan.Catalogue.SHA256 || loaded.Baseline.RunID != "run-clean" {
		t.Fatalf("loaded plan = %+v, want preserved references", loaded)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestBuildRejectsBaselineThatDoesNotMatchOracle(t *testing.T) {
	oracleArtifact := oracle.Artifact{
		Schema:       oracle.Schema,
		Status:       "frozen",
		Contract:     oracle.ContractReference{ID: "document-pipeline", Version: 1, SHA256: strings.Repeat("a", 64)},
		PolicySHA256: strings.Repeat("b", 64),
		Cases:        []oracle.Case{{CaseID: "case-0001", RuleID: "rule", Strength: "must"}},
	}
	request := BuildRequest{
		CataloguePath:   "catalogue.yaml",
		CatalogueSHA256: strings.Repeat("c", 64),
		Catalogue: mutation.Catalogue{
			Schema:          mutation.Schema,
			ID:              "catalogue",
			Version:         1,
			ContractID:      "document-pipeline",
			ContractVersion: 1,
			Mutations: []mutation.Spec{{
				ID:              "m1",
				Plane:           "implementation",
				Operator:        "response.status.replace",
				Target:          "POST /documents",
				Description:     "change",
				Change:          map[string]any{"from": 1, "to": 2},
				ExpectedRuleIDs: []string{"rule"},
				Status:          "candidate",
			}},
		},
		Contract: contract.Document{Contract: map[string]any{
			"id": "document-pipeline", "version": int64(1), "rules": []any{map[string]any{"id": "rule"}},
		}},
		ContractSHA256: strings.Repeat("a", 64),
		Oracle:         oracleArtifact,
		Baseline: runner.BaselineReference{
			EvidencePath: "baseline",
			RunID:        "run-clean",
			Contract:     runner.ContractReference{ID: "document-pipeline", Version: 1, SHA256: strings.Repeat("a", 64)},
			Oracle:       &runner.OracleReference{Schema: oracle.Schema, SHA256: strings.Repeat("d", 64)},
		},
	}
	if _, err := Build(request); err == nil || !strings.Contains(err.Error(), "baseline oracle") {
		t.Fatalf("Build() = %v, want baseline oracle mismatch", err)
	}
}
