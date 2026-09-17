package attestation

import (
	"context"
	"fmt"
	"time"

	"ingen/lockwood/internal/artifact"
)

const (
	AuthorizationDecisionIndeterminate = "indeterminate"
	AuthorizationFailureDenied         = "authorization-denied"
	AuthorizationFailureIndeterminate  = "authorization-indeterminate"
)

// AuthorizationVerificationRequest is the complete local context supplied to
// a caller-owned external authorization verifier.
type AuthorizationVerificationRequest struct {
	Handoff           AuthorizationHandoff
	LocalTargetDigest string
	EvaluatedAt       time.Time
}

// AuthorizationVerifier is implemented by the caller that owns external
// identity, authorization, policy, and revocation trust. Implementations may
// use network services outside Lockwood, but must return canonical evidence
// digests in the result.
type AuthorizationVerifier interface {
	VerifyAuthorization(context.Context, AuthorizationVerificationRequest) (AuthorizationVerificationResult, error)
}

// AuthorizationVerificationResult is transient evidence returned by a
// caller-owned external verification adapter. It records the adapter's
// claims, but does not itself verify external signatures or revocation data.
type AuthorizationVerificationResult struct {
	TargetDigest             string `json:"target_digest"`
	Action                   string `json:"action"`
	PrincipalReference       string `json:"principal_reference"`
	IdentityAssertionDigest  string `json:"identity_assertion_digest,omitempty"`
	IdentityVerified         bool   `json:"identity_verified"`
	AuthorizationDecision    string `json:"authorization_decision"`
	AuthorizationVerified    bool   `json:"authorization_verified"`
	AssertionDigest          string `json:"assertion_digest"`
	PolicyDigest             string `json:"policy_digest,omitempty"`
	RevocationSnapshotDigest string `json:"revocation_snapshot_digest"`
	EvaluatedAt              string `json:"evaluated_at"`
	FailureCode              string `json:"failure_code,omitempty"`
}

// AuthorizationVerificationReceipt is transient operational evidence. It
// records the canonical handoff identity alongside the adapter result and does
// not become a custody artifact or authorization grant.
type AuthorizationVerificationReceipt struct {
	HandoffDigest string `json:"handoff_digest"`
	AuthorizationVerificationResult
}

// VerifyAuthorizationHandoff validates local input, delegates external
// verification to the caller-owned verifier, and validates the returned
// result before constructing a transient receipt. It is read-only.
func VerifyAuthorizationHandoff(ctx context.Context, verifier AuthorizationVerifier, request AuthorizationVerificationRequest) (AuthorizationVerificationReceipt, error) {
	if ctx == nil {
		return AuthorizationVerificationReceipt{}, fmt.Errorf("authorization verification context is required")
	}
	if verifier == nil {
		return AuthorizationVerificationReceipt{}, fmt.Errorf("authorization verifier is required")
	}
	if err := request.Handoff.ValidateAt(request.EvaluatedAt); err != nil {
		return AuthorizationVerificationReceipt{}, fmt.Errorf("validate authorization verification request: %w", err)
	}
	if err := artifact.ValidateDigest(request.LocalTargetDigest); err != nil {
		return AuthorizationVerificationReceipt{}, fmt.Errorf("invalid local authorization target digest: %w", err)
	}
	if request.LocalTargetDigest != request.Handoff.Target.Digest {
		return AuthorizationVerificationReceipt{}, fmt.Errorf("authorization request target does not match locally verified target")
	}
	result, err := verifier.VerifyAuthorization(ctx, request)
	if err != nil {
		return AuthorizationVerificationReceipt{}, fmt.Errorf("external authorization verification: %w", err)
	}
	if err := result.ValidateAgainst(request.Handoff, request.LocalTargetDigest, request.EvaluatedAt); err != nil {
		return AuthorizationVerificationReceipt{}, fmt.Errorf("validate external authorization result: %w", err)
	}
	handoffDigest, err := AuthorizationHandoffDigest(request.Handoff)
	if err != nil {
		return AuthorizationVerificationReceipt{}, err
	}
	return AuthorizationVerificationReceipt{
		HandoffDigest:                   handoffDigest,
		AuthorizationVerificationResult: result,
	}, nil
}

// ValidateAgainst checks that an adapter result is bound to the exact local
// target and handoff evaluated at evaluatedAt. It does not make an external
// trust decision; the caller remains responsible for supplying a result from a
// verifier that actually checked the external evidence.
func (result AuthorizationVerificationResult) ValidateAgainst(handoff AuthorizationHandoff, localTargetDigest string, evaluatedAt time.Time) error {
	if err := handoff.ValidateAt(evaluatedAt); err != nil {
		return fmt.Errorf("validate authorization handoff for result: %w", err)
	}
	if err := artifact.ValidateDigest(localTargetDigest); err != nil {
		return fmt.Errorf("invalid local authorization target digest: %w", err)
	}
	if localTargetDigest != handoff.Target.Digest {
		return fmt.Errorf("authorization handoff target does not match locally verified target")
	}
	if result.TargetDigest != localTargetDigest {
		return fmt.Errorf("authorization result target digest mismatch")
	}
	if result.Action != handoff.Action {
		return fmt.Errorf("authorization result action mismatch")
	}
	if result.PrincipalReference != handoff.Principal.Reference {
		return fmt.Errorf("authorization result principal reference mismatch")
	}
	if result.IdentityAssertionDigest != handoff.Principal.AssertionDigest {
		return fmt.Errorf("authorization result identity assertion digest mismatch")
	}
	if result.IdentityVerified && result.IdentityAssertionDigest == "" {
		return fmt.Errorf("authorization result cannot verify an identity without an identity assertion")
	}
	if err := artifact.ValidateDigest(result.AssertionDigest); err != nil {
		return fmt.Errorf("invalid authorization result assertion digest: %w", err)
	}
	if result.AssertionDigest != handoff.Authorization.AssertionDigest {
		return fmt.Errorf("authorization result assertion digest mismatch")
	}
	if result.PolicyDigest != handoff.Authorization.PolicyDigest {
		return fmt.Errorf("authorization result policy digest mismatch")
	}
	if err := artifact.ValidateDigest(result.RevocationSnapshotDigest); err != nil {
		return fmt.Errorf("invalid authorization result revocation digest: %w", err)
	}
	if result.RevocationSnapshotDigest != handoff.Authorization.RevocationSnapshotDigest {
		return fmt.Errorf("authorization result revocation digest mismatch")
	}
	if result.EvaluatedAt != evaluatedAt.UTC().Format(time.RFC3339) {
		return fmt.Errorf("authorization result evaluation time mismatch")
	}
	switch result.AuthorizationDecision {
	case AuthorizationDecisionAuthorized, AuthorizationDecisionDenied, AuthorizationDecisionIndeterminate:
	default:
		return fmt.Errorf("invalid authorization result decision %q", result.AuthorizationDecision)
	}
	if result.AuthorizationVerified {
		if result.AuthorizationDecision != AuthorizationDecisionAuthorized {
			return fmt.Errorf("authorization result cannot verify a non-authorized decision")
		}
		if result.FailureCode != "" {
			return fmt.Errorf("authorized result must not contain a failure code")
		}
		return nil
	}
	if result.FailureCode == "" {
		return fmt.Errorf("non-authorized result requires a failure code")
	}
	return nil
}
