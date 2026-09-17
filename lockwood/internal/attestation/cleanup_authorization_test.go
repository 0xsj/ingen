package attestation

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fixedCleanupAuthorizationVerifier struct {
	result CleanupAuthorizationResult
	err    error
	calls  int
}

func (verifier *fixedCleanupAuthorizationVerifier) VerifyCleanupAuthorization(_ context.Context, _ CleanupAuthorizationRequest) (CleanupAuthorizationResult, error) {
	verifier.calls++
	return verifier.result, verifier.err
}

func cleanupAuthorizationRequestFixture(t *testing.T) CleanupAuthorizationRequest {
	t.Helper()
	target, err := NewCleanupArtifactAuthorizationTarget(
		"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"2026-09-17T12:00:00Z",
	)
	if err != nil {
		t.Fatal(err)
	}
	policy := CleanupPolicyReference{
		Schema:          CleanupPolicyReferenceSchema,
		TargetDigest:    target.ArtifactDigest,
		PolicyID:        "governance.cleanup",
		PolicyVersion:   "v1",
		SnapshotDigest:  "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		RetentionRule:   "orphan-grace-v1",
		SourceTimestamp: "2026-09-17T12:00:00Z",
		ExpiresAt:       "2026-09-17T14:00:00Z",
		EvaluatedAt:     "2026-09-17T12:30:00Z",
		Status:          CleanupEvidenceStatusComplete,
	}
	hold := CleanupHoldReference{
		Schema:         CleanupHoldReferenceSchema,
		TargetDigest:   target.ArtifactDigest,
		SnapshotDigest: "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		Decision:       CleanupHoldDecisionNotHeld,
		EvaluatedAt:    "2026-09-17T12:30:00Z",
		Status:         CleanupEvidenceStatusComplete,
	}
	request, err := NewCleanupAuthorizationRequest(
		target,
		policy,
		hold,
		AuthorizationPrincipalReference{Authority: "amber", Reference: "amber:principal:cleanup-0001"},
		AuthorizationEvidence{
			Authority:                "governance.example",
			Decision:                 AuthorizationDecisionAuthorized,
			AssertionDigest:          "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
			PolicyDigest:             "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
			IssuedAt:                 "2026-09-17T12:00:00Z",
			NotBefore:                "2026-09-17T12:00:00Z",
			ExpiresAt:                "2026-09-17T13:00:00Z",
			RevocationSnapshotDigest: "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		},
		time.Date(2026, 9, 17, 12, 30, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func cleanupAuthorizationResultFixture(t *testing.T, request CleanupAuthorizationRequest) CleanupAuthorizationResult {
	t.Helper()
	handoffDigest, err := AuthorizationHandoffDigest(request.Handoff)
	if err != nil {
		t.Fatal(err)
	}
	return CleanupAuthorizationResult{
		Schema:             CleanupAuthorizationResultSchema,
		HandoffDigest:      handoffDigest,
		HoldSnapshotDigest: request.Hold.SnapshotDigest,
		HoldDecision:       CleanupHoldDecisionNotHeld,
		Authorization: AuthorizationVerificationResult{
			TargetDigest:             request.Handoff.Target.Digest,
			Action:                   AuthorizationActionCleanupDelete,
			PrincipalReference:       request.Handoff.Principal.Reference,
			AuthorizationDecision:    AuthorizationDecisionAuthorized,
			AuthorizationVerified:    true,
			AssertionDigest:          request.Handoff.Authorization.AssertionDigest,
			PolicyDigest:             request.Handoff.Authorization.PolicyDigest,
			RevocationSnapshotDigest: request.Handoff.Authorization.RevocationSnapshotDigest,
			EvaluatedAt:              request.EvaluatedAt,
		},
	}
}

func TestCleanupAuthorizationRequestAndResultAreExplicitlyBound(t *testing.T) {
	request := cleanupAuthorizationRequestFixture(t)
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	result := cleanupAuthorizationResultFixture(t, request)
	if err := result.ValidateAgainst(request); err != nil {
		t.Fatal(err)
	}

	changedRequest := request
	changedRequest.Handoff.Action = AuthorizationActionCustodyRecord
	if err := changedRequest.Validate(); err == nil || !strings.Contains(err.Error(), "action") {
		t.Fatalf("cleanup action mismatch accepted: %v", err)
	}

	held := result
	held.HoldDecision = CleanupHoldDecisionHeld
	if err := held.ValidateAgainst(request); err == nil || !strings.Contains(err.Error(), "hold decision") {
		t.Fatalf("authorized result with active hold accepted: %v", err)
	}

	noPolicy := request
	noPolicy.Handoff.Authorization.PolicyDigest = ""
	if err := noPolicy.Validate(); err == nil || !strings.Contains(err.Error(), "policy digest is required") {
		t.Fatalf("cleanup request without policy digest accepted: %v", err)
	}

	wrongPolicyTarget := request
	wrongPolicyTarget.Policy.TargetDigest = "sha256:9999999999999999999999999999999999999999999999999999999999999999"
	if err := wrongPolicyTarget.Validate(); err == nil || !strings.Contains(err.Error(), "policy reference target digest") {
		t.Fatalf("policy target mismatch accepted: %v", err)
	}

	wrongHoldTime := request
	wrongHoldTime.Hold.EvaluatedAt = "2026-09-17T12:31:00Z"
	if err := wrongHoldTime.Validate(); err == nil || !strings.Contains(err.Error(), "hold evaluation time") {
		t.Fatalf("hold evaluation mismatch accepted: %v", err)
	}

	incompleteEvidence := request
	incompleteEvidence.Policy.Status = CleanupEvidenceStatusIndeterminate
	if err := result.ValidateAgainst(incompleteEvidence); err == nil || !strings.Contains(err.Error(), "incomplete policy or hold evidence") {
		t.Fatalf("authorized result with incomplete evidence accepted: %v", err)
	}
}

func TestCleanupAuthorizationRequestAndResultCanonicalTransport(t *testing.T) {
	request := cleanupAuthorizationRequestFixture(t)
	requestBytes, err := MarshalCanonicalCleanupAuthorizationRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	decodedRequest, err := UnmarshalCanonicalCleanupAuthorizationRequest(requestBytes)
	if err != nil {
		t.Fatal(err)
	}
	if decodedRequest.Schema != request.Schema || decodedRequest.Target != request.Target || decodedRequest.Handoff != request.Handoff || decodedRequest.EvaluatedAt != request.EvaluatedAt {
		t.Fatalf("decoded cleanup request = %+v, want %+v", decodedRequest, request)
	}
	if digest, err := CleanupAuthorizationRequestDigest(request); err != nil || digest == "" {
		t.Fatalf("cleanup request digest = %q, err = %v", digest, err)
	}

	result := cleanupAuthorizationResultFixture(t, request)
	resultBytes, err := MarshalCanonicalCleanupAuthorizationResult(result)
	if err != nil {
		t.Fatal(err)
	}
	decodedResult, err := UnmarshalCanonicalCleanupAuthorizationResult(resultBytes)
	if err != nil {
		t.Fatal(err)
	}
	if decodedResult.Schema != result.Schema || decodedResult.HandoffDigest != result.HandoffDigest || decodedResult.HoldDecision != result.HoldDecision || decodedResult.Authorization != result.Authorization {
		t.Fatalf("decoded cleanup result = %+v, want %+v", decodedResult, result)
	}
	if digest, err := CleanupAuthorizationResultDigest(result); err != nil || digest == "" {
		t.Fatalf("cleanup result digest = %q, err = %v", digest, err)
	}

	for _, test := range []struct {
		name string
		data []byte
		want string
	}{
		{name: "request whitespace", data: append([]byte(" \n"), requestBytes...), want: "request is not canonical JSON"},
		{name: "result whitespace", data: append([]byte(" \n"), resultBytes...), want: "result is not canonical JSON"},
		{name: "request unknown field", data: append(requestBytes[:len(requestBytes)-1], []byte(","+`"extra":true}`)...), want: "decode cleanup authorization request"},
		{name: "result unknown field", data: append(resultBytes[:len(resultBytes)-1], []byte(","+`"extra":true}`)...), want: "decode cleanup authorization result"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var err error
			if bytes.HasPrefix(test.data, []byte(" \n")) && strings.Contains(test.name, "request") {
				_, err = UnmarshalCanonicalCleanupAuthorizationRequest(test.data)
			} else if bytes.HasPrefix(test.data, []byte(" \n")) && strings.Contains(test.name, "result") {
				_, err = UnmarshalCanonicalCleanupAuthorizationResult(test.data)
			} else if strings.Contains(test.name, "request") {
				_, err = UnmarshalCanonicalCleanupAuthorizationRequest(test.data)
			} else {
				_, err = UnmarshalCanonicalCleanupAuthorizationResult(test.data)
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestVerifyCleanupAuthorizationIsReadOnlyAndFailClosed(t *testing.T) {
	request := cleanupAuthorizationRequestFixture(t)
	result := cleanupAuthorizationResultFixture(t, request)
	verifier := &fixedCleanupAuthorizationVerifier{result: result}
	verified, err := VerifyCleanupAuthorization(context.Background(), verifier, request)
	if err != nil {
		t.Fatal(err)
	}
	if verifier.calls != 1 || verified.HoldDecision != CleanupHoldDecisionNotHeld {
		t.Fatalf("verified cleanup result = %+v, verifier calls = %d", verified, verifier.calls)
	}

	verifier.err = errors.New("hold service unavailable")
	if _, err := VerifyCleanupAuthorization(context.Background(), verifier, request); err == nil || !strings.Contains(err.Error(), "hold service unavailable") {
		t.Fatalf("external cleanup error not propagated: %v", err)
	}
	if verifier.calls != 2 {
		t.Fatalf("verifier calls = %d, want 2", verifier.calls)
	}
}

func TestCleanupAuthorizationRequestRejectsEvaluationBeforePlan(t *testing.T) {
	target, err := NewCleanupArtifactAuthorizationTarget(
		"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"2026-09-17T12:00:00Z",
	)
	if err != nil {
		t.Fatal(err)
	}
	request, err := NewCleanupAuthorizationRequest(
		target,
		CleanupPolicyReference{
			Schema:          CleanupPolicyReferenceSchema,
			TargetDigest:    target.ArtifactDigest,
			PolicyID:        "governance.cleanup",
			PolicyVersion:   "v1",
			SnapshotDigest:  "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
			RetentionRule:   "orphan-grace-v1",
			SourceTimestamp: "2026-09-17T11:00:00Z",
			ExpiresAt:       "2026-09-17T14:00:00Z",
			EvaluatedAt:     "2026-09-17T11:30:00Z",
			Status:          CleanupEvidenceStatusComplete,
		},
		CleanupHoldReference{
			Schema:         CleanupHoldReferenceSchema,
			TargetDigest:   target.ArtifactDigest,
			SnapshotDigest: "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			Decision:       CleanupHoldDecisionNotHeld,
			EvaluatedAt:    "2026-09-17T11:30:00Z",
			Status:         CleanupEvidenceStatusComplete,
		},
		AuthorizationPrincipalReference{Authority: "amber", Reference: "amber:principal:cleanup-0001"},
		AuthorizationEvidence{
			Authority:                "governance.example",
			Decision:                 AuthorizationDecisionAuthorized,
			AssertionDigest:          "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
			PolicyDigest:             "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
			IssuedAt:                 "2026-09-17T11:00:00Z",
			NotBefore:                "2026-09-17T11:00:00Z",
			ExpiresAt:                "2026-09-17T13:00:00Z",
			RevocationSnapshotDigest: "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		},
		time.Date(2026, 9, 17, 11, 30, 0, 0, time.UTC),
	)
	if err == nil || !strings.Contains(err.Error(), "precedes cleanup plan") {
		t.Fatalf("evaluation before plan accepted: request=%+v, err=%v", request, err)
	}
}
