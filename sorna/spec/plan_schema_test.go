package spec

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"ingen/sorna/internal/campaign"
	"ingen/sorna/internal/mutation"
	"ingen/sorna/internal/oracle"
	"ingen/sorna/internal/runner"
)

func TestMutationPlanSchemaAcceptsProducedPlan(t *testing.T) {
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() did not return the schema test path")
	}
	schemaPath := filepath.Join(filepath.Dir(sourcePath), "ingen.mutation-plan-v1.schema.json")
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(schemaPath, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}

	contractHash := strings.Repeat("a", 64)
	oracleHash := strings.Repeat("b", 64)
	plan := campaign.Plan{
		Schema: campaign.Schema,
		Status: "ready",
		Catalogue: campaign.CatalogueReference{
			Path: "mutations.yaml", ID: "document-pipeline-mutations", Version: 1, SHA256: strings.Repeat("c", 64),
		},
		Contract: runner.ContractReference{ID: "document-pipeline", Version: 1, SHA256: contractHash},
		Oracle:   runner.OracleReference{Schema: oracle.Schema, SHA256: oracleHash},
		Baseline: runner.BaselineReference{
			EvidencePath: ".artifacts/baseline",
			RunID:        "baseline-run",
			Contract:     runner.ContractReference{ID: "document-pipeline", Version: 1, SHA256: contractHash},
			Oracle:       &runner.OracleReference{Schema: oracle.Schema, SHA256: oracleHash},
		},
		OraclePolicySHA256:  strings.Repeat("d", 64),
		SubjectPolicySHA256: strings.Repeat("e", 64),
		Mutations: []campaign.MutationEntry{{
			Sequence: 1,
			Spec: mutation.Spec{
				ID:              "status-200-create",
				Plane:           "implementation",
				Operator:        "response.status.replace",
				Target:          "POST /documents",
				Description:     "Change the accepted response status.",
				Change:          map[string]any{"from": 202, "to": 200},
				ExpectedRuleIDs: []string{"document.create.valid.accepted"},
				Status:          "candidate",
			},
		}},
	}
	contents, err := campaign.CanonicalJSON(plan)
	if err != nil {
		t.Fatal(err)
	}
	var instance any
	if err := json.Unmarshal(contents, &instance); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(instance); err != nil {
		t.Fatalf("produced mutation plan does not match published schema: %v", err)
	}
}
