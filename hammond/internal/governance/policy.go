package governance

import (
	"fmt"
	"strings"
)

// AuthorityVerifier is the runtime seam for checking whether an actor may use
// a role at a decision timestamp. Implementations may consult a local
// snapshot or an external organization-backed authority source.
type AuthorityVerifier interface {
	Verify(actor, role, at string) (bool, error)
}

// ReviewPolicy controls when an active review cycle materializes approved
// state. Authority, when present, points to a separately versioned local
// actor-to-role snapshot; it is not an external identity proof.
type ReviewPolicy struct {
	Reference         PolicyReference
	MinimumApprovals  int
	RequiredRoles     []string
	Authority         AuthorityReference
	AuthorityVerifier AuthorityVerifier
	// ActorRoles is retained as the materialized local snapshot for audit and
	// compatibility. New runtime authority sources should use AuthorityVerifier.
	ActorRoles map[string][]string
}

type AuthoritySignature struct {
	Algorithm string `json:"algorithm"`
	KeyID     string `json:"key_id"`
	Signature string `json:"signature"`
}

type ReviewAuthority struct {
	Reference AuthorityReference
	Actors    map[string][]string
	Signature *AuthoritySignature
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
				SHA256: "c80517e8aa51ca1715fc5e1214b2222aa99ed5fa286aee9f7340814c39caac10",
			},
		},
		MinimumApprovals: 1,
		Authority: AuthorityReference{
			ID:      "local-reviewers",
			Version: 1,
			Schema:  AuthoritySchema,
			Artifact: Artifact{
				URI:    "hammond/examples/review-authority-v1.json",
				SHA256: "f478a9d1b32421f22645b2f374feb8cbcf5978aec2ef3317605e92d8322e3c62",
			},
		},
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
	if !isEmptyAuthorityReference(policy.Authority) {
		if problems := validateAuthorityReference(policy.Authority, "authority"); len(problems) > 0 {
			return fmt.Errorf("%s", strings.Join(problems, "; "))
		}
	}
	if err := validateRoleList(policy.RequiredRoles, "required roles"); err != nil {
		return err
	}
	return validateActorRoleGrants(policy.ActorRoles)
}

func (authority ReviewAuthority) Validate() error {
	if problems := validateAuthorityReference(authority.Reference, "authority"); len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	if authority.Actors == nil {
		return fmt.Errorf("authority actors are required")
	}
	if err := validateAuthoritySignature(authority.Signature); err != nil {
		return err
	}
	return validateActorRoleGrants(authority.Actors)
}

// Verify implements AuthorityVerifier for a verified local authority snapshot.
func (authority ReviewAuthority) Verify(actor, role, _ string) (bool, error) {
	roles, exists := authority.Actors[strings.TrimSpace(actor)]
	if !exists {
		return false, nil
	}
	role = strings.TrimSpace(role)
	for _, grantedRole := range roles {
		if strings.TrimSpace(grantedRole) == role {
			return true, nil
		}
	}
	return false, nil
}

func validateRoleList(roles []string, label string) error {
	seenRoles := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		role = strings.TrimSpace(role)
		if role == "" {
			return fmt.Errorf("%s must not be empty", label)
		}
		if _, exists := seenRoles[role]; exists {
			return fmt.Errorf("%s role %q is duplicated", label, role)
		}
		seenRoles[role] = struct{}{}
	}
	return nil
}

func validateActorRoleGrants(actorRoles map[string][]string) error {
	for actor, roles := range actorRoles {
		if strings.TrimSpace(actor) == "" {
			return fmt.Errorf("actor role grants must not contain an empty actor")
		}
		if err := validateRoleList(roles, fmt.Sprintf("actor %q", actor)); err != nil {
			return err
		}
	}
	return nil
}

func (policy ReviewPolicy) authorizes(actor, role, at string) (bool, error) {
	if _, err := parseUTC(at); err != nil {
		return false, fmt.Errorf("decision timestamp must be validated before authority verification: %w", err)
	}
	if policy.AuthorityVerifier != nil {
		return policy.AuthorityVerifier.Verify(actor, role, at)
	}
	if isEmptyAuthorityReference(policy.Authority) && len(policy.ActorRoles) == 0 {
		return true, nil
	}
	return ReviewAuthority{Actors: policy.ActorRoles}.Verify(actor, role, at)
}

func (policy ReviewPolicy) hasAuthorityVerifier() bool {
	return policy.AuthorityVerifier != nil || !isEmptyAuthorityReference(policy.Authority) || len(policy.ActorRoles) > 0
}

func (policy ReviewPolicy) satisfied(events []Event, cycleID string) (bool, error) {
	if policy.MinimumApprovals < 1 || strings.TrimSpace(cycleID) == "" {
		return false, nil
	}

	actors := make(map[string]struct{})
	roles := make(map[string]struct{})
	for _, event := range events {
		if event.Type != EventApprovalRecorded || event.ReviewCycleID != cycleID || event.Decision != DecisionApprove {
			continue
		}
		actor := strings.TrimSpace(event.Actor)
		role := strings.TrimSpace(event.Role)
		authorized, err := policy.authorizes(actor, role, event.At)
		if err != nil {
			return false, fmt.Errorf("verify actor %q for role %q: %w", actor, role, err)
		}
		if authorized && actor != "" {
			actors[actor] = struct{}{}
		}
		if authorized && role != "" {
			roles[role] = struct{}{}
		}
	}
	if len(actors) < policy.MinimumApprovals {
		return false, nil
	}
	for _, requiredRole := range policy.RequiredRoles {
		if _, exists := roles[strings.TrimSpace(requiredRole)]; !exists {
			return false, nil
		}
	}
	return true, nil
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
