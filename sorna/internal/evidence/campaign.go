package evidence

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ingen/core/ciresult"
	"ingen/sorna/internal/campaign"
)

const mutationCampaignExplanationSchema = "sorna.mutation-campaign-explanation/v1"

const (
	mutationFailureCategoryExecutionError          = "execution-error"
	mutationFailureCategoryContractInsensitive     = "contract-insensitive"
	mutationFailureCategoryInsufficientObservation = "insufficient-observation"
	mutationFailureCategoryInvalidMutant           = "invalid-mutant"
	mutationFailureCategoryEquivalentMutant        = "equivalent-mutant"
	mutationFailureCategoryExecutionTimeout        = "execution-timeout"
	mutationFailureCategoryCampaignFailure         = "campaign-failure"
)

// MutationCampaignFailure keeps the CI explanation useful without requiring
// a consumer to understand the complete producer-owned campaign result.
type MutationCampaignFailure struct {
	Sequence   int                 `json:"sequence"`
	MutationID string              `json:"mutation_id"`
	Status     string              `json:"status"`
	Outcome    string              `json:"outcome,omitempty"`
	Category   string              `json:"category"`
	Reason     string              `json:"reason,omitempty"`
	Diagnosis  *campaign.Diagnosis `json:"diagnosis,omitempty"`
}

// MutationCampaignExplanation is the compact, coordinator-facing summary of
// a campaign. The complete campaign result remains in Report.
type MutationCampaignExplanation struct {
	Schema            string                    `json:"schema"`
	Status            string                    `json:"status"`
	Summary           campaign.Summary          `json:"summary"`
	FailureCategories map[string]int            `json:"failure_categories,omitempty"`
	Failures          []MutationCampaignFailure `json:"failures,omitempty"`
}

// BuildMutationCampaignCIResult adapts a verified campaign result to the
// shared InGen CI envelope. The campaign result remains the producer-owned
// report; the envelope binds it and its plan to exact file hashes.
func BuildMutationCampaignCIResult(result campaign.Result, campaignPath, sourceRoot string) (ciresult.Artifact, error) {
	if problems := campaign.ValidateResult(result); len(problems) > 0 {
		return ciresult.Artifact{}, fmt.Errorf("invalid mutation campaign result: %s", strings.Join(problems, "; "))
	}
	if strings.TrimSpace(campaignPath) == "" {
		return ciresult.Artifact{}, fmt.Errorf("mutation campaign result path must not be empty")
	}
	if strings.TrimSpace(sourceRoot) == "" {
		sourceRoot = "."
	}
	campaignHash, err := campaign.HashFile(campaignPath)
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("hash mutation campaign result: %w", err)
	}
	report, err := json.Marshal(result)
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("encode mutation campaign report: %w", err)
	}
	explanation, err := json.Marshal(buildMutationCampaignExplanation(result))
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("encode mutation campaign explanation: %w", err)
	}
	exitCode, err := ciresult.ExitCodeForStatus(result.Status)
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("map mutation campaign status: %w", err)
	}
	artifact := ciresult.Artifact{
		Schema:    ciresult.Schema,
		Tool:      "sorna",
		Kind:      "mutation-campaign",
		Status:    result.Status,
		ExitCode:  exitCode,
		CreatedAt: result.FinishedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
		Source:    ciresult.Source{Root: sourceRoot},
		Inputs: map[string]ciresult.FileRef{
			"campaign_result": {Path: campaignPath, SHA256: campaignHash},
			"plan":            {Path: result.Plan.Path, SHA256: result.Plan.SHA256},
		},
		Report:      report,
		Explanation: explanation,
	}
	if result.Status == "error" {
		artifact.Error = "mutation campaign encountered one or more execution errors"
	}
	if err := artifact.Validate(); err != nil {
		return ciresult.Artifact{}, fmt.Errorf("validate mutation campaign CI result: %w", err)
	}
	return artifact, nil
}

// BuildMutationCampaignCIErrorResult produces a collector-friendly error
// envelope when the campaign result or its evidence cannot be verified.
func BuildMutationCampaignCIErrorResult(campaignPath, sourceRoot string, cause error) (ciresult.Artifact, error) {
	if cause == nil {
		return ciresult.Artifact{}, fmt.Errorf("mutation campaign CI error result requires an error")
	}
	if strings.TrimSpace(sourceRoot) == "" {
		sourceRoot = "."
	}
	artifact := ciresult.Artifact{
		Schema:    ciresult.Schema,
		Tool:      "sorna",
		Kind:      "mutation-campaign",
		Status:    "error",
		ExitCode:  2,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Source:    ciresult.Source{Root: sourceRoot},
		Error:     cause.Error(),
	}
	if strings.TrimSpace(campaignPath) != "" {
		if hash, err := campaign.HashFile(campaignPath); err == nil {
			artifact.Inputs = map[string]ciresult.FileRef{
				"campaign_result": {Path: campaignPath, SHA256: hash},
			}
		}
	}
	if err := artifact.Validate(); err != nil {
		return ciresult.Artifact{}, fmt.Errorf("validate mutation campaign CI error result: %w", err)
	}
	return artifact, nil
}

func buildMutationCampaignExplanation(result campaign.Result) MutationCampaignExplanation {
	explanation := MutationCampaignExplanation{
		Schema:            mutationCampaignExplanationSchema,
		Status:            result.Status,
		Summary:           result.Summary,
		FailureCategories: make(map[string]int),
	}
	for _, entry := range result.Entries {
		if entry.Status == "passed" {
			continue
		}
		category := mutationFailureCategory(entry)
		explanation.FailureCategories[category]++
		explanation.Failures = append(explanation.Failures, MutationCampaignFailure{
			Sequence:   entry.Sequence,
			MutationID: entry.MutationID,
			Status:     entry.Status,
			Outcome:    entry.Outcome,
			Category:   category,
			Reason:     entry.Reason,
			Diagnosis:  entry.Diagnosis,
		})
	}
	if len(explanation.FailureCategories) == 0 {
		explanation.FailureCategories = nil
	}
	return explanation
}

func mutationFailureCategory(entry campaign.EntryResult) string {
	if entry.Status == "error" {
		return mutationFailureCategoryExecutionError
	}
	switch entry.Outcome {
	case "survived":
		return mutationFailureCategoryContractInsensitive
	case "inconclusive":
		return mutationFailureCategoryInsufficientObservation
	case "invalid":
		return mutationFailureCategoryInvalidMutant
	case "equivalent":
		return mutationFailureCategoryEquivalentMutant
	case "timeout":
		return mutationFailureCategoryExecutionTimeout
	default:
		return mutationFailureCategoryCampaignFailure
	}
}
