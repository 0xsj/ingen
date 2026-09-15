package campaign

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMutationProviderSchemaContract(t *testing.T) {
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() did not return the test source path")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(sourcePath), "..", "..", ".."))
	data, err := os.ReadFile(filepath.Join(repoRoot, "sorna", "spec", "ingen.mutation-provider-v1.schema.json"))
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
		Defs                 map[string]json.RawMessage `json:"$defs"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("decode provider schema: %v", err)
	}
	if schema.ID == "" || schema.Draft == "" || schema.Type != "object" || schema.AdditionalProperties {
		t.Fatalf("incomplete provider schema: %+v", schema)
	}
	if !contains(schema.Required, "mutation_provider") {
		t.Fatal("provider schema is missing required field mutation_provider")
	}
	if _, ok := schema.Properties["mutation_provider"]; !ok {
		t.Fatal("provider schema is missing mutation_provider property")
	}
	for _, definition := range []string{"provider", "capability", "entry", "provenance", "target_resolution"} {
		if _, ok := schema.Defs[definition]; !ok {
			t.Fatalf("provider schema is missing definition %q", definition)
		}
	}

	var provider struct {
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema.Defs["provider"], &provider); err != nil {
		t.Fatalf("decode provider definition: %v", err)
	}
	for _, required := range []string{"schema", "id", "version", "plan_schema", "capabilities", "entries"} {
		if !contains(provider.Required, required) {
			t.Fatalf("provider definition is missing required field %q", required)
		}
		if _, ok := provider.Properties[required]; !ok {
			t.Fatalf("provider definition is missing property %q", required)
		}
	}

	var capability struct {
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema.Defs["capability"], &capability); err != nil {
		t.Fatalf("decode capability definition: %v", err)
	}
	for _, required := range []string{"plane", "operator", "target"} {
		if !contains(capability.Required, required) {
			t.Fatalf("capability definition is missing required field %q", required)
		}
		if _, ok := capability.Properties[required]; !ok {
			t.Fatalf("capability definition is missing property %q", required)
		}
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
