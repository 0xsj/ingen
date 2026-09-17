package attestation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"ingen/lockwood/internal/artifact"
)

const (
	CleanupAuthorizationRequestSchema = "lockwood.cleanup-authorization-request/v1"
	CleanupAuthorizationResultSchema  = "lockwood.cleanup-authorization-result/v1"

	CleanupHoldDecisionNotHeld       = "not-held"
	CleanupHoldDecisionHeld          = "held"
	CleanupHoldDecisionIndeterminate = "indeterminate"
)

// CleanupAuthorizationRequest is the typed, read-only handoff between a
// cleanup plan and a caller-owned authorization authority.
type CleanupAuthorizationRequest struct {
	Schema      string                     `json:"schema"`
	Target      CleanupAuthorizationTarget `json:"target"`
	Policy      CleanupPolicyReference     `json:"policy"`
	Hold        CleanupHoldReference       `json:"hold"`
	Handoff     AuthorizationHandoff       `json:"handoff"`
	EvaluatedAt string                     `json:"evaluated_at"`
}

// NewCleanupAuthorizationRequest binds a cleanup target to the existing
// external authorization envelope at one explicit evaluation time.
func NewCleanupAuthorizationRequest(target CleanupAuthorizationTarget, policy CleanupPolicyReference, hold CleanupHoldReference, principal AuthorizationPrincipalReference, authorization AuthorizationEvidence, evaluatedAt time.Time) (CleanupAuthorizationRequest, error) {
	if evaluatedAt.IsZero() {
		return CleanupAuthorizationRequest{}, fmt.Errorf("cleanup authorization evaluation time is required")
	}
	authorizationTarget, err := target.AuthorizationTarget()
	if err != nil {
		return CleanupAuthorizationRequest{}, err
	}
	handoff, err := NewAuthorizationHandoff(authorizationTarget, principal, authorization)
	if err != nil {
		return CleanupAuthorizationRequest{}, err
	}
	request := CleanupAuthorizationRequest{
		Schema:      CleanupAuthorizationRequestSchema,
		Target:      target,
		Policy:      policy,
		Hold:        hold,
		Handoff:     handoff,
		EvaluatedAt: evaluatedAt.UTC().Format(time.RFC3339),
	}
	if err := request.Validate(); err != nil {
		return CleanupAuthorizationRequest{}, err
	}
	return request, nil
}

func (request CleanupAuthorizationRequest) EvaluationTime() (time.Time, error) {
	return parseRequiredAuthorizationTime("evaluated_at", request.EvaluatedAt)
}

func (request CleanupAuthorizationRequest) Validate() error {
	if request.Schema != CleanupAuthorizationRequestSchema {
		return fmt.Errorf("unexpected cleanup authorization request schema %q", request.Schema)
	}
	if err := request.Target.Validate(); err != nil {
		return err
	}
	if err := request.Policy.Validate(); err != nil {
		return err
	}
	if err := request.Hold.Validate(); err != nil {
		return err
	}
	evaluatedAt, err := request.EvaluationTime()
	if err != nil {
		return err
	}
	planAsOf, err := parseRequiredAuthorizationTime("plan_as_of", request.Target.PlanAsOf)
	if err != nil {
		return err
	}
	if evaluatedAt.Before(planAsOf) {
		return fmt.Errorf("cleanup authorization evaluation time precedes cleanup plan as_of")
	}
	if request.Policy.TargetDigest != request.Target.ArtifactDigest {
		return fmt.Errorf("cleanup policy reference target digest does not match cleanup target")
	}
	if request.Hold.TargetDigest != request.Target.ArtifactDigest {
		return fmt.Errorf("cleanup hold reference target digest does not match cleanup target")
	}
	if request.Policy.EvaluatedAt != request.EvaluatedAt {
		return fmt.Errorf("cleanup policy evaluation time does not match request")
	}
	if request.Hold.EvaluatedAt != request.EvaluatedAt {
		return fmt.Errorf("cleanup hold evaluation time does not match request")
	}
	localTarget, err := request.Target.AuthorizationTarget()
	if err != nil {
		return err
	}
	if request.Handoff.Target != localTarget {
		return fmt.Errorf("cleanup authorization handoff target does not match cleanup target")
	}
	if request.Handoff.Action != AuthorizationActionCleanupDelete {
		return fmt.Errorf("cleanup authorization handoff action must be %q", AuthorizationActionCleanupDelete)
	}
	if request.Handoff.Authorization.PolicyDigest == "" {
		return fmt.Errorf("cleanup authorization policy digest is required")
	}
	if request.Handoff.Authorization.PolicyDigest != request.Policy.SnapshotDigest {
		return fmt.Errorf("cleanup authorization policy digest does not match policy reference")
	}
	if err := request.Handoff.ValidateAt(evaluatedAt); err != nil {
		return fmt.Errorf("validate cleanup authorization handoff: %w", err)
	}
	return nil
}

