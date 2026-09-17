package release

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	ManifestSchema     = "sorna.release/v1"
	VerificationSchema = "sorna.release-verification/v1"
	ProvenanceSchema   = "sorna.release-provenance/v1"
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

type ProvenanceInput struct {
	Repository string
	Ref        string
	Tag        string
	Commit     string
	Workflow   string
	RunID      string
	RunAttempt string
	Runner     string
	BuildDate  string
}

type Provenance struct {
	Schema       string                 `json:"schema"`
	Name         string                 `json:"name"`
	Version      string                 `json:"version"`
	Source       ProvenanceSource       `json:"source"`
	Build        ProvenanceBuild        `json:"build"`
	Manifest     ProvenanceFile         `json:"manifest"`
	Verification ProvenanceVerification `json:"verification"`
}

type ProvenanceSource struct {
	Repository string `json:"repository"`
	Ref        string `json:"ref"`
	Tag        string `json:"tag"`
	Commit     string `json:"commit"`
}

type ProvenanceBuild struct {
	Workflow   string `json:"workflow"`
	RunID      string `json:"run_id"`
	RunAttempt string `json:"run_attempt"`
	Runner     string `json:"runner"`
	BuildDate  string `json:"build_date"`
}

type ProvenanceFile struct {
	SHA256 string `json:"sha256"`
}

type ProvenanceVerification struct {
	SHA256 string `json:"sha256"`
	Status string `json:"status"`
}

