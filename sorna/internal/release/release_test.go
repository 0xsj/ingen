package release_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/sorna/internal/release"
)

func TestVerifyManifest(t *testing.T) {
	directory := t.TempDir()
	artifact := []byte("sorna release archive")
	digest := sha256.Sum256(artifact)
	manifest := release.Manifest{
		Schema:    release.ManifestSchema,
		Name:      "sorna",
		Version:   "0.1.0",
		Commit:    "test",
		BuildDate: "today",
		Artifacts: []release.Artifact{{Name: "sorna_0.1.0_linux_amd64.tar.gz", SHA256: hex.EncodeToString(digest[:])}},
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
	if result.Status != "passed" || result.Artifacts[0].Status != "passed" {
		t.Fatalf("verification = %#v, want pass", result)
	}

	if err := os.WriteFile(artifactPath, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err = release.Verify(manifestPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "failed" || result.Artifacts[0].Status != "failed" {
		t.Fatalf("changed verification = %#v, want fail", result)
	}
}

func TestValidateManifestRejectsUnsafeArtifactName(t *testing.T) {
	manifest := release.Manifest{
		Schema:    release.ManifestSchema,
		Name:      "sorna",
		Version:   "0.1.0",
		Commit:    "test",
		BuildDate: "today",
		Artifacts: []release.Artifact{{Name: "../outside.tar.gz", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
	}
	if err := release.ValidateManifest(manifest); err == nil {
		t.Fatal("ValidateManifest accepted unsafe artifact name")
	}
}

func TestLoadManifestRejectsUnknownFieldsAndTrailingValues(t *testing.T) {
	directory := t.TempDir()
	base := `{"schema":"sorna.release/v1","name":"sorna","version":"0.1.0","commit":"test","build_date":"today","artifacts":[{"name":"archive.tar.gz","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}`
	unknownPath := filepath.Join(directory, "unknown.json")
	if err := os.WriteFile(unknownPath, []byte(base[:len(base)-1]+`,"unexpected":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := release.LoadManifest(unknownPath); err == nil {
		t.Fatal("LoadManifest accepted an unknown field")
	}
	trailingPath := filepath.Join(directory, "trailing.json")
	if err := os.WriteFile(trailingPath, []byte(base+"\n{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := release.LoadManifest(trailingPath); err == nil {
		t.Fatal("LoadManifest accepted trailing JSON")
	}
}

func TestCreateAndVerifyProvenanceBindsManifestAndVerificationBytes(t *testing.T) {
	directory := t.TempDir()
	archive := []byte("sorna release archive")
	digest := sha256.Sum256(archive)
	manifest := release.Manifest{
		Schema:    release.ManifestSchema,
		Name:      "sorna",
		Version:   "0.1.0",
		Commit:    "commit-123",
		BuildDate: "2026-09-17T00:00:00Z",
		Artifacts: []release.Artifact{{Name: "sorna_0.1.0_linux_amd64.tar.gz", SHA256: hex.EncodeToString(digest[:])}},
	}
	manifestPath := filepath.Join(directory, "release-manifest.json")
	writeJSON(t, manifestPath, manifest)
	verification := release.Verification{
		Schema:    release.VerificationSchema,
		Status:    "passed",
		Manifest:  manifestPath,
		Directory: directory,
		Version:   manifest.Version,
		Artifacts: []release.ArtifactResult{{
			Name:           manifest.Artifacts[0].Name,
			ExpectedSHA256: manifest.Artifacts[0].SHA256,
			ActualSHA256:   manifest.Artifacts[0].SHA256,
			Status:         "passed",
		}},
	}
	verificationPath := filepath.Join(directory, "release-verification.json")
	writeJSON(t, verificationPath, verification)
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
	provenancePath := filepath.Join(directory, "release-provenance.json")
	if err := release.WriteProvenance(provenancePath, provenance); err != nil {
		t.Fatal(err)
	}
	if err := release.VerifyProvenance(provenancePath, manifestPath, verificationPath); err != nil {
		t.Fatalf("VerifyProvenance() = %v, want pass", err)
	}
	if _, err := release.LoadProvenance(provenancePath); err != nil {
		t.Fatalf("LoadProvenance() = %v, want valid provenance", err)
	}

	if err := os.WriteFile(verificationPath, append(readFile(t, verificationPath), '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := release.VerifyProvenance(provenancePath, manifestPath, verificationPath); err == nil || !strings.Contains(err.Error(), "verification sha256 mismatch") {
		t.Fatalf("VerifyProvenance() after verification drift = %v, want digest mismatch", err)
	}
}

func TestCreateProvenanceRejectsFailedVerification(t *testing.T) {
	directory := t.TempDir()
	manifest := release.Manifest{
		Schema:    release.ManifestSchema,
		Name:      "sorna",
		Version:   "0.1.0",
		Commit:    "commit-123",
		BuildDate: "2026-09-17T00:00:00Z",
		Artifacts: []release.Artifact{{Name: "archive.tar.gz", SHA256: strings.Repeat("a", 64)}},
	}
	manifestPath := filepath.Join(directory, "release-manifest.json")
	writeJSON(t, manifestPath, manifest)
	verificationPath := filepath.Join(directory, "release-verification.json")
	writeJSON(t, verificationPath, release.Verification{
		Schema:    release.VerificationSchema,
		Status:    "failed",
		Manifest:  manifestPath,
		Directory: directory,
		Version:   manifest.Version,
		Artifacts: []release.ArtifactResult{{
			Name:           "archive.tar.gz",
			ExpectedSHA256: strings.Repeat("a", 64),
			Status:         "failed",
			Error:          "missing",
		}},
	})
	_, err := release.CreateProvenance(manifestPath, verificationPath, release.ProvenanceInput{
		Repository: "repo",
		Ref:        "ref",
		Tag:        "sorna-v0.1.0",
		Commit:     manifest.Commit,
		Workflow:   "workflow",
		RunID:      "run",
		RunAttempt: "1",
		Runner:     "runner",
		BuildDate:  manifest.BuildDate,
	})
	if err == nil || !strings.Contains(err.Error(), "passed verification") {
		t.Fatalf("CreateProvenance() = %v, want failed-verification rejection", err)
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