// CleanupAuthorizationResult wraps the generic external authorization result
// with the cleanup-specific legal-hold decision.
type CleanupAuthorizationResult struct {
	Schema             string                          `json:"schema"`
	HandoffDigest      string                          `json:"handoff_digest"`
	HoldSnapshotDigest string                          `json:"hold_snapshot_digest"`
	HoldDecision       string                          `json:"hold_decision"`
	Authorization      AuthorizationVerificationResult `json:"authorization"`
}

func (result CleanupAuthorizationResult) ValidateAgainst(request CleanupAuthorizationRequest) error {
	if err := request.Validate(); err != nil {
		return fmt.Errorf("validate cleanup authorization request: %w", err)
	}
	if result.Schema != CleanupAuthorizationResultSchema {
		return fmt.Errorf("unexpected cleanup authorization result schema %q", result.Schema)
	}
	handoffDigest, err := AuthorizationHandoffDigest(request.Handoff)
	if err != nil {
		return err
	}
	if result.HandoffDigest != handoffDigest {
		return fmt.Errorf("cleanup authorization result handoff digest mismatch")
	}
	if result.HoldSnapshotDigest != request.Hold.SnapshotDigest {
		return fmt.Errorf("cleanup authorization result hold snapshot digest mismatch")
	}
	evaluatedAt, err := request.EvaluationTime()
	if err != nil {
		return err
	}
	if err := result.Authorization.ValidateAgainst(request.Handoff, request.Handoff.Target.Digest, evaluatedAt); err != nil {
		return err
	}
	switch result.HoldDecision {
	case CleanupHoldDecisionNotHeld, CleanupHoldDecisionHeld, CleanupHoldDecisionIndeterminate:
	default:
		return fmt.Errorf("invalid cleanup hold decision %q", result.HoldDecision)
	}
	if result.HoldDecision != request.Hold.Decision {
		return fmt.Errorf("cleanup authorization result hold decision mismatch")
	}
	if result.Authorization.AuthorizationVerified && result.HoldDecision != CleanupHoldDecisionNotHeld {
		return fmt.Errorf("cleanup authorization cannot verify an authorized result while hold decision is %q", result.HoldDecision)
	}
	if result.Authorization.AuthorizationVerified && (request.Policy.Status != CleanupEvidenceStatusComplete || request.Hold.Status != CleanupEvidenceStatusComplete) {
		return fmt.Errorf("cleanup authorization cannot verify an authorized result with incomplete policy or hold evidence")
	}
	return nil
}

// MarshalCanonicalCleanupAuthorizationRequest returns the compact canonical
// JSON transport for a typed cleanup request.
func MarshalCanonicalCleanupAuthorizationRequest(request CleanupAuthorizationRequest) ([]byte, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(request)
}