func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read release manifest: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse release manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Manifest{}, fmt.Errorf("parse release manifest: multiple JSON values")
		}
		return Manifest{}, fmt.Errorf("parse release manifest trailing data: %w", err)
	}
	if err := ValidateManifest(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func LoadVerification(path string) (Verification, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Verification{}, fmt.Errorf("read release verification: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var verification Verification
	if err := decoder.Decode(&verification); err != nil {
		return Verification{}, fmt.Errorf("parse release verification: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Verification{}, fmt.Errorf("parse release verification: multiple JSON values")
		}
		return Verification{}, fmt.Errorf("parse release verification trailing data: %w", err)
	}
	if err := ValidateVerification(verification); err != nil {
		return Verification{}, err
	}
	return verification, nil
}

func ValidateManifest(manifest Manifest) error {
	if manifest.Schema != ManifestSchema {
		return fmt.Errorf("release manifest schema must be %s, got %q", ManifestSchema, manifest.Schema)
	}
	if manifest.Name != "sorna" {
		return fmt.Errorf("release manifest name must be sorna, got %q", manifest.Name)
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
		if err != nil || len(digest) != sha256.Size || strings.ToLower(artifact.SHA256) != artifact.SHA256 {
			return fmt.Errorf("release artifact %q has invalid sha256", artifact.Name)
		}
	}
	return nil
}

func ValidateVerification(verification Verification) error {
	if verification.Schema != VerificationSchema {
		return fmt.Errorf("release verification schema must be %s, got %q", VerificationSchema, verification.Schema)
	}
	if verification.Status != "passed" && verification.Status != "failed" {
		return fmt.Errorf("release verification status must be passed or failed, got %q", verification.Status)
	}
	if verification.Manifest == "" || verification.Directory == "" || verification.Version == "" {
		return fmt.Errorf("release verification manifest, directory, and version are required")
	}
	if len(verification.Artifacts) == 0 {
		return fmt.Errorf("release verification must contain artifacts")
	}
	for _, artifact := range verification.Artifacts {
		if artifact.Name == "" || artifact.ExpectedSHA256 == "" {
			return fmt.Errorf("release verification artifacts require name and expected_sha256")
		}
		if !safeFileName(artifact.Name) {
			return fmt.Errorf("release verification artifact name must be a safe file name: %q", artifact.Name)
		}
		if !validSHA256(artifact.ExpectedSHA256) {
			return fmt.Errorf("release verification artifact %q has invalid expected_sha256", artifact.Name)
		}
		if artifact.ActualSHA256 != "" && !validSHA256(artifact.ActualSHA256) {
			return fmt.Errorf("release verification artifact %q has invalid actual_sha256", artifact.Name)
		}
		if artifact.Status != "passed" && artifact.Status != "failed" {
			return fmt.Errorf("release verification artifact %q has unsupported status %q", artifact.Name, artifact.Status)
		}
		if artifact.Status == "passed" && artifact.ActualSHA256 != artifact.ExpectedSHA256 {
			return fmt.Errorf("release verification artifact %q passed without matching sha256", artifact.Name)
		}
	}
	return nil
}

func CreateProvenance(manifestPath, verificationPath string, input ProvenanceInput) (Provenance, error) {
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return Provenance{}, err
	}
	verification, err := LoadVerification(verificationPath)
	if err != nil {
		return Provenance{}, err
	}
	if verification.Status != "passed" {
		return Provenance{}, fmt.Errorf("release provenance requires a passed verification")
	}
	if verification.Version != manifest.Version {
		return Provenance{}, fmt.Errorf("release provenance version %q does not match verification version %q", manifest.Version, verification.Version)
	}
	if input.Commit != manifest.Commit {
		return Provenance{}, fmt.Errorf("release provenance commit %q does not match manifest commit %q", input.Commit, manifest.Commit)
	}
	if input.BuildDate != manifest.BuildDate {
		return Provenance{}, fmt.Errorf("release provenance build_date %q does not match manifest build_date %q", input.BuildDate, manifest.BuildDate)
	}

	manifestSHA256, err := hashFile(manifestPath)
	if err != nil {
		return Provenance{}, fmt.Errorf("hash release manifest: %w", err)
	}
	verificationSHA256, err := hashFile(verificationPath)
	if err != nil {
		return Provenance{}, fmt.Errorf("hash release verification: %w", err)
	}
	provenance := Provenance{
		Schema:  ProvenanceSchema,
		Name:    manifest.Name,
		Version: manifest.Version,
		Source: ProvenanceSource{
			Repository: input.Repository,
			Ref:        input.Ref,
			Tag:        input.Tag,
			Commit:     input.Commit,
		},
		Build: ProvenanceBuild{
			Workflow:   input.Workflow,
			RunID:      input.RunID,
			RunAttempt: input.RunAttempt,
			Runner:     input.Runner,
			BuildDate:  input.BuildDate,
		},
		Manifest:     ProvenanceFile{SHA256: manifestSHA256},
		Verification: ProvenanceVerification{SHA256: verificationSHA256, Status: verification.Status},
	}
	if err := ValidateProvenance(provenance); err != nil {
		return Provenance{}, err
	}
	return provenance, nil
}

func ValidateProvenance(provenance Provenance) error {
	if provenance.Schema != ProvenanceSchema {
		return fmt.Errorf("release provenance schema must be %s, got %q", ProvenanceSchema, provenance.Schema)
	}
	if provenance.Name != "sorna" || provenance.Version == "" {
		return fmt.Errorf("release provenance name must be sorna and version is required")
	}
	if provenance.Source.Repository == "" || provenance.Source.Ref == "" || provenance.Source.Tag == "" || provenance.Source.Commit == "" {
		return fmt.Errorf("release provenance source repository, ref, tag, and commit are required")
	}
	if provenance.Source.Tag != "sorna-v"+provenance.Version {
		return fmt.Errorf("release provenance tag %q does not match version %q", provenance.Source.Tag, provenance.Version)
	}
	if provenance.Build.Workflow == "" || provenance.Build.RunID == "" || provenance.Build.RunAttempt == "" || provenance.Build.Runner == "" || provenance.Build.BuildDate == "" {
		return fmt.Errorf("release provenance build workflow, run_id, run_attempt, runner, and build_date are required")
	}
	if !validSHA256(provenance.Manifest.SHA256) {
		return fmt.Errorf("release provenance manifest sha256 must be a lowercase SHA-256 digest")
	}
	if !validSHA256(provenance.Verification.SHA256) || provenance.Verification.Status != "passed" {
		return fmt.Errorf("release provenance verification must contain a passed result and lowercase SHA-256 digest")
	}
	return nil
}

func LoadProvenance(path string) (Provenance, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Provenance{}, fmt.Errorf("read release provenance: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var provenance Provenance
	if err := decoder.Decode(&provenance); err != nil {
		return Provenance{}, fmt.Errorf("parse release provenance: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Provenance{}, fmt.Errorf("parse release provenance: multiple JSON values")
		}
		return Provenance{}, fmt.Errorf("parse release provenance trailing data: %w", err)
	}
	if err := ValidateProvenance(provenance); err != nil {
		return Provenance{}, err
	}
	return provenance, nil
}

func WriteProvenance(path string, provenance Provenance) error {
	if err := ValidateProvenance(provenance); err != nil {
		return err
	}
	data, err := json.MarshalIndent(provenance, "", "  ")
	if err != nil {
		return fmt.Errorf("encode release provenance: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write release provenance: %w", err)
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
		Status:    "passed",
		Manifest:  manifestPath,
		Directory: directory,
		Version:   manifest.Version,
		Artifacts: make([]ArtifactResult, 0, len(manifest.Artifacts)),
	}
	for _, expected := range manifest.Artifacts {
		artifactResult := ArtifactResult{
			Name:           expected.Name,
			ExpectedSHA256: expected.SHA256,
			Status:         "passed",
		}
		data, readErr := os.ReadFile(filepath.Join(directory, expected.Name))
		if readErr != nil {
			artifactResult.Status = "failed"
			artifactResult.Error = readErr.Error()
		} else {
			digest := sha256.Sum256(data)
			artifactResult.ActualSHA256 = hex.EncodeToString(digest[:])
			if artifactResult.ActualSHA256 != expected.SHA256 {
				artifactResult.Status = "failed"
				artifactResult.Error = "sha256 mismatch"
			}
		}
		if artifactResult.Status != "passed" {
			result.Status = "failed"
		}
		result.Artifacts = append(result.Artifacts, artifactResult)
	}
	return result, nil
}

func VerifyProvenance(provenancePath, manifestPath, verificationPath string) error {
	provenance, err := LoadProvenance(provenancePath)
	if err != nil {
		return err
	}
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return err
	}
	verification, err := LoadVerification(verificationPath)
	if err != nil {
		return err
	}
	if provenance.Name != manifest.Name || provenance.Version != manifest.Version {
		return fmt.Errorf("release provenance identity does not match manifest")
	}
	if provenance.Source.Commit != manifest.Commit || provenance.Build.BuildDate != manifest.BuildDate {
		return fmt.Errorf("release provenance source does not match manifest metadata")
	}
	if verification.Version != manifest.Version || verification.Status != provenance.Verification.Status {
		return fmt.Errorf("release provenance verification does not match manifest")
	}
	manifestSHA256, err := hashFile(manifestPath)
	if err != nil {
		return fmt.Errorf("hash release manifest: %w", err)
	}
	if manifestSHA256 != provenance.Manifest.SHA256 {
		return fmt.Errorf("release provenance manifest sha256 mismatch")
	}
	verificationSHA256, err := hashFile(verificationPath)
	if err != nil {
		return fmt.Errorf("hash release verification: %w", err)
	}
	if verificationSHA256 != provenance.Verification.SHA256 {
		return fmt.Errorf("release provenance verification sha256 mismatch")
	}
	return nil
}

func Text(result Verification) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "RELEASE-VERIFY %s %s\n", strings.ToUpper(result.Status), result.Manifest)
	for _, artifact := range result.Artifacts {
		if artifact.Status == "passed" {
			fmt.Fprintf(&builder, "  PASS %s\n", artifact.Name)
		} else {
			fmt.Fprintf(&builder, "  FAIL %s — %s\n", artifact.Name, artifact.Error)
		}
	}
	return builder.String()
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func safeFileName(name string) bool {
	return name != "" && filepath.Base(name) == name && !strings.ContainsAny(name, `/\`)
}
