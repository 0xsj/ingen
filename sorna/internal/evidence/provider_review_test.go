package evidence

import (
	"encoding/json"
	"strings"
	"testing"

	"ingen/sorna/internal/campaign"
)

func TestBuildProviderReviewCIResultPreservesReviewAndInputs(t *testing.T) {
	review := campaign.ProviderReview{
		Schema: campaign.ProviderReviewSchema,
		Status: "ready",
		Plan:   campaign.ProviderReviewPlan{Path: "plan.json", SHA256: strings.Repeat("a", 64)},
		Provider: campaign.ProviderReviewProvider{
			Path: "provider.yaml", SHA256: strings.Repeat("b", 64), ID: "provider", Version: 1, PlanBinding: "matched",
		},
	}
	artifact, err := BuildProviderReviewCIResult(review, ".")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Tool != "sorna" || artifact.Kind != "mutation-provider-review" || artifact.Status != "passed" || artifact.ExitCode != 0 {
		t.Fatalf("artifact = %+v, want passing provider review envelope", artifact)
	}
	if artifact.Inputs["plan"].SHA256 != strings.Repeat("a", 64) || artifact.Inputs["provider"].SHA256 != strings.Repeat("b", 64) {
		t.Fatalf("artifact inputs = %+v, want plan and provider hashes", artifact.Inputs)
	}
	var decoded campaign.ProviderReview
	if err := json.Unmarshal(artifact.Report, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Status != "ready" || decoded.Provider.PlanBinding != "matched" {
		t.Fatalf("decoded report = %+v, want original provider review", decoded)
	}
}

func TestBuildProviderReviewCIResultMapsBlockedReviewToFailed(t *testing.T) {
	review := campaign.ProviderReview{
		Schema: campaign.ProviderReviewSchema,
		Status: "blocked",
		Plan:   campaign.ProviderReviewPlan{Path: "plan.json", SHA256: strings.Repeat("a", 64)},
		Provider: campaign.ProviderReviewProvider{
			Path: "provider.yaml", SHA256: strings.Repeat("b", 64), ID: "provider", Version: 1, PlanBinding: "mismatch",
		},
	}
	artifact, err := BuildProviderReviewCIResult(review, ".")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Status != "failed" || artifact.ExitCode != 1 {
		t.Fatalf("artifact = %+v, want failed provider review envelope", artifact)
	}
	var explanation providerReviewExplanationPayload
	if err := json.Unmarshal(artifact.Explanation, &explanation); err != nil {
		t.Fatal(err)
	}
	if len(explanation.BlockedChecks) != 1 || explanation.PlanBinding != "mismatch" {
		t.Fatalf("explanation = %+v, want plan mismatch reason", explanation)
	}
}

func TestBuildProviderReviewCIResultExplainsRequiredUnboundBinding(t *testing.T) {
	review := campaign.ProviderReview{
		Schema:             campaign.ProviderReviewSchema,
		Status:             "blocked",
		RequirePlanBinding: true,
		Plan:               campaign.ProviderReviewPlan{Path: "plan.json", SHA256: strings.Repeat("a", 64)},
		Provider: campaign.ProviderReviewProvider{
			Path: "provider.yaml", SHA256: strings.Repeat("b", 64), ID: "provider", Version: 1, PlanBinding: "unbound",
		},
	}
	artifact, err := BuildProviderReviewCIResult(review, ".")
	if err != nil {
		t.Fatal(err)
	}
	var explanation providerReviewExplanationPayload
	if err := json.Unmarshal(artifact.Explanation, &explanation); err != nil {
		t.Fatal(err)
	}
	if !explanation.PlanBindingRequired || len(explanation.BlockedChecks) != 1 || !strings.Contains(explanation.BlockedChecks[0], "required") {
		t.Fatalf("explanation = %+v, want required unbound binding reason", explanation)
	}
}
