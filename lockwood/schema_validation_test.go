package lockwood

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

const (
	artifactSchemaURL      = "https://ingen.example/spec/lockwood.artifact-v1.schema.json"
	custodySchemaURL       = "https://ingen.example/spec/lockwood.custody-v1.schema.json"
	custodyV2SchemaURL     = "https://ingen.example/spec/lockwood.custody-v2.schema.json"
	attestationSchemaURL   = "https://ingen.example/spec/lockwood.attestation-v1.schema.json"
	trustSchemaURL         = "https://ingen.example/spec/lockwood.attestation-trust-v1.schema.json"
	handlingEventSchemaURL = "https://ingen.example/spec/lockwood.handling-event-v1.schema.json"
)

func TestDraftSchemasValidateFixtures(t *testing.T) {
	custodySchema := compileLockwoodSchema(t, custodySchemaURL, "spec/lockwood.custody-v1.schema.json")
	artifactSchema := compileLockwoodSchema(t, artifactSchemaURL, "spec/lockwood.artifact-v1.schema.json")

	valid := loadJSONDocument(t, "testdata/valid-custody-record.json")
	if err := custodySchema.Validate(valid); err != nil {
		t.Fatalf("valid custody fixture rejected: %v", err)
	}
	artifact := valid.(map[string]any)["artifact"]
	if err := artifactSchema.Validate(artifact); err != nil {
		t.Fatalf("valid artifact reference rejected: %v", err)
	}
}

func TestDraftCustodySchemaRejectsInvalidFixtures(t *testing.T) {
	schema := compileLockwoodSchema(t, custodySchemaURL, "spec/lockwood.custody-v1.schema.json")
	for _, name := range []string{
		"invalid-digest.json",
		"invalid-missing-required.json",
		"invalid-lineage.json",
	} {
		t.Run(name, func(t *testing.T) {
			if err := schema.Validate(loadJSONDocument(t, filepath.Join("testdata", name))); err == nil {
				t.Fatal("invalid custody fixture was accepted by the JSON Schema")
			}
		})
	}
}

func TestDraftCustodyV2SchemaValidatesRemoteFixture(t *testing.T) {
	schema := compileLockwoodSchema(t, custodyV2SchemaURL, "spec/lockwood.custody-v2.schema.json")
	if err := schema.Validate(loadJSONDocument(t, "testdata/valid-remote-custody-v2-record.json")); err != nil {
		t.Fatalf("valid remote v2 custody fixture rejected: %v", err)
	}
}

func TestDraftAttestationSchemaValidatesFixture(t *testing.T) {
	schema := compileLockwoodSchema(t, attestationSchemaURL, "spec/lockwood.attestation-v1.schema.json")
	if err := schema.Validate(loadJSONDocument(t, "testdata/valid-attestation-v1.json")); err != nil {
		t.Fatalf("valid attestation rejected by draft schema: %v", err)
	}
}

func TestDraftAttestationTrustSchemaValidatesFixture(t *testing.T) {
	schema := compileLockwoodSchema(t, trustSchemaURL, "spec/lockwood.attestation-trust-v1.schema.json")
	if err := schema.Validate(loadJSONDocument(t, "testdata/valid-attestation-trust-v1.json")); err != nil {
		t.Fatalf("valid attestation trust registry rejected by draft schema: %v", err)
	}
}

func TestDraftHandlingEventSchemaValidatesFixture(t *testing.T) {
	schema := compileLockwoodSchema(t, handlingEventSchemaURL, "spec/lockwood.handling-event-v1.schema.json")
	if err := schema.Validate(loadJSONDocument(t, "testdata/valid-handling-event-v1.json")); err != nil {
		t.Fatalf("valid handling event rejected by draft schema: %v", err)
	}
}

func compileLockwoodSchema(t *testing.T, url, relativePath string) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat = true
	for _, resource := range []struct {
		url  string
		path string
	}{
		{url: artifactSchemaURL, path: "spec/lockwood.artifact-v1.schema.json"},
		{url: custodySchemaURL, path: "spec/lockwood.custody-v1.schema.json"},
		{url: custodyV2SchemaURL, path: "spec/lockwood.custody-v2.schema.json"},
		{url: attestationSchemaURL, path: "spec/lockwood.attestation-v1.schema.json"},
		{url: trustSchemaURL, path: "spec/lockwood.attestation-trust-v1.schema.json"},
		{url: handlingEventSchemaURL, path: "spec/lockwood.handling-event-v1.schema.json"},
	} {
		data, err := os.ReadFile(resource.path)
		if err != nil {
			t.Fatal(err)
		}
		if err := compiler.AddResource(resource.url, bytes.NewReader(data)); err != nil {
			t.Fatalf("add schema resource %s: %v", resource.url, err)
		}
	}
	schema, err := compiler.Compile(url)
	if err != nil {
		t.Fatalf("compile schema %s (%s): %v", url, relativePath, err)
	}
	return schema
}

func loadJSONDocument(t *testing.T, path string) any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return document
}