// UnmarshalCanonicalCleanupAuthorizationRequest strictly decodes one request
// and rejects unknown fields, multiple values, or noncanonical JSON.
func UnmarshalCanonicalCleanupAuthorizationRequest(data []byte) (CleanupAuthorizationRequest, error) {
	var request CleanupAuthorizationRequest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return CleanupAuthorizationRequest{}, fmt.Errorf("decode cleanup authorization request: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return CleanupAuthorizationRequest{}, fmt.Errorf("cleanup authorization request contains multiple JSON values")
		}
		return CleanupAuthorizationRequest{}, fmt.Errorf("decode cleanup authorization request: %w", err)
	}
	canonical, err := MarshalCanonicalCleanupAuthorizationRequest(request)
	if err != nil {
		return CleanupAuthorizationRequest{}, err
	}
	if !bytes.Equal(data, canonical) {
		return CleanupAuthorizationRequest{}, fmt.Errorf("cleanup authorization request is not canonical JSON")
	}
	return request, nil
}

// CleanupAuthorizationRequestDigest identifies the canonical request bytes.
func CleanupAuthorizationRequestDigest(request CleanupAuthorizationRequest) (string, error) {
	encoded, err := MarshalCanonicalCleanupAuthorizationRequest(request)
	if err != nil {
		return "", err
	}
	return artifact.DigestBytes(encoded), nil
}

// MarshalCanonicalCleanupAuthorizationResult returns the compact canonical
// result transport. Full target binding is checked with ValidateAgainst when
// the corresponding request is available.
func MarshalCanonicalCleanupAuthorizationResult(result CleanupAuthorizationResult) ([]byte, error) {
	if err := validateCleanupAuthorizationResultShape(result); err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

// UnmarshalCanonicalCleanupAuthorizationResult strictly decodes one result
// and validates its standalone transport shape. Callers must still invoke
// ValidateAgainst to bind it to a request.
func UnmarshalCanonicalCleanupAuthorizationResult(data []byte) (CleanupAuthorizationResult, error) {
	var result CleanupAuthorizationResult
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return CleanupAuthorizationResult{}, fmt.Errorf("decode cleanup authorization result: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return CleanupAuthorizationResult{}, fmt.Errorf("cleanup authorization result contains multiple JSON values")
		}
		return CleanupAuthorizationResult{}, fmt.Errorf("decode cleanup authorization result: %w", err)
	}
	canonical, err := MarshalCanonicalCleanupAuthorizationResult(result)
	if err != nil {
		return CleanupAuthorizationResult{}, err
	}
	if !bytes.Equal(data, canonical) {
		return CleanupAuthorizationResult{}, fmt.Errorf("cleanup authorization result is not canonical JSON")
	}
	return result, nil
}

// CleanupAuthorizationResultDigest identifies the canonical result bytes. It
// is not an authorization decision and does not replace ValidateAgainst.
func CleanupAuthorizationResultDigest(result CleanupAuthorizationResult) (string, error) {
	encoded, err := MarshalCanonicalCleanupAuthorizationResult(result)
	if err != nil {
		return "", err
	}
	return artifact.DigestBytes(encoded), nil
}

