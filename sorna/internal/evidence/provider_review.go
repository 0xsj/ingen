package evidence

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ingen/core/ciresult"
	"ingen/sorna/internal/campaign"
)

// BuildProviderReviewCIResult adapts a no-execution provider review to the
// shared CI envelope. The review remains the producer-owned report; the
// envelope only exposes its status and exact plan/provider inputs.
func BuildProviderReviewCIResult(review campaign.ProviderReview, sourceRoot string) (ciresult.Artifact, error) {
	var status string
	var exitCode int
	switch review.Status {
	case "ready":
		status = "passed"
		exitCode = 0
	case "blocked":
		status = "failed"
		exitCode = 1
	default:
		return ciresult.Artifact{}, fmt.Errorf("provider review has unsupported status %q", review.Status)
	}
	if strings.TrimSpace(sourceRoot) == "" {
		sourceRoot = "."
	}
	report, err := json.Marshal(review)
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("encode provider review report: %w", err)
	}
	explanation, err := json.Marshal(buildProviderReviewExplanation(review))
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("encode provider review explanation: %w", err)
	}
	artifact := ciresult.Artifact{
		Schema:    ciresult.Schema,
		Tool:      "sorna",
		Kind:      "mutation-provider-review",
		Status:    status,
		ExitCode:  exitCode,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Source:    ciresult.Source{Root: sourceRoot},
		Inputs: map[string]ciresult.FileRef{
			"plan":     {Path: review.Plan.Path, SHA256: review.Plan.SHA256},
			"provider": {Path: review.Provider.Path, SHA256: review.Provider.SHA256},
		},
		Report:      report,
		Explanation: explanation,
	}
	if err := artifact.Validate(); err != nil {
		return ciresult.Artifact{}, fmt.Errorf("validate provider review CI result: %w", err)
	}
	return artifact, nil
}

type providerReviewExplanationPayload struct {
	Schema              string   `json:"schema"`
	Status              string   `json:"status"`
	PlanBinding         string   `json:"plan_binding"`
	PlanBindingRequired bool     `json:"plan_binding_required,omitempty"`
	BlockedChecks       []string `json:"blocked_checks,omitempty"`
}

func buildProviderReviewExplanation(review campaign.ProviderReview) providerReviewExplanationPayload {
	explanation := providerReviewExplanationPayload{
		Schema:              "sorna.provider-review-explanation/v1",
		Status:              review.Status,
		PlanBinding:         review.Provider.PlanBinding,
		PlanBindingRequired: review.RequirePlanBinding,
	}
	if review.Provider.PlanBinding == "mismatch" {
		explanation.BlockedChecks = append(explanation.BlockedChecks, "provider plan hash does not match the reviewed plan")
	} else if review.RequirePlanBinding && review.Provider.PlanBinding == "unbound" {
		explanation.BlockedChecks = append(explanation.BlockedChecks, "provider plan hash is required but was not declared")
	}
	for _, mutation := range review.Mutations {
		if mutation.Status != "supported" {
			explanation.BlockedChecks = append(explanation.BlockedChecks, fmt.Sprintf("mutation %s: entry=%s capability=%s", mutation.MutationID, mutation.EntryStatus, mutation.CapabilityStatus))
		}
	}
	return explanation
}
