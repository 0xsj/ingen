package campaign

import (
	"strings"
	"testing"

	"ingen/sorna/internal/mutation"
)

func TestBuildProviderReviewReportsBlockedMutationCapability(t *testing.T) {
	plan := reviewPlan()
	provider := ProviderManifest{
		ID:         "review-provider",
		Version:    1,
		PlanSHA256: strings.Repeat("a", 64),
		Capabilities: []ProviderCapability{{
			Plane: "implementation", Operator: "response.status.replace", Target: "POST /documents",
		}},
		Entries: []ProviderEntry{{MutationID: "status-200-create", Command: "subject"}},
	}
	review := BuildProviderReview(ProviderReviewInput{
		PlanPath:       "plan.json",
		PlanSHA256:     strings.Repeat("b", 64),
		ProviderPath:   "provider.yaml",
		ProviderSHA256: strings.Repeat("c", 64),
		Plan:           plan,
		Provider:       provider,
	})
	if review.Schema != ProviderReviewSchema || review.Status != "blocked" || review.Provider.PlanBinding != "mismatch" {
		t.Fatalf("review = %+v, want blocked mismatched review", review)
	}
	if len(review.Mutations) != 2 || review.Mutations[0].Status != "supported" || review.Mutations[1].EntryStatus != "missing" || review.Mutations[1].CapabilityStatus != "undeclared" || review.Mutations[1].Status != "blocked" {
		t.Fatalf("mutation review = %+v, want one supported and one blocked mutation", review.Mutations)
	}
}

func TestBuildProviderReviewReportsReadyUnboundFixture(t *testing.T) {
	plan := reviewPlan()
	provider := ProviderManifest{
		ID:      "fixture-provider",
		Version: 1,
		Capabilities: []ProviderCapability{
			{Plane: "implementation", Operator: "response.status.replace", Target: "POST /documents"},
			{Plane: "implementation", Operator: "response.field.remove", Target: "POST /documents"},
		},
		Entries: []ProviderEntry{
			{MutationID: "status-200-create", Command: "status-subject"},
			{MutationID: "remove-name-create", Command: "field-subject"},
		},
	}
	review := BuildProviderReview(ProviderReviewInput{Plan: plan, PlanSHA256: strings.Repeat("d", 64), Provider: provider})
	if review.Status != "ready" || review.Provider.PlanBinding != "unbound" {
		t.Fatalf("review = %+v, want ready unbound fixture review", review)
	}
	for _, mutationReview := range review.Mutations {
		if mutationReview.Status != "supported" {
			t.Fatalf("mutation review = %+v, want supported", mutationReview)
		}
	}
}

func TestBuildProviderReviewBlocksUnboundProviderWhenBindingIsRequired(t *testing.T) {
	review := BuildProviderReview(ProviderReviewInput{
		Plan:               reviewPlan(),
		PlanSHA256:         strings.Repeat("d", 64),
		RequirePlanBinding: true,
		Provider: ProviderManifest{
			ID: "fixture-provider", Version: 1,
			Capabilities: []ProviderCapability{
				{Plane: "implementation", Operator: "response.status.replace", Target: "POST /documents"},
				{Plane: "implementation", Operator: "response.field.remove", Target: "POST /documents"},
			},
			Entries: []ProviderEntry{
				{MutationID: "status-200-create", Command: "status-subject"},
				{MutationID: "remove-name-create", Command: "field-subject"},
			},
		},
	})
	if review.Status != "blocked" || !review.RequirePlanBinding || review.Provider.PlanBinding != "unbound" {
		t.Fatalf("review = %+v, want blocked required-binding review", review)
	}
}

func reviewPlan() Plan {
	return Plan{Mutations: []MutationEntry{
		{Sequence: 1, Spec: mutation.Spec{ID: "status-200-create", Plane: "implementation", Operator: "response.status.replace", Target: "POST /documents"}},
		{Sequence: 2, Spec: mutation.Spec{ID: "remove-name-create", Plane: "implementation", Operator: "response.field.remove", Target: "POST /documents"}},
	}}
}