func validateCleanupAuthorizationResultShape(result CleanupAuthorizationResult) error {
	if result.Schema != CleanupAuthorizationResultSchema {
		return fmt.Errorf("unexpected cleanup authorization result schema %q", result.Schema)
	}
	if err := artifact.ValidateDigest(result.HandoffDigest); err != nil {
		return fmt.Errorf("invalid cleanup authorization handoff digest: %w", err)
	}
	if err := artifact.ValidateDigest(result.HoldSnapshotDigest); err != nil {
		return fmt.Errorf("invalid cleanup authorization hold snapshot digest: %w", err)
	}
	switch result.HoldDecision {
	case CleanupHoldDecisionNotHeld, CleanupHoldDecisionHeld, CleanupHoldDecisionIndeterminate:
	default:
		return fmt.Errorf("invalid cleanup hold decision %q", result.HoldDecision)
	}
	if err := artifact.ValidateDigest(result.Authorization.TargetDigest); err != nil {
		return fmt.Errorf("invalid cleanup authorization result target digest: %w", err)
	}
	if result.Authorization.Action != AuthorizationActionCleanupDelete {
		return fmt.Errorf("cleanup authorization result action must be %q", AuthorizationActionCleanupDelete)
	}
	if !validOpaqueAuthorizationReference(result.Authorization.PrincipalReference) {
		return fmt.Errorf("invalid cleanup authorization result principal reference")
	}
	if err := artifact.ValidateDigest(result.Authorization.AssertionDigest); err != nil {
		return fmt.Errorf("invalid cleanup authorization result assertion digest: %w", err)
	}
	if result.Authorization.IdentityAssertionDigest != "" {
		if err := artifact.ValidateDigest(result.Authorization.IdentityAssertionDigest); err != nil {
			return fmt.Errorf("invalid cleanup authorization result identity assertion digest: %w", err)
		}
	}
	if err := artifact.ValidateDigest(result.Authorization.PolicyDigest); err != nil {
		return fmt.Errorf("invalid cleanup authorization result policy digest: %w", err)
	}
	if err := artifact.ValidateDigest(result.Authorization.RevocationSnapshotDigest); err != nil {
		return fmt.Errorf("invalid cleanup authorization result revocation digest: %w", err)
	}
	if _, err := parseRequiredAuthorizationTime("evaluated_at", result.Authorization.EvaluatedAt); err != nil {
		return err
	}
	switch result.Authorization.AuthorizationDecision {
	case AuthorizationDecisionAuthorized, AuthorizationDecisionDenied, AuthorizationDecisionIndeterminate:
	default:
		return fmt.Errorf("invalid cleanup authorization result decision %q", result.Authorization.AuthorizationDecision)
	}
	if !result.Authorization.AuthorizationVerified && result.Authorization.FailureCode == "" {
		return fmt.Errorf("non-authorized cleanup result requires a failure code")
	}
	if result.Authorization.AuthorizationVerified && result.Authorization.AuthorizationDecision != AuthorizationDecisionAuthorized {
		return fmt.Errorf("cleanup result cannot verify a non-authorized decision")
	}
	return nil
}

// CleanupAuthorizationVerifier is caller-owned. Its implementation may use
// external identity, policy, legal-hold, and revocation systems, but it must
// not mutate Lockwood state.
type CleanupAuthorizationVerifier interface {
	VerifyCleanupAuthorization(context.Context, CleanupAuthorizationRequest) (CleanupAuthorizationResult, error)
}

// VerifyCleanupAuthorization validates a typed request, delegates the
// decision to the caller-owned verifier, and validates the typed result. It
// never changes a cleanup plan, blob, or custody record.
func VerifyCleanupAuthorization(ctx context.Context, verifier CleanupAuthorizationVerifier, request CleanupAuthorizationRequest) (CleanupAuthorizationResult, error) {
	if ctx == nil {
		return CleanupAuthorizationResult{}, fmt.Errorf("cleanup authorization context is required")
	}
	if verifier == nil {
		return CleanupAuthorizationResult{}, fmt.Errorf("cleanup authorization verifier is required")
	}
	if err := request.Validate(); err != nil {
		return CleanupAuthorizationResult{}, fmt.Errorf("validate cleanup authorization request: %w", err)
	}
	result, err := verifier.VerifyCleanupAuthorization(ctx, request)
	if err != nil {
		return CleanupAuthorizationResult{}, fmt.Errorf("external cleanup authorization verification: %w", err)
	}
	if err := result.ValidateAgainst(request); err != nil {
		return CleanupAuthorizationResult{}, fmt.Errorf("validate cleanup authorization result: %w", err)
	}
	return result, nil
}
