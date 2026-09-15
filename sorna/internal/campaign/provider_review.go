package campaign

// ProviderReviewSchema is the versioned JSON shape for a no-execution review
// of a mutation provider against a campaign plan.
const ProviderReviewSchema = "ingen.mutation-provider-review/v1"

// ProviderReviewInput identifies the exact bytes reviewed by BuildProviderReview.
type ProviderReviewInput struct {
	PlanPath           string
	PlanSHA256         string
	ProviderPath       string
	ProviderSHA256     string
	RequirePlanBinding bool
	Plan               Plan
	Provider           ProviderManifest
}

// ProviderReview is a human- and CI-readable capability review. It describes
// why a provider is ready or blocked without starting any subject process.
type ProviderReview struct {
	Schema             string                   `json:"schema"`
	Status             string                   `json:"status"`
	RequirePlanBinding bool                     `json:"require_plan_binding,omitempty"`
	Plan               ProviderReviewPlan       `json:"plan"`
	Provider           ProviderReviewProvider   `json:"provider"`
	Capabilities       []ProviderCapability     `json:"capabilities"`
	Mutations          []ProviderMutationReview `json:"mutations"`
}

type ProviderReviewPlan struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type ProviderReviewProvider struct {
	Path        string `json:"path"`
	SHA256      string `json:"sha256"`
	ID          string `json:"id"`
	Version     int64  `json:"version"`
	PlanSHA256  string `json:"plan_sha256,omitempty"`
	PlanBinding string `json:"plan_binding"`
}

type ProviderMutationReview struct {
	Sequence         int    `json:"sequence"`
	MutationID       string `json:"mutation_id"`
	Plane            string `json:"plane"`
	Operator         string `json:"operator"`
	Target           string `json:"target"`
	EntryStatus      string `json:"entry_status"`
	CapabilityStatus string `json:"capability_status"`
	Status           string `json:"status"`
}

// BuildProviderReview compares the provider's declared capabilities and
// prepared entries with a plan. It does not inspect or execute subject code.
func BuildProviderReview(input ProviderReviewInput) ProviderReview {
	review := ProviderReview{
		Schema:             ProviderReviewSchema,
		Status:             "ready",
		RequirePlanBinding: input.RequirePlanBinding,
		Plan: ProviderReviewPlan{
			Path:   input.PlanPath,
			SHA256: input.PlanSHA256,
		},
		Provider: ProviderReviewProvider{
			Path:        input.ProviderPath,
			SHA256:      input.ProviderSHA256,
			ID:          input.Provider.ID,
			Version:     input.Provider.Version,
			PlanSHA256:  input.Provider.PlanSHA256,
			PlanBinding: "unbound",
		},
		Capabilities: append([]ProviderCapability(nil), input.Provider.Capabilities...),
		Mutations:    make([]ProviderMutationReview, 0, len(input.Plan.Mutations)),
	}
	if input.Provider.PlanSHA256 != "" {
		if input.Provider.PlanSHA256 == input.PlanSHA256 {
			review.Provider.PlanBinding = "matched"
		} else {
			review.Provider.PlanBinding = "mismatch"
			review.Status = "blocked"
		}
	} else if input.RequirePlanBinding {
		review.Status = "blocked"
	}

	entries := make(map[string]bool, len(input.Provider.Entries))
	for _, entry := range input.Provider.Entries {
		entries[entry.MutationID] = true
	}
	for _, planned := range input.Plan.Mutations {
		entryStatus := "present"
		if !entries[planned.Spec.ID] {
			entryStatus = "missing"
		}
		capabilityStatus := "declared"
		if !input.Provider.Supports(planned.Spec) {
			capabilityStatus = "undeclared"
		}
		status := "supported"
		if entryStatus != "present" || capabilityStatus != "declared" {
			status = "blocked"
			review.Status = "blocked"
		}
		review.Mutations = append(review.Mutations, ProviderMutationReview{
			Sequence:         planned.Sequence,
			MutationID:       planned.Spec.ID,
			Plane:            planned.Spec.Plane,
			Operator:         planned.Spec.Operator,
			Target:           planned.Spec.Target,
			EntryStatus:      entryStatus,
			CapabilityStatus: capabilityStatus,
			Status:           status,
		})
	}
	return review
}
