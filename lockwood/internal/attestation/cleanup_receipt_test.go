package attestation

import (
	"strings"
	"testing"
)

func cleanupOutcomeReceiptFixture(t *testing.T) (CleanupAuthorizationRequest, CleanupAuthorizationResult, CleanupOutcomeReceipt) {
	t.Helper()
	_, target, request, result := cleanupReadinessFixture(t, "cleanup-candidate")
	handoffDigest, err := AuthorizationHandoffDigest(request.Handoff)
	if err != nil {
		t.Fatal(err)
	}
	receipt := CleanupOutcomeReceipt{
		Schema:                       CleanupOutcomeReceiptSchema,
		ReceiptID:                    "receipt:cleanup-0001",
		TargetDigest:                 target.ArtifactDigest,
		Action:                       AuthorizationActionCleanupDelete,
		CleanupPlanDigest:            target.CleanupPlanDigest,
		PolicySnapshotDigest:         request.Policy.SnapshotDigest,
		HoldSnapshotDigest:           request.Hold.SnapshotDigest,
		HandoffDigest:                handoffDigest,
		EvaluationTime:               request.EvaluatedAt,
		AuthorizationAuthority:       request.Handoff.Authorization.Authority,
		AuthorizationAssertionDigest: request.Handoff.Authorization.AssertionDigest,
		PrincipalReference:           request.Handoff.Principal.Reference,
		AuthorizationDecision:        result.Authorization.AuthorizationDecision,
		HoldDecision:                 result.HoldDecision,
		Outcome:                      CleanupOutcomeEligibilityLost,
		Postcondition:                CleanupPostconditionNotChecked,
		FailureCode:                  "lease-lost-before-attempt",
	}
	return request, result, receipt
}

func TestCleanupOutcomeReceiptBindsLifecycleEvidence(t *testing.T) {
	request, result, receipt := cleanupOutcomeReceiptFixture(t)
	if err := receipt.ValidateAgainst(request, result); err != nil {
		t.Fatal(err)
	}
	encoded, err := MarshalCanonicalCleanupOutcomeReceipt(receipt)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := UnmarshalCanonicalCleanupOutcomeReceipt(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != receipt {
		t.Fatalf("decoded cleanup receipt = %+v, want %+v", decoded, receipt)
	}

	changed := receipt
	changed.TargetDigest = "sha256:9999999999999999999999999999999999999999999999999999999999999999"
	if err := changed.ValidateAgainst(request, result); err == nil || !strings.Contains(err.Error(), "target digest mismatch") {
		t.Fatalf("receipt target mismatch accepted: %v", err)
	}

	changed = receipt
	changed.Outcome = CleanupOutcomeSucceeded
	if err := changed.Validate(); err == nil || !strings.Contains(err.Error(), "succeeded cleanup outcome") {
		t.Fatalf("unattempted success accepted: %v", err)
	}
}

func TestCleanupOutcomeReceiptFailsClosedOnAttemptAndPostconditionRules(t *testing.T) {
	_, _, receipt := cleanupOutcomeReceiptFixture(t)
	receipt.Outcome = CleanupOutcomeAttempted
	receipt.AttemptTime = receipt.EvaluationTime
	if err := receipt.Validate(); err == nil || !strings.Contains(err.Error(), "requires lease and fencing") {
		t.Fatalf("attempt without lease accepted: %v", err)
	}

	receipt.LeaseID = "lease:cleanup-0001"
	receipt.FencingToken = "fence:0001"
	receipt.Coordinator = "coordinator.example"
	if err := receipt.Validate(); err != nil {
		t.Fatal(err)
	}

	receipt.Outcome = CleanupOutcomeFailed
	receipt.Postcondition = CleanupPostconditionNotChecked
	if err := receipt.Validate(); err == nil || !strings.Contains(err.Error(), "failed postcondition") {
		t.Fatalf("attempted failure without failed postcondition accepted: %v", err)
	}

	receipt.Postcondition = CleanupPostconditionFailed
	if err := receipt.Validate(); err != nil {
		t.Fatal(err)
	}
}
