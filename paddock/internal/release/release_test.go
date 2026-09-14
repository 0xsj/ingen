package release_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"ingen/paddock/internal/release"
)

func TestVerifyManifest(t *testing.T) {
	directory := t.TempDir()
	artifact := []byte("release archive")
	digest := sha256.Sum256(artifact)
	manifest := release.Manifest{
		Schema:    release.ManifestSchema,
		Name:      "paddock",
		Version:   "0.1.0",
		Commit:    "test",
		BuildDate: "today",
		Artifacts: []release.Artifact{{Name: "paddock_0.1.0_linux_amd64.tar.gz", SHA256: hex.EncodeToString(digest[:])}},
	}
	artifactPath := filepath.Join(directory, manifest.Artifacts[0].Name)
	if err := os.WriteFile(artifactPath, artifact, 0o644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(directory, "release-manifest.json")
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := release.Verify(manifestPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "PASS" || result.Artifacts[0].Status != "PASS" {
		t.Fatalf("verification = %#v, want pass", result)
	}

	if err := os.WriteFile(artifactPath, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err = release.Verify(manifestPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "FAIL" || result.Artifacts[0].Status != "FAIL" {
		t.Fatalf("changed verification = %#v, want fail", result)
	}
}

func TestValidateManifestRejectsUnsafeArtifactName(t *testing.T) {
	manifest := release.Manifest{
		Schema:    release.ManifestSchema,
		Name:      "paddock",
		Version:   "0.1.0",
		Commit:    "test",
		BuildDate: "today",
		Artifacts: []release.Artifact{{Name: "../outside.tar.gz", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
	}
	if err := release.ValidateManifest(manifest); err == nil {
		t.Fatal("ValidateManifest accepted unsafe artifact name")
	}
}
