package attestation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fixedAuthorizationVerifier struct {
	result AuthorizationVerificationResult
	err    error
	calls  int
}

func (verifier *fixedAuthorizationVerifier) VerifyAuthorization(_ context.Context, request AuthorizationVerificationRequest) (AuthorizationVerificationResult, error) {
	verifier.calls++
	if request.LocalTargetDigest == "" || request.Handoff.Target.Digest == "" {
		return AuthorizationVerificationResult{}, errors.New("incomplete request")
	}
	return verifier.result, verifier.err
}

func TestAuthorizationVerificationResultBindsToHandoff(t *testing.T) {
	handoff := loadAuthorizationHandoffFixture(t, "valid-authorization-handoff-v1.json")
	evaluatedAt := time.Date(2026, 9, 17, 12, 30, 0, 0, time.UTC)
	result := AuthorizationVerificationResult{
		TargetDigest:             handoff.Target.Digest,
		Action:                   handoff.Action,
		PrincipalReference:       handoff.Principal.Reference,
		IdentityAssertionDigest:  handoff.Principal.AssertionDigest,
		IdentityVerified:         true,
		AuthorizationDecision:    AuthorizationDecisionAuthorized,
		AuthorizationVerified:    true,
		AssertionDigest:          handoff.Authorization.AssertionDigest,
		PolicyDigest:             handoff.Authorization.PolicyDigest,
		RevocationSnapshotDigest: handoff.Authorization.RevocationSnapshotDigest,
		EvaluatedAt:              evaluatedAt.Format(time.RFC3339),
	}
	if err := result.ValidateAgainst(handoff, handoff.Target.Digest, evaluatedAt); err != nil {
		t.Fatalf("valid authorization result rejected: %v", err)
	}

	tests := []struct {
		name string
		edit func(*AuthorizationVerificationResult)
		want string
	}{
		{name: "target mismatch", edit: func(result *AuthorizationVerificationResult) {
			result.TargetDigest = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
		}, want: "target digest mismatch"},
		{name: "principal mismatch", edit: func(result *AuthorizationVerificationResult) { result.PrincipalReference = "amber:principal:other" }, want: "principal reference mismatch"},
		{name: "assertion mismatch", edit: func(result *AuthorizationVerificationResult) {
			result.AssertionDigest = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
		}, want: "assertion digest mismatch"},
		{name: "evaluation mismatch", edit: func(result *AuthorizationVerificationResult) { result.EvaluatedAt = "2026-09-17T12:31:00Z" }, want: "evaluation time mismatch"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := result
			test.edit(&changed)
			if err := changed.ValidateAgainst(handoff, handoff.Target.Digest, evaluatedAt); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestAuthorizationVerificationResultKeepsNonAuthorizedStatesExplicit(t *testing.T) {
	handoff := loadAuthorizationHandoffFixture(t, "valid-authorization-handoff-v1.json")
	evaluatedAt := time.Date(2026, 9, 17, 12, 30, 0, 0, time.UTC)
	base := AuthorizationVerificationResult{
		TargetDigest:             handoff.Target.Digest,
		Action:                   handoff.Action,
		PrincipalReference:       handoff.Principal.Reference,
		IdentityAssertionDigest:  handoff.Principal.AssertionDigest,
		IdentityVerified:         true,
		AuthorizationDecision:    AuthorizationDecisionDenied,
		AssertionDigest:          handoff.Authorization.AssertionDigest,
		PolicyDigest:             handoff.Authorization.PolicyDigest,
		RevocationSnapshotDigest: handoff.Authorization.RevocationSnapshotDigest,
		EvaluatedAt:              evaluatedAt.Format(time.RFC3339),
		FailureCode:              AuthorizationFailureDenied,
	}
	if err := base.ValidateAgainst(handoff, handoff.Target.Digest, evaluatedAt); err != nil {
		t.Fatalf("explicit denied result rejected: %v", err)
	}

	missingFailure := base
	missingFailure.FailureCode = ""
	if err := missingFailure.ValidateAgainst(handoff, handoff.Target.Digest, evaluatedAt); err == nil || !strings.Contains(err.Error(), "requires a failure code") {
		t.Fatalf("non-authorized result without failure code accepted: %v", err)
	}

	verifiedDenied := base
	verifiedDenied.AuthorizationVerified = true
	if err := verifiedDenied.ValidateAgainst(handoff, handoff.Target.Digest, evaluatedAt); err == nil || !strings.Contains(err.Error(), "non-authorized decision") {
		t.Fatalf("verified denied result accepted: %v", err)
	}

	identityOnly := loadAuthorizationHandoffFixture(t, "valid-authorization-handoff-reference-only-v1.json")
	identityOnlyResult := AuthorizationVerificationResult{
		TargetDigest:             identityOnly.Target.Digest,
		Action:                   identityOnly.Action,
		PrincipalReference:       identityOnly.Principal.Reference,
		AuthorizationDecision:    AuthorizationDecisionAuthorized,
		AuthorizationVerified:    true,
		AssertionDigest:          identityOnly.Authorization.AssertionDigest,
		RevocationSnapshotDigest: identityOnly.Authorization.RevocationSnapshotDigest,
		EvaluatedAt:              evaluatedAt.Format(time.RFC3339),
	}
	if err := identityOnlyResult.ValidateAgainst(identityOnly, identityOnly.Target.Digest, evaluatedAt); err != nil {
		t.Fatalf("authorization independent of descriptive identity reference rejected: %v", err)
	}
	identityOnlyResult.IdentityVerified = true
	if err := identityOnlyResult.ValidateAgainst(identityOnly, identityOnly.Target.Digest, evaluatedAt); err == nil || !strings.Contains(err.Error(), "without an identity assertion") {
		t.Fatalf("identity verified from descriptive reference alone: %v", err)
	}
}

func TestVerifyAuthorizationHandoffBuildsTransientReceipt(t *testing.T) {
	handoff := loadAuthorizationHandoffFixture(t, "valid-authorization-handoff-v1.json")
	evaluatedAt := time.Date(2026, 9, 17, 12, 30, 0, 0, time.UTC)
	result := AuthorizationVerificationResult{
		TargetDigest:             handoff.Target.Digest,
		Action:                   handoff.Action,
		PrincipalReference:       handoff.Principal.Reference,
		IdentityAssertionDigest:  handoff.Principal.AssertionDigest,
		IdentityVerified:         true,
		AuthorizationDecision:    AuthorizationDecisionAuthorized,
		AuthorizationVerified:    true,
		AssertionDigest:          handoff.Authorization.AssertionDigest,
		PolicyDigest:             handoff.Authorization.PolicyDigest,
		RevocationSnapshotDigest: handoff.Authorization.RevocationSnapshotDigest,
		EvaluatedAt:              evaluatedAt.Format(time.RFC3339),
	}
	verifier := &fixedAuthorizationVerifier{result: result}
	receipt, err := VerifyAuthorizationHandoff(context.Background(), verifier, AuthorizationVerificationRequest{
		Handoff:           handoff,
		LocalTargetDigest: handoff.Target.Digest,
		EvaluatedAt:       evaluatedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if verifier.calls != 1 || receipt.HandoffDigest == "" || !receipt.AuthorizationVerified || receipt.TargetDigest != handoff.Target.Digest {
		t.Fatalf("authorization receipt = %+v, verifier calls = %d", receipt, verifier.calls)
	}
}

func TestVerifyAuthorizationHandoffRejectsInvalidRequestAndAdapterErrors(t *testing.T) {
	handoff := loadAuthorizationHandoffFixture(t, "valid-authorization-handoff-v1.json")
	evaluatedAt := time.Date(2026, 9, 17, 12, 30, 0, 0, time.UTC)
	verifier := &fixedAuthorizationVerifier{}
	request := AuthorizationVerificationRequest{
		Handoff:           handoff,
		LocalTargetDigest: handoff.Target.Digest,
		EvaluatedAt:       evaluatedAt,
	}
	if _, err := VerifyAuthorizationHandoff(nil, verifier, request); err == nil || !strings.Contains(err.Error(), "context is required") {
		t.Fatalf("nil context accepted: %v", err)
	}
	if _, err := VerifyAuthorizationHandoff(context.Background(), nil, request); err == nil || !strings.Contains(err.Error(), "verifier is required") {
		t.Fatalf("nil verifier accepted: %v", err)
	}
	request.LocalTargetDigest = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	if _, err := VerifyAuthorizationHandoff(context.Background(), verifier, request); err == nil || !strings.Contains(err.Error(), "does not match locally verified target") {
		t.Fatalf("mismatched local target accepted: %v", err)
	}
	if verifier.calls != 0 {
		t.Fatalf("verifier called for invalid request: %d", verifier.calls)
	}
	verifier.err = errors.New("external service unavailable")
	request.LocalTargetDigest = handoff.Target.Digest
	if _, err := VerifyAuthorizationHandoff(context.Background(), verifier, request); err == nil || !strings.Contains(err.Error(), "external service unavailable") {
		t.Fatalf("adapter error not propagated: %v", err)
	}
}
