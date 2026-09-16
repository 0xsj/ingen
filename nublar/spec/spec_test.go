package spec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	nublarrun "ingen/nublar/internal/run"
	"ingen/nublar/internal/workflow"
)

func TestNublarSchemaContracts(t *testing.T) {
	repoRoot := repositoryRoot(t)
	tests := []struct {
		name           string
		path           string
		constValue     string
		requiredFields []string
		definitions    []string
	}{
		{
			name:           "workflow",
			path:           filepath.Join(repoRoot, "nublar", "spec", "workflow-v1.schema.json"),
			constValue:     "ingen.nublar-workflow/v1",
			requiredFields: []string{"schema", "id", "checks"},
		},
		{
			name:           "run",
			path:           filepath.Join(repoRoot, "nublar", "spec", "run-v1.schema.json"),
			constValue:     "ingen.nublar-run/v1",
			requiredFields: []string{"schema", "run_id", "workflow", "status", "exit_code", "created_at", "completed_at", "checks"},
			definitions:    []string{"file-ref", "workflow", "check", "result", "issue"},
		},
		{
			name:           "decision",
			path:           filepath.Join(repoRoot, "nublar", "spec", "decision-v1.schema.json"),
			constValue:     "ingen.nublar-decision/v1",
			requiredFields: []string{"schema", "run_id", "workflow", "status", "exit_code", "created_at", "completed_at", "checks"},
			definitions:    []string{"file-ref", "workflow", "check", "issue"},
		},
		{
			name:           "receipt",
			path:           filepath.Join(repoRoot, "nublar", "spec", "receipt-v1.schema.json"),
			constValue:     "ingen.nublar-delivery-receipt/v1",
			requiredFields: []string{"schema", "run_id", "transport", "status", "attempted_at"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data, err := os.ReadFile(test.path)
			if err != nil {
				t.Fatal(err)
			}
			var schema struct {
				ID                   string                     `json:"$id"`
				Draft                string                     `json:"$schema"`
				Type                 string                     `json:"type"`
				AdditionalProperties bool                       `json:"additionalProperties"`
				Required             []string                   `json:"required"`
				Properties           map[string]json.RawMessage `json:"properties"`
				Definitions          map[string]json.RawMessage `json:"$defs"`
			}
			if err := json.Unmarshal(data, &schema); err != nil {
				t.Fatalf("decode %s schema: %v", test.name, err)
			}
			if schema.ID == "" || schema.Draft == "" || schema.Type != "object" || schema.AdditionalProperties {
				t.Fatalf("incomplete or open %s schema: %+v", test.name, schema)
			}
			for _, required := range test.requiredFields {
				if !contains(schema.Required, required) {
					t.Fatalf("%s schema is missing required field %q", test.name, required)
				}
				if _, ok := schema.Properties[required]; !ok {
					t.Fatalf("%s schema is missing property %q", test.name, required)
				}
			}
			var discriminator struct {
				Const string `json:"const"`
			}
			if err := json.Unmarshal(schema.Properties["schema"], &discriminator); err != nil {
				t.Fatalf("decode %s discriminator: %v", test.name, err)
			}
			if discriminator.Const != test.constValue {
				t.Fatalf("%s discriminator = %q, want %q", test.name, discriminator.Const, test.constValue)
			}
			for _, definition := range test.definitions {
				if _, ok := schema.Definitions[definition]; !ok {
					t.Fatalf("%s schema is missing definition %q", test.name, definition)
				}
			}
		})
	}
}

func TestRunSchemaReferencesSharedCIEnvelope(t *testing.T) {
	repoRoot := repositoryRoot(t)
	data, err := os.ReadFile(filepath.Join(repoRoot, "nublar", "spec", "run-v1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Definitions map[string]json.RawMessage `json:"$defs"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema.Definitions["result"], &result); err != nil {
		t.Fatal(err)
	}
	var artifactRef struct {
		Ref string `json:"$ref"`
	}
	if err := json.Unmarshal(result.Properties["artifact"], &artifactRef); err != nil {
		t.Fatal(err)
	}
	if artifactRef.Ref != "https://ingen.example/spec/ingen.ci-result-v1.schema.json" {
		t.Fatalf("run artifact reference = %q, want shared CI envelope schema", artifactRef.Ref)
	}
}

func TestGeneratedWorkflowAndRunUseVersionedDiscriminators(t *testing.T) {
	repoRoot := repositoryRoot(t)
	workflowPath := filepath.Join(repoRoot, "nublar", "testdata", "workflows", "mixed-producers.yaml")
	artifactRoot := filepath.Join(repoRoot, "nublar", "testdata", "ci-results")
	document, err := workflow.LoadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	if document.Schema != workflow.Schema {
		t.Fatalf("workflow schema = %q, want %q", document.Schema, workflow.Schema)
	}
	record, err := nublarrun.CollectWorkflowFile(workflowPath, artifactRoot, "schema-test-run")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatal(err)
	}
	var schemaValue string
	if err := json.Unmarshal(object["schema"], &schemaValue); err != nil {
		t.Fatal(err)
	}
	if schemaValue != nublarrun.Schema {
		t.Fatalf("run schema = %q, want %q", schemaValue, nublarrun.Schema)
	}
	for _, field := range []string{"run_id", "workflow", "checks", "status", "exit_code"} {
		if _, ok := object[field]; !ok {
			t.Fatalf("generated run is missing schema field %q", field)
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() did not return source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(sourcePath), "..", ".."))
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
