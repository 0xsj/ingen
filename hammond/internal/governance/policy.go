package governance

import (
	"fmt"
	"strings"
)

// ReviewPolicy controls when an active review cycle materializes approved
// state. Identity and role authorization remain outside this value for now.
type ReviewPolicy struct {
	Reference        PolicyReference
	MinimumApprovals int
	RequiredRoles    []string
	ActorRoles       map[string][]string
}

// DefaultReviewPolicy is the explicit v1 policy used by the local store and
// CLI: one approval from one distinct actor.
func DefaultReviewPolicy() ReviewPolicy {
	return ReviewPolicy{
		Reference: PolicyReference{
			ID:      "single-approval",
			Version: 1,
			Schema:  PolicySchema,
			Artifact: Artifact{
				URI:    "hammond/examples/review-policy-v1.json",
				SHA256: "3b823defcb99297bb05f7ca36e126d8f0ca376ef3fb507cbefe43f6c3e2bfa13",
			},
		},
		MinimumApprovals: 1,
		ActorRoles: map[string][]string{
			"reviewer":              {"product-reviewer"},
			"reviewer@example.test": {"product-reviewer"},
		},
	}
}

func (policy ReviewPolicy) Validate() error {
	if strings.TrimSpace(policy.Reference.ID) == "" {
		return fmt.Errorf("policy id is required")
	}
	if policy.Reference.Version < 1 {
		return fmt.Errorf("policy version must be positive")
	}
	if policy.Reference.Schema != PolicySchema {
		return fmt.Errorf("policy schema must be %s", PolicySchema)
	}
	if strings.TrimSpace(policy.Reference.Artifact.URI) == "" {
		return fmt.Errorf("policy artifact uri is required")
	}
	if !validSHA256(policy.Reference.Artifact.SHA256) {
		return fmt.Errorf("policy artifact sha256 must be a lowercase SHA-256 digest")
	}
	if policy.MinimumApprovals < 1 {
		return fmt.Errorf("minimum approvals must be positive")
	}
	seenRoles := make(map[string]struct{}, len(policy.RequiredRoles))
	for _, role := range policy.RequiredRoles {
		role = strings.TrimSpace(role)
		if role == "" {
			return fmt.Errorf("required roles must not be empty")
		}
		if _, exists := seenRoles[role]; exists {
			return fmt.Errorf("required role %q is duplicated", role)
		}
		seenRoles[role] = struct{}{}
	}
	for actor, roles := range policy.ActorRoles {
		if strings.TrimSpace(actor) == "" {
			return fmt.Errorf("actor role grants must not contain an empty actor")
		}
		seenActorRoles := make(map[string]struct{}, len(roles))
		for _, role := range roles {
			role = strings.TrimSpace(role)
			if role == "" {
				return fmt.Errorf("actor %q has an empty role grant", actor)
			}
			if _, exists := seenActorRoles[role]; exists {
				return fmt.Errorf("actor %q has duplicated role %q", actor, role)
			}
			seenActorRoles[role] = struct{}{}
		}
	}
	return nil
}

func (policy ReviewPolicy) authorizes(actor, role string) bool {
	if len(policy.ActorRoles) == 0 {
		return true
	}
	roles, exists := policy.ActorRoles[strings.TrimSpace(actor)]
	if !exists {
		return false
	}
	role = strings.TrimSpace(role)
	for _, grantedRole := range roles {
		if strings.TrimSpace(grantedRole) == role {
			return true
		}
	}
	return false
}

func (policy ReviewPolicy) satisfied(events []Event, cycleID string) bool {
	if policy.MinimumApprovals < 1 || strings.TrimSpace(cycleID) == "" {
		return false
	}

	actors := make(map[string]struct{})
	roles := make(map[string]struct{})
	for _, event := range events {
		if event.Type != EventApprovalRecorded || event.ReviewCycleID != cycleID || event.Decision != DecisionApprove {
			continue
		}
		actor := strings.TrimSpace(event.Actor)
		role := strings.TrimSpace(event.Role)
		if actor != "" && policy.authorizes(actor, role) {
			actors[actor] = struct{}{}
		}
		if role != "" && policy.authorizes(actor, role) {
			roles[role] = struct{}{}
		}
	}
	if len(actors) < policy.MinimumApprovals {
		return false
	}
	for _, requiredRole := range policy.RequiredRoles {
		if _, exists := roles[strings.TrimSpace(requiredRole)]; !exists {
			return false
		}
	}
	return true
}

func activeReviewCycle(events []Event) string {
	cycleID := ""
	for _, event := range events {
		if event.Type == EventReviewOpened && strings.TrimSpace(event.ReviewCycleID) != "" {
			cycleID = event.ReviewCycleID
		}
	}
	return cycleID
}
