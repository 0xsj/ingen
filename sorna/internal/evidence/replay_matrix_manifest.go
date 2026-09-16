package evidence

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"ingen/core/ciresult"
)

// ReplayMatrixManifestSchema identifies the declarative expectation file used
// to build a replay matrix.
const ReplayMatrixManifestSchema = "sorna.replay-matrix-manifest/v1"

// ReplayMatrixManifest is a reviewable, language-neutral set of replay result
// paths and their expected outer and nested classifications.
type ReplayMatrixManifest struct {
	Schema  string             `json:"schema" yaml:"schema"`
	ID      string             `json:"id" yaml:"id"`
	Version int64              `json:"version" yaml:"version"`
	Cases   []ReplayMatrixCase `json:"cases" yaml:"cases"`
}

// LoadReplayMatrixManifest loads a strict YAML or JSON replay matrix
// manifest. JSON is accepted so CI systems can generate the same shape when
// YAML is inconvenient.
func LoadReplayMatrixManifest(path string) (ReplayMatrixManifest, error) {
	if strings.TrimSpace(path) == "" {
		return ReplayMatrixManifest{}, fmt.Errorf("replay matrix manifest path must not be empty")
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".yaml" && ext != ".yml" && ext != ".json" {
		return ReplayMatrixManifest{}, fmt.Errorf("replay matrix manifest must use .json, .yaml, or .yml")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return ReplayMatrixManifest{}, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	decoder.KnownFields(true)
	var manifest ReplayMatrixManifest
	if err := decoder.Decode(&manifest); err != nil {
		return ReplayMatrixManifest{}, fmt.Errorf("parse replay matrix manifest %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return ReplayMatrixManifest{}, fmt.Errorf("parse replay matrix manifest %s: multiple documents are not supported", path)
		}
		return ReplayMatrixManifest{}, fmt.Errorf("parse replay matrix manifest %s: %w", path, err)
	}
	if err := ValidateReplayMatrixManifest(manifest); err != nil {
		return ReplayMatrixManifest{}, fmt.Errorf("validate replay matrix manifest %s: %w", path, err)
	}
	return manifest, nil
}

// ValidateReplayMatrixManifest checks the manifest identity and delegates
// case classification checks to the same rules used by the CLI/API path.
func ValidateReplayMatrixManifest(manifest ReplayMatrixManifest) error {
	if manifest.Schema != ReplayMatrixManifestSchema {
		return fmt.Errorf("replay matrix manifest schema must be %s", ReplayMatrixManifestSchema)
	}
	if strings.TrimSpace(manifest.ID) == "" {
		return fmt.Errorf("replay matrix manifest ID must not be empty")
	}
	if manifest.Version < 1 {
		return fmt.Errorf("replay matrix manifest version must be positive")
	}
	return validateReplayMatrixCases(manifest.Cases)
}

func replayMatrixManifestPath(path, sourceRoot string) string {
	if strings.TrimSpace(sourceRoot) == "" {
		sourceRoot = "."
	}
	return resolveReplayPath(path, sourceRoot)
}

// BuildReplayMatrixCIResultFromManifest loads a declarative manifest and
// binds its exact bytes into the resulting CI envelope.
func BuildReplayMatrixCIResultFromManifest(manifestPath, sourceRoot string) (ciresult.Artifact, error) {
	manifest, err := LoadReplayMatrixManifest(replayMatrixManifestPath(manifestPath, sourceRoot))
	if err != nil {
		return ciresult.Artifact{}, err
	}
	artifact, err := BuildReplayMatrixCIResult(manifest.Cases, sourceRoot)
	if err != nil {
		return ciresult.Artifact{}, err
	}
	if err := bindReplayMatrixManifest(&artifact, manifestPath, sourceRoot); err != nil {
		return ciresult.Artifact{}, err
	}
	return artifact, nil
}

// BuildReplayMatrixCIErrorResultFromManifest preserves the manifest hash when
// an input member cannot be loaded or the manifest itself cannot be parsed.
func BuildReplayMatrixCIErrorResultFromManifest(manifestPath, sourceRoot string, cause error) (ciresult.Artifact, error) {
	if cause == nil {
		return ciresult.Artifact{}, fmt.Errorf("replay matrix CI error result requires an error")
	}
	var cases []ReplayMatrixCase
	if manifest, err := LoadReplayMatrixManifest(replayMatrixManifestPath(manifestPath, sourceRoot)); err == nil {
		cases = manifest.Cases
	}
	artifact, err := BuildReplayMatrixCIErrorResult(cases, sourceRoot, cause)
	if err != nil {
		return ciresult.Artifact{}, err
	}
	if err := bindReplayMatrixManifest(&artifact, manifestPath, sourceRoot); err != nil {
		if _, statErr := os.Stat(replayMatrixManifestPath(manifestPath, sourceRoot)); statErr != nil {
			return artifact, nil
		}
		return ciresult.Artifact{}, err
	}
	return artifact, nil
}

func bindReplayMatrixManifest(artifact *ciresult.Artifact, manifestPath, sourceRoot string) error {
	if strings.TrimSpace(manifestPath) == "" {
		return fmt.Errorf("replay matrix manifest path must not be empty")
	}
	hash, err := hashFile(replayMatrixManifestPath(manifestPath, sourceRoot))
	if err != nil {
		return fmt.Errorf("hash replay matrix manifest: %w", err)
	}
	if artifact.Inputs == nil {
		artifact.Inputs = make(map[string]ciresult.FileRef)
	}
	artifact.Inputs["matrix_manifest"] = ciresult.FileRef{Path: manifestPath, SHA256: hash}
	if err := artifact.Validate(); err != nil {
		return fmt.Errorf("validate manifest-bound replay matrix CI result: %w", err)
	}
	return nil
}
