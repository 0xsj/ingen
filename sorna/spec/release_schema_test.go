package spec

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"ingen/sorna/internal/release"
)

func TestReleaseSchemasAcceptProducedManifestAndVerification(t *testing.T) {
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() did not return the schema test path")
	}
	schemaDirectory := filepath.Dir(sourcePath)
	manifestSchema := compileSchema(t, filepath.Join(schemaDirectory, "sorna.release-v1.schema.json"))
	verificationSchema := compileSchema(t, filepath.Join(schemaDirectory, "sorna.release-verification-v1.schema.json"))
	provenanceSchema := compileSchema(t, filepath.Join(schemaDirectory, "sorna.release-provenance-v1.schema.json"))

	directory := t.TempDir()
	archive := []byte("sorna release archive")
	digest := sha256.Sum256(archive)
	manifest := release.Manifest{
		Schema:    release.ManifestSchema,
		Name:      "sorna",
		Version:   "0.1.0",
		Commit:    "test",
		BuildDate: "2026-09-17T00:00:00Z",
		Artifacts: []release.Artifact{{Name: "sorna_0.1.0_linux_amd64.tar.gz", SHA256: hex.EncodeToString(digest[:])}},
	}
	manifestPath := filepath.Join(directory, "release-manifest.json")
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, manifest.Artifacts[0].Name), archive, 0o644); err != nil {
		t.Fatal(err)
	}

	var manifestInstance any
	if err := json.Unmarshal(manifestBytes, &manifestInstance); err != nil {
		t.Fatal(err)
	}
	if err := manifestSchema.Validate(manifestInstance); err != nil {
		t.Fatalf("produced release manifest does not match schema: %v", err)
	}
	result, err := release.Verify(manifestPath, directory)
	if err != nil {
		t.Fatal(err)
	}
	resultBytes, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var resultInstance any
	if err := json.Unmarshal(resultBytes, &resultInstance); err != nil {
		t.Fatal(err)
	}
	if err := verificationSchema.Validate(resultInstance); err != nil {
		t.Fatalf("produced release verification does not match schema: %v", err)
	}
	verificationPath := filepath.Join(directory, "release-verification.json")
	if err := os.WriteFile(verificationPath, resultBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	provenance, err := release.CreateProvenance(manifestPath, verificationPath, release.ProvenanceInput{
		Repository: "https://github.com/ingen/ingen",
		Ref:        "refs/tags/sorna-v0.1.0",
		Tag:        "sorna-v0.1.0",
		Commit:     manifest.Commit,
		Workflow:   "Sorna release",
		RunID:      "123",
		RunAttempt: "1",
		Runner:     "macos",
		BuildDate:  manifest.BuildDate,
	})
	if err != nil {
		t.Fatal(err)
	}
	provenanceBytes, err := json.Marshal(provenance)
	if err != nil {
		t.Fatal(err)
	}
	var provenanceInstance any
	if err := json.Unmarshal(provenanceBytes, &provenanceInstance); err != nil {
		t.Fatal(err)
	}
	if err := provenanceSchema.Validate(provenanceInstance); err != nil {
		t.Fatalf("produced release provenance does not match schema: %v", err)
	}
}

func compileSchema(t *testing.T, path string) *jsonschema.Schema {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(path, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(path)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}
