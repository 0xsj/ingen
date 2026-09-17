package attestation

import (
	"bytes"
	"strings"
	"testing"
)

func cleanupEvidenceTargetDigest() string {
	return "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
}

func cleanupPolicyReferenceFixture() CleanupPolicyReference {
	return CleanupPolicyReference{
		Schema:          CleanupPolicyReferenceSchema,
		TargetDigest:    cleanupEvidenceTargetDigest(),
		PolicyID:        "governance.cleanup",
		PolicyVersion:   "v1",
		SnapshotDigest:  "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		RetentionRule:   "orphan-grace-v1",
		SourceTimestamp: "2026-09-17T12:00:00Z",
		ExpiresAt:       "2026-09-17T14:00:00Z",
		EvaluatedAt:     "2026-09-17T12:30:00Z",
		Status:          CleanupEvidenceStatusComplete,
	}
}

func cleanupHoldReferenceFixture() CleanupHoldReference {
	return CleanupHoldReference{
		Schema:         CleanupHoldReferenceSchema,
		TargetDigest:   cleanupEvidenceTargetDigest(),
		SnapshotDigest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		Decision:       CleanupHoldDecisionNotHeld,
		EvaluatedAt:    "2026-09-17T12:30:00Z",
		Status:         CleanupEvidenceStatusComplete,
	}
}

func TestCleanupPolicyAndHoldReferencesRoundTripCanonically(t *testing.T) {
	policy := cleanupPolicyReferenceFixture()
	policyBytes, err := MarshalCanonicalCleanupPolicyReference(policy)
	if err != nil {
		t.Fatal(err)
	}
	decodedPolicy, err := UnmarshalCanonicalCleanupPolicyReference(policyBytes)
	if err != nil {
		t.Fatal(err)
	}
	if decodedPolicy != policy {
		t.Fatalf("decoded policy reference = %+v, want %+v", decodedPolicy, policy)
	}
	if digest, err := CleanupPolicyReferenceDigest(policy); err != nil || digest == "" {
		t.Fatalf("policy reference digest = %q, err = %v", digest, err)
	}

	hold := cleanupHoldReferenceFixture()
	holdBytes, err := MarshalCanonicalCleanupHoldReference(hold)
	if err != nil {
		t.Fatal(err)
	}
	decodedHold, err := UnmarshalCanonicalCleanupHoldReference(holdBytes)
	if err != nil {
		t.Fatal(err)
	}
	if decodedHold != hold {
		t.Fatalf("decoded hold reference = %+v, want %+v", decodedHold, hold)
	}
	if digest, err := CleanupHoldReferenceDigest(hold); err != nil || digest == "" {
		t.Fatalf("hold reference digest = %q, err = %v", digest, err)
	}

	if _, err := UnmarshalCanonicalCleanupPolicyReference(append([]byte(" \n"), policyBytes...)); err == nil || !strings.Contains(err.Error(), "not canonical JSON") {
		t.Fatalf("noncanonical policy reference accepted: %v", err)
	}
	if _, err := UnmarshalCanonicalCleanupHoldReference(append(holdBytes, []byte(" {}")...)); err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("multiple hold reference values accepted: %v", err)
	}
	if !bytes.Equal(policyBytes, mustCleanupPolicyBytes(t, policy)) {
		t.Fatal("policy reference canonical encoding was not stable")
	}
}

func TestCleanupEvidenceReferencesFailClosed(t *testing.T) {
	policy := cleanupPolicyReferenceFixture()
	policy.Status = "unknown"
	if err := policy.Validate(); err == nil || !strings.Contains(err.Error(), "invalid cleanup policy status") {
		t.Fatalf("unknown policy status accepted: %v", err)
	}

	hold := cleanupHoldReferenceFixture()
	hold.Status = CleanupEvidenceStatusIndeterminate
	hold.Decision = CleanupHoldDecisionNotHeld
	if err := hold.Validate(); err == nil || !strings.Contains(err.Error(), "must have indeterminate decision") {
		t.Fatalf("inconsistent hold state accepted: %v", err)
	}

	hold = cleanupHoldReferenceFixture()
	hold.Decision = CleanupHoldDecisionIndeterminate
	if err := hold.Validate(); err == nil || !strings.Contains(err.Error(), "complete cleanup hold reference") {
		t.Fatalf("indeterminate complete hold accepted: %v", err)
	}

	policy = cleanupPolicyReferenceFixture()
	policy.EvaluatedAt = "2026-09-17T11:00:00Z"
	if err := policy.Validate(); err == nil || !strings.Contains(err.Error(), "precedes source timestamp") {
		t.Fatalf("policy evaluated before source accepted: %v", err)
	}
}

func mustCleanupPolicyBytes(t *testing.T, policy CleanupPolicyReference) []byte {
	t.Helper()
	encoded, err := MarshalCanonicalCleanupPolicyReference(policy)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
