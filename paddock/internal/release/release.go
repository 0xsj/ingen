package release

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	ManifestSchema     = "paddock.release/v1"
	VerificationSchema = "paddock.release-verification/v1"
)

type Manifest struct {
	Schema    string     `json:"schema"`
	Name      string     `json:"name"`
	Version   string     `json:"version"`
	Commit    string     `json:"commit"`
	BuildDate string     `json:"build_date"`
	Artifacts []Artifact `json:"artifacts"`
}

type Artifact struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

type Verification struct {
	Schema    string           `json:"schema"`
	Status    string           `json:"status"`
	Manifest  string           `json:"manifest"`
	Directory string           `json:"directory"`
	Version   string           `json:"version"`
	Artifacts []ArtifactResult `json:"artifacts"`
}

type ArtifactResult struct {
	Name           string `json:"name"`
	ExpectedSHA256 string `json:"expected_sha256"`
	ActualSHA256   string `json:"actual_sha256,omitempty"`
	Status         string `json:"status"`
	Error          string `json:"error,omitempty"`
}

func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read release manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse release manifest: %w", err)
	}
	if err := ValidateManifest(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func ValidateManifest(manifest Manifest) error {
	if manifest.Schema != ManifestSchema {
		return fmt.Errorf("release manifest schema must be %s, got %q", ManifestSchema, manifest.Schema)
	}
	if manifest.Name != "paddock" {
		return fmt.Errorf("release manifest name must be paddock, got %q", manifest.Name)
	}
	if manifest.Version == "" || manifest.Commit == "" || manifest.BuildDate == "" {
		return fmt.Errorf("release manifest version, commit, and build_date are required")
	}
	if len(manifest.Artifacts) == 0 {
		return fmt.Errorf("release manifest must contain artifacts")
	}
	seen := map[string]struct{}{}
	for _, artifact := range manifest.Artifacts {
		if artifact.Name == "" || artifact.SHA256 == "" {
			return fmt.Errorf("release artifacts require name and sha256")
		}
		if filepath.Base(artifact.Name) != artifact.Name || strings.ContainsAny(artifact.Name, `/\\`) {
			return fmt.Errorf("release artifact name must be a safe file name: %q", artifact.Name)
		}
		if _, ok := seen[artifact.Name]; ok {
			return fmt.Errorf("release manifest contains duplicate artifact %q", artifact.Name)
		}
		seen[artifact.Name] = struct{}{}
		digest, err := hex.DecodeString(artifact.SHA256)
		if err != nil || len(digest) != sha256.Size {
			return fmt.Errorf("release artifact %q has invalid sha256", artifact.Name)
		}
	}
	return nil
}

func Verify(manifestPath, directory string) (Verification, error) {
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return Verification{}, err
	}
	if directory == "" {
		directory = filepath.Dir(manifestPath)
	}
	result := Verification{
		Schema:    VerificationSchema,
		Status:    "PASS",
		Manifest:  manifestPath,
		Directory: directory,
		Version:   manifest.Version,
		Artifacts: make([]ArtifactResult, 0, len(manifest.Artifacts)),
	}
	for _, expected := range manifest.Artifacts {
		artifactResult := ArtifactResult{
			Name:           expected.Name,
			ExpectedSHA256: expected.SHA256,
			Status:         "PASS",
		}
		data, readErr := os.ReadFile(filepath.Join(directory, expected.Name))
		if readErr != nil {
			artifactResult.Status = "FAIL"
			artifactResult.Error = readErr.Error()
		} else {
			digest := sha256.Sum256(data)
			artifactResult.ActualSHA256 = hex.EncodeToString(digest[:])
			if artifactResult.ActualSHA256 != expected.SHA256 {
				artifactResult.Status = "FAIL"
				artifactResult.Error = "sha256 mismatch"
			}
		}
		if artifactResult.Status != "PASS" {
			result.Status = "FAIL"
		}
		result.Artifacts = append(result.Artifacts, artifactResult)
	}
	return result, nil
}

func Text(result Verification) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "RELEASE-VERIFY %s %s\n", result.Status, result.Manifest)
	for _, artifact := range result.Artifacts {
		if artifact.Status == "PASS" {
			fmt.Fprintf(&builder, "  PASS %s\n", artifact.Name)
		} else {
			fmt.Fprintf(&builder, "  FAIL %s — %s\n", artifact.Name, artifact.Error)
		}
	}
	return builder.String()
}
