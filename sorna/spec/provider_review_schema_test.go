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
	"ingen/sorna/internal/evidence"
)

func TestProviderReviewSchemaAcceptsProducedReport(t *testing.T) {
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() did not return the schema test path")
	}
	schemaPath := filepath.Join(filepath.Dir(sourcePath), "ingen.mutation-provider-review-v1.schema.json")
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

	review := campaign.ProviderReview{
		Schema: campaign.ProviderReviewSchema,
		Status: "ready",
		Plan: campaign.ProviderReviewPlan{
			Path:           "plan.json",
			SHA256:         strings.Repeat("a", 64),
			SemanticSHA256: strings.Repeat("b", 64),
		},
		Provider: campaign.ProviderReviewProvider{
			Path:            "provider.json",
			SHA256:          strings.Repeat("c", 64),
			ID:              "provider",
			Version:         1,
			PlanSHA256:      strings.Repeat("a", 64),
			PlanBinding:     "matched",
			SemanticBinding: "unbound",
		},
		Capabilities: []campaign.ProviderCapability{{
			Plane: "implementation", Operator: "response.status.replace", Target: "POST /documents",
		}},
		Mutations: []campaign.ProviderMutationReview{{
			Sequence: 1, MutationID: "status-200-create", Plane: "implementation",
			Operator: "response.status.replace", Target: "POST /documents",
			EntryStatus: "present", CapabilityStatus: "declared", Status: "supported",
		}},
	}
	artifact, err := evidence.BuildProviderReviewCIResult(review, ".")
	if err != nil {
		t.Fatal(err)
	}
	var instance any
	if err := json.Unmarshal(artifact.Report, &instance); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(instance); err != nil {
		t.Fatalf("produced provider review does not match published schema: %v", err)
	}
}
