package spec

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

func TestPublishedSchemaContracts(t *testing.T) {
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() did not return the schema test path")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(sourcePath), "..", ".."))
	tests := []struct {
		name                    string
		file                    string
		constValue              string
		requiredFields          []string
		definitions             []string
		discriminatorDefinition string
	}{
		{
			name:                    "contract",
			file:                    "ingen.contract-v1.schema.json",
			constValue:              "ingen.contract/v1",
			requiredFields:          []string{"contract"},
			definitions:             []string{"contract", "interface", "rule"},
			discriminatorDefinition: "contract",
		},
		{
			name:                    "policy",
			file:                    "ingen.policy-v1.schema.json",
			constValue:              "ingen.policy/v1",
			requiredFields:          []string{"policy"},
			definitions:             []string{"policy", "filesystem", "network", "process"},
			discriminatorDefinition: "policy",
		},
		{
			name:           "oracle",
			file:           "ingen.oracle-v1.schema.json",
			constValue:     "ingen.oracle/v1",
			requiredFields: []string{"schema", "status", "contract", "policy_sha256", "cases"},
			definitions:    []string{"contract", "case"},
		},
		{
			name:           "run",
			file:           "ingen.run-v1.schema.json",
			constValue:     "ingen.run/v1",
			requiredFields: []string{"schema", "run_id", "created_at", "assurance", "contract", "contract_verdict", "subject", "summary", "rules"},
			definitions:    []string{"assurance", "contract-reference", "rule-result", "observation"},
		},
		{
			name:           "evidence",
			file:           "sorna.evidence-v1.schema.json",
			constValue:     "sorna.evidence/v1",
			requiredFields: []string{"schema", "run_id", "created_at", "assurance", "contract", "subject", "artifacts_sha256"},
			definitions:    []string{"assurance", "contract-reference", "campaign"},
		},
		{
			name:           "mutation-campaign-result",
			file:           "ingen.mutation-campaign-result-v1.schema.json",
			constValue:     "ingen.mutation-campaign-result/v1",
			requiredFields: []string{"schema", "status", "plan", "started_at", "finished_at", "summary", "entries"},
			definitions:    []string{"plan", "entry", "summary", "diagnosis"},
		},
		{
			name:           "report",
			file:           "ingen.replay-matrix-v1.schema.json",
			constValue:     "sorna.replay-matrix/v1",
			requiredFields: []string{"schema", "status", "total", "matched", "mismatched", "entries"},
			definitions:    []string{"entry"},
		},
		{
			name:           "explanation",
			file:           "ingen.replay-matrix-explanation-v1.schema.json",
			constValue:     "sorna.replay-matrix-explanation/v1",
			requiredFields: []string{"schema", "status", "total", "matched", "mismatched"},
			definitions:    []string{"failure"},
		},
		{
			name:           "manifest",
			file:           "ingen.replay-matrix-manifest-v1.schema.json",
			constValue:     "sorna.replay-matrix-manifest/v1",
			requiredFields: []string{"schema", "id", "version", "cases"},
			definitions:    []string{"case"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(repoRoot, "sorna", "spec", test.file))
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
			compiler := jsonschema.NewCompiler()
			if err := compiler.AddResource(test.file, bytes.NewReader(data)); err != nil {
				t.Fatalf("register %s schema: %v", test.name, err)
			}
			if _, err := compiler.Compile(test.file); err != nil {
				t.Fatalf("compile %s schema: %v", test.name, err)
			}
			if schema.ID == "" || schema.Draft == "" || schema.Type != "object" || schema.AdditionalProperties {
				t.Fatalf("incomplete or open %s schema: %+v", test.name, schema)
			}
			for _, required := range test.requiredFields {
				if !containsSchemaField(schema.Required, required) {
					t.Fatalf("%s schema is missing required field %q", test.name, required)
				}
				if _, ok := schema.Properties[required]; !ok {
					t.Fatalf("%s schema is missing property %q", test.name, required)
				}
			}
			properties := schema.Properties
			if test.discriminatorDefinition != "" {
				var definition struct {
					Properties map[string]json.RawMessage `json:"properties"`
				}
				if err := json.Unmarshal(schema.Definitions[test.discriminatorDefinition], &definition); err != nil {
					t.Fatalf("decode %s discriminator definition: %v", test.name, err)
				}
				properties = definition.Properties
			}
			discriminator, ok := properties["schema"]
			if !ok {
				t.Fatalf("%s schema is missing schema discriminator", test.name)
			}
			var discriminatorShape struct {
				Const string `json:"const"`
			}
			if err := json.Unmarshal(discriminator, &discriminatorShape); err != nil {
				t.Fatalf("decode %s discriminator: %v", test.name, err)
			}
			if discriminatorShape.Const != test.constValue {
				t.Fatalf("%s discriminator = %q, want %q", test.name, discriminatorShape.Const, test.constValue)
			}
			for _, definition := range test.definitions {
				if _, ok := schema.Definitions[definition]; !ok {
					t.Fatalf("%s schema is missing definition %q", test.name, definition)
				}
			}
		})
	}
}

func containsSchemaField(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
