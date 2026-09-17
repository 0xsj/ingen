package campaign

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"ingen/sorna/internal/mutation"
)

func TestProviderConformanceFixtures(t *testing.T) {
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() did not return the test source path")
	}
	fixtureRoot := filepath.Join(filepath.Dir(sourcePath), "..", "..", "testdata", "provider-conformance")
	plan := conformancePlan()
	planHash := strings.Repeat("d", 64)

	tests := []struct {
		name                 string
		file                 string
		wantLoadError        string
		wantStatus           string
		wantPlanBinding      string
		wantSemanticBinding  string
		wantMutationStatuses []string
	}{
		{
			name:                 "valid",
			file:                 "valid.yaml",
			wantStatus:           "ready",
			wantPlanBinding:      "unbound",
			wantSemanticBinding:  "unbound",
			wantMutationStatuses: []string{"supported", "supported"},
		},
		{
			name:                 "exact-plan-drift",
			file:                 "exact-plan-drift.yaml",
			wantStatus:           "blocked",
			wantPlanBinding:      "mismatch",
			wantSemanticBinding:  "unbound",
			wantMutationStatuses: []string{"supported", "supported"},
		},
		{
			name:                 "semantic-plan-drift",
			file:                 "semantic-plan-drift.yaml",
			wantStatus:           "blocked",
			wantPlanBinding:      "unbound",
			wantSemanticBinding:  "mismatch",
			wantMutationStatuses: []string{"supported", "supported"},
		},
		{
			name:                 "unsupported-capability",
			file:                 "unsupported-capability.yaml",
			wantStatus:           "blocked",
			wantPlanBinding:      "unbound",
			wantSemanticBinding:  "unbound",
			wantMutationStatuses: []string{"blocked", "blocked"},
		},
		{
			name:                 "partial-preparation",
			file:                 "partial-preparation.yaml",
			wantStatus:           "blocked",
			wantPlanBinding:      "unbound",
			wantSemanticBinding:  "unbound",
			wantMutationStatuses: []string{"supported", "blocked"},
		},
		{
			name:          "malformed-entry",
			file:          "malformed-entry.yaml",
			wantLoadError: "command must be non-empty",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider, err := LoadProviderFile(filepath.Join(fixtureRoot, test.file))
			if test.wantLoadError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantLoadError) {
					t.Fatalf("LoadProviderFile() = %v, want error containing %q", err, test.wantLoadError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			review := BuildProviderReview(ProviderReviewInput{
				PlanPath:       "plan.json",
				PlanSHA256:     planHash,
				ProviderPath:   filepath.Join(fixtureRoot, test.file),
				ProviderSHA256: strings.Repeat("e", 64),
				Plan:           plan,
				Provider:       provider,
			})
			if review.Status != test.wantStatus {
				t.Fatalf("review status = %q, want %q", review.Status, test.wantStatus)
			}
			if review.Provider.PlanBinding != test.wantPlanBinding {
				t.Fatalf("plan binding = %q, want %q", review.Provider.PlanBinding, test.wantPlanBinding)
			}
			if review.Provider.SemanticBinding != test.wantSemanticBinding {
				t.Fatalf("semantic binding = %q, want %q", review.Provider.SemanticBinding, test.wantSemanticBinding)
			}
			if len(review.Mutations) != len(test.wantMutationStatuses) {
				t.Fatalf("mutation reviews = %+v, want statuses %v", review.Mutations, test.wantMutationStatuses)
			}
			for index, wantStatus := range test.wantMutationStatuses {
				if review.Mutations[index].Status != wantStatus {
					t.Fatalf("mutation review %d = %+v, want status %q", index, review.Mutations[index], wantStatus)
				}
			}
		})
	}
}

func conformancePlan() Plan {
	plan := validReviewPlan()
	plan.Mutations = []MutationEntry{
		{
			Sequence: 1,
			Spec: mutation.Spec{
				ID: "status-200-create", Plane: "implementation", Operator: "response.status.replace", Target: "POST /documents",
				Description: "change accepted status", Change: map[string]any{"from": 202, "to": 200}, ExpectedRuleIDs: []string{"rule-1"}, Status: "candidate",
			},
		},
		{
			Sequence: 2,
			Spec: mutation.Spec{
				ID: "remove-name-create", Plane: "implementation", Operator: "response.field.remove", Target: "POST /documents",
				Description: "remove required name", Change: map[string]any{"field": "name"}, ExpectedRuleIDs: []string{"rule-1"}, Status: "candidate",
			},
		},
	}
	return plan
}
