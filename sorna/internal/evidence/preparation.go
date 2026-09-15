package evidence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ingen/core/ciresult"
	"ingen/sorna/internal/campaign"
)

const mutationPreparationExplanationSchema = "sorna.mutation-preparation-explanation/v1"

// BuildMutationPreparationCIResult adapts a validated provider preparation
// summary to the shared CI envelope. It binds the summary to the exact
// provider manifest and plan bytes without interpreting source-language code.
func BuildMutationPreparationCIResult(summaryPath, providerPath, sourceRoot string) (ciresult.Artifact, error) {
	if strings.TrimSpace(summaryPath) == "" || strings.TrimSpace(providerPath) == "" {
		return ciresult.Artifact{}, fmt.Errorf("preparation summary and provider paths must not be empty")
	}
	if strings.TrimSpace(sourceRoot) == "" {
		sourceRoot = "."
	}
	summaryBytes, err := os.ReadFile(summaryPath)
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("read preparation summary: %w", err)
	}
	summary, err := campaign.LoadPreparationBytes(summaryPath, summaryBytes)
	if err != nil {
		return ciresult.Artifact{}, err
	}
	providerBytes, err := os.ReadFile(providerPath)
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("read preparation provider: %w", err)
	}
	provider, err := campaign.LoadProviderBytes(providerPath, providerBytes)
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("load preparation provider: %w", err)
	}
	if provider.ID != summary.ProviderID {
		return ciresult.Artifact{}, fmt.Errorf("preparation provider ID %q does not match summary provider ID %q", provider.ID, summary.ProviderID)
	}
	if provider.PlanSHA256 != summary.PlanSHA256 {
		return ciresult.Artifact{}, fmt.Errorf("preparation provider plan hash %q does not match summary %q", provider.PlanSHA256, summary.PlanSHA256)
	}
	planPath := summary.PlanPath
	if !filepath.IsAbs(planPath) && sourceRoot != "." {
		planPath = filepath.Join(sourceRoot, planPath)
	}
	planHash, err := campaign.HashFile(planPath)
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("hash preparation plan: %w", err)
	}
	if planHash != summary.PlanSHA256 {
		return ciresult.Artifact{}, fmt.Errorf("preparation plan hash %q does not match summary %q", planHash, summary.PlanSHA256)
	}
	if err := validatePreparationBinding(summary, provider); err != nil {
		return ciresult.Artifact{}, err
	}
	changedFiles := 0
	for _, variant := range summary.Variants {
		changedFiles += len(variant.ChangedFiles)
	}
	explanation, err := json.Marshal(mutationPreparationExplanationPayload{
		Schema:           mutationPreparationExplanationSchema,
		Status:           "ready",
		ProviderID:       summary.ProviderID,
		VariantCount:     len(summary.Variants),
		ChangedFileCount: changedFiles,
	})
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("encode preparation explanation: %w", err)
	}
	artifact := ciresult.Artifact{
		Schema:    ciresult.Schema,
		Tool:      "sorna",
		Kind:      "mutation-preparation",
		Status:    "passed",
		ExitCode:  0,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Source:    ciresult.Source{Root: sourceRoot},
		Inputs: map[string]ciresult.FileRef{
			"preparation": {Path: summaryPath, SHA256: campaign.HashBytes(summaryBytes)},
			"provider":    {Path: providerPath, SHA256: campaign.HashBytes(providerBytes)},
			"plan":        {Path: summary.PlanPath, SHA256: summary.PlanSHA256},
		},
		Report:      summaryBytes,
		Explanation: explanation,
	}
	if err := artifact.Validate(); err != nil {
		return ciresult.Artifact{}, fmt.Errorf("validate preparation CI result: %w", err)
	}
	return artifact, nil
}

// BuildMutationPreparationCIErrorResult produces a collector-friendly error
// envelope when a preparation summary cannot be validated or bound.
func BuildMutationPreparationCIErrorResult(summaryPath, providerPath, sourceRoot string, cause error) (ciresult.Artifact, error) {
	if cause == nil {
		return ciresult.Artifact{}, fmt.Errorf("preparation CI error result requires an error")
	}
	if strings.TrimSpace(sourceRoot) == "" {
		sourceRoot = "."
	}
	artifact := ciresult.Artifact{
		Schema:    ciresult.Schema,
		Tool:      "sorna",
		Kind:      "mutation-preparation",
		Status:    "error",
		ExitCode:  2,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Source:    ciresult.Source{Root: sourceRoot},
		Error:     cause.Error(),
		Inputs:    make(map[string]ciresult.FileRef),
	}
	if strings.TrimSpace(summaryPath) != "" {
		if hash, err := campaign.HashFile(summaryPath); err == nil {
			artifact.Inputs["preparation"] = ciresult.FileRef{Path: summaryPath, SHA256: hash}
		}
	}
	if strings.TrimSpace(providerPath) != "" {
		if hash, err := campaign.HashFile(providerPath); err == nil {
			artifact.Inputs["provider"] = ciresult.FileRef{Path: providerPath, SHA256: hash}
		}
	}
	if err := artifact.Validate(); err != nil {
		return ciresult.Artifact{}, fmt.Errorf("validate preparation CI error result: %w", err)
	}
	return artifact, nil
}

type mutationPreparationExplanationPayload struct {
	Schema           string `json:"schema"`
	Status           string `json:"status"`
	ProviderID       string `json:"provider_id"`
	VariantCount     int    `json:"variant_count"`
	ChangedFileCount int    `json:"changed_file_count"`
}

func validatePreparationBinding(summary campaign.PreparationSummary, provider campaign.ProviderManifest) error {
	entries := make(map[string]campaign.ProviderEntry, len(provider.Entries))
	for _, entry := range provider.Entries {
		entries[entry.MutationID] = entry
	}
	for _, variant := range summary.Variants {
		entry, ok := entries[variant.MutationID]
		if !ok {
			return fmt.Errorf("preparation provider is missing variant %q", variant.MutationID)
		}
		if entry.Command != variant.BinaryPath {
			return fmt.Errorf("preparation variant %q binary path does not match provider entry", variant.MutationID)
		}
		if entry.Provenance == nil {
			return fmt.Errorf("preparation provider entry %q has no provenance", variant.MutationID)
		}
		if entry.Provenance.SourceDir != variant.SourceDir || entry.Provenance.SourceSHA256 != variant.SourceSHA256 || entry.Provenance.BinarySHA256 != variant.BinarySHA256 {
			return fmt.Errorf("preparation variant %q provenance does not match provider entry", variant.MutationID)
		}
	}
	return nil
}
