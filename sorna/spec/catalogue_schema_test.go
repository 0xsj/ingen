package spec

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v3"
	"ingen/sorna/internal/mutation"
)

func TestMutationCatalogueSchemaAcceptsRepositoryFixture(t *testing.T) {
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() did not return the schema test path")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(sourcePath), "..", ".."))
	schemaPath := filepath.Join(filepath.Dir(sourcePath), "ingen.mutation-catalogue-v1.schema.json")
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

	fixturePath := filepath.Join(repoRoot, "examples", "document-pipeline-lab", "mutations", "catalogue.yaml")
	if _, err := mutation.LoadFile(fixturePath); err != nil {
		t.Fatalf("repository catalogue fixture does not load: %v", err)
	}
	fixture, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(fixture, &document); err != nil {
		t.Fatalf("decode repository catalogue fixture: %v", err)
	}
	instanceBytes, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	var instance any
	if err := json.Unmarshal(instanceBytes, &instance); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(instance); err != nil {
		t.Fatalf("repository mutation catalogue does not match published schema: %v", err)
	}
}
