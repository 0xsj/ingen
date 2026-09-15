package campaign

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PreparationSummarySchema identifies the language-neutral provider
// preparation artifact emitted before mutation execution.
const PreparationSummarySchema = "ingen.mutation-preparation/v1"

// PreparationVariant is the stable, path-relative review record for one
// prepared mutation. ChangedFiles identifies what moved in the isolated copy;
// the retained source variant remains available for a full diff.
type PreparationVariant struct {
	Sequence     int                 `json:"sequence"`
	MutationID   string              `json:"mutation_id"`
	SourceDir    string              `json:"source_dir"`
	SourceSHA256 string              `json:"source_sha256"`
	BinaryPath   string              `json:"binary_path"`
	BinarySHA256 string              `json:"binary_sha256"`
	ChangedFiles []string            `json:"changed_files"`
	Provenance   *ProviderProvenance `json:"provenance,omitempty"`
}

// PreparationSummary is a compact review artifact separate from the
// executable provider manifest. It records what a provider prepared without
// asking the campaign executor or Nublar to understand source-language ASTs.
type PreparationSummary struct {
	Schema             string               `json:"schema"`
	ProviderID         string               `json:"provider_id"`
	PlanPath           string               `json:"plan_path"`
	PlanSHA256         string               `json:"plan_sha256"`
	PlanSemanticSHA256 string               `json:"plan_semantic_sha256,omitempty"`
	Variants           []PreparationVariant `json:"variants"`
}

// ValidatePreparationSummary returns structural errors in a provider
// preparation summary. The summary is intentionally stricter than legacy
// provider manifests because it is a durable review artifact.
func ValidatePreparationSummary(summary PreparationSummary) []string {
	problems := make([]string, 0)
	if summary.Schema != PreparationSummarySchema {
		problems = append(problems, fmt.Sprintf("preparation.schema must be %s", PreparationSummarySchema))
	}
	if strings.TrimSpace(summary.ProviderID) == "" {
		problems = append(problems, "preparation.provider_id must be non-empty")
	}
	if strings.TrimSpace(summary.PlanPath) == "" {
		problems = append(problems, "preparation.plan_path must be non-empty")
	}
	if !digestPattern.MatchString(summary.PlanSHA256) {
		problems = append(problems, "preparation.plan_sha256 must be a lowercase SHA-256 digest")
	}
	if strings.TrimSpace(summary.PlanSemanticSHA256) != "" && !digestPattern.MatchString(summary.PlanSemanticSHA256) {
		problems = append(problems, "preparation.plan_semantic_sha256 must be a lowercase SHA-256 digest when present")
	}
	if len(summary.Variants) == 0 {
		problems = append(problems, "preparation.variants must contain at least one variant")
	}
	seen := make(map[string]bool, len(summary.Variants))
	for index, variant := range summary.Variants {
		path := fmt.Sprintf("preparation.variants[%d]", index)
		if variant.Sequence != index+1 {
			problems = append(problems, fmt.Sprintf("%s.sequence must be %d", path, index+1))
		}
		if strings.TrimSpace(variant.MutationID) == "" {
			problems = append(problems, path+".mutation_id must be non-empty")
		} else if seen[variant.MutationID] {
			problems = append(problems, fmt.Sprintf("%s.mutation_id duplicates %q", path, variant.MutationID))
		} else {
			seen[variant.MutationID] = true
		}
		if strings.TrimSpace(variant.SourceDir) == "" {
			problems = append(problems, path+".source_dir must be non-empty")
		}
		if !digestPattern.MatchString(variant.SourceSHA256) {
			problems = append(problems, path+".source_sha256 must be a lowercase SHA-256 digest")
		}
		if strings.TrimSpace(variant.BinaryPath) == "" {
			problems = append(problems, path+".binary_path must be non-empty")
		}
		if !digestPattern.MatchString(variant.BinarySHA256) {
			problems = append(problems, path+".binary_sha256 must be a lowercase SHA-256 digest")
		}
		if len(variant.ChangedFiles) == 0 {
			problems = append(problems, path+".changed_files must contain at least one file")
		}
		if variant.Provenance == nil {
			problems = append(problems, path+".provenance must be present")
		}
	}
	return problems
}

// WritePreparationSummary writes a reviewable preparation artifact without
// replacing an existing attempt. It returns the hash of the exact bytes.
func WritePreparationSummary(path string, summary PreparationSummary) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("preparation summary output path must not be empty")
	}
	if problems := ValidatePreparationSummary(summary); len(problems) > 0 {
		return "", fmt.Errorf("invalid preparation summary: %s", strings.Join(problems, "; "))
	}
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("preparation summary already exists: %s", path)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect preparation summary: %w", err)
	}
	contents, err := CanonicalPreparationJSON(summary)
	if err != nil {
		return "", fmt.Errorf("encode preparation summary: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		return "", err
	}
	return HashBytes(contents), nil
}

// LoadPreparationSummary loads and verifies a canonical preparation summary.
func LoadPreparationSummary(path string) (PreparationSummary, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return PreparationSummary{}, err
	}
	return LoadPreparationBytes(path, contents)
}

// LoadPreparationBytes loads a preparation summary from already captured
// bytes and rejects non-canonical or structurally invalid input.
func LoadPreparationBytes(path string, contents []byte) (PreparationSummary, error) {
	var summary PreparationSummary
	if err := json.Unmarshal(contents, &summary); err != nil {
		return PreparationSummary{}, fmt.Errorf("parse preparation summary %s: %w", path, err)
	}
	if problems := ValidatePreparationSummary(summary); len(problems) > 0 {
		return PreparationSummary{}, fmt.Errorf("invalid preparation summary: %s", strings.Join(problems, "; "))
	}
	canonical, err := CanonicalPreparationJSON(summary)
	if err != nil {
		return PreparationSummary{}, err
	}
	if !bytes.Equal(contents, canonical) {
		return PreparationSummary{}, fmt.Errorf("preparation summary %s is not canonical JSON", path)
	}
	return summary, nil
}

// CanonicalPreparationJSON returns deterministic JSON bytes for a valid
// preparation summary.
func CanonicalPreparationJSON(summary PreparationSummary) ([]byte, error) {
	if problems := ValidatePreparationSummary(summary); len(problems) > 0 {
		return nil, fmt.Errorf("invalid preparation summary: %s", strings.Join(problems, "; "))
	}
	contents, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("canonicalize preparation summary: %w", err)
	}
	return append(contents, '\n'), nil
}
