package attestation

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"ingen/lockwood/internal/custody"
)

func cleanupPreflightEvidenceFixture(t *testing.T) (CleanupReadinessReport, CleanupRevalidationEvidence, CleanupLeaseEvidence) {
	t.Helper()
	plan, target, request, result := cleanupReadinessFixture(t, custody.CleanupCandidateState)
	readiness := EvaluateCleanupReadiness(plan, target, request, result)
	evaluatedAt, err := request.EvaluationTime()
	if err != nil {
		t.Fatal(err)
	}
	revalidation := CleanupRevalidationEvidence{
		Schema:            CleanupRevalidationSchema,
		TargetDigest:      target.ArtifactDigest,
		CleanupPlanDigest: target.CleanupPlanDigest,
		ObservedDigest:    target.ArtifactDigest,
		ObservedSizeBytes: 10,
		CheckedAt:         evaluatedAt.Format(time.RFC3339),
		ReferenceState:    CleanupRevalidationClear,
		RecoveryState:     CleanupRevalidationClear,
		ProtectionState:   CleanupRevalidationClear,
		State:             CleanupRevalidationClear,
	}
	lease := CleanupLeaseEvidence{
		Schema:            CleanupLeaseSchema,
		TargetDigest:      target.ArtifactDigest,
		CleanupPlanDigest: target.CleanupPlanDigest,
		Coordinator:       "coordinator.example",
		LeaseID:           "lease:cleanup-0001",
		FencingToken:      "fence:0001",
		AcquiredAt:        evaluatedAt.Add(-time.Minute).Format(time.RFC3339),
		ExpiresAt:         evaluatedAt.Add(30 * time.Minute).Format(time.RFC3339),
		State:             CleanupLeaseValid,
	}
	return readiness, revalidation, lease
}

func TestCleanupPreflightEvidenceIsTargetBoundAndCanonical(t *testing.T) {
	readiness, revalidation, lease := cleanupPreflightEvidenceFixture(t)
	if err := readiness.Validate(); err != nil {
		t.Fatal(err)
	}
	revalidationBytes, err := MarshalCanonicalCleanupRevalidation(revalidation)
	if err != nil {
		t.Fatal(err)
	}
	decodedRevalidation, err := UnmarshalCanonicalCleanupRevalidation(revalidationBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decodedRevalidation, revalidation) {
		t.Fatalf("decoded revalidation = %+v, want %+v", decodedRevalidation, revalidation)
	}
	leaseBytes, err := MarshalCanonicalCleanupLease(lease)
	if err != nil {
		t.Fatal(err)
	}
	decodedLease, err := UnmarshalCanonicalCleanupLease(leaseBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decodedLease, lease) {
		t.Fatalf("decoded lease = %+v, want %+v", decodedLease, lease)
	}

	evaluatedAt := time.Date(2026, 9, 17, 12, 30, 0, 0, time.UTC)
	preflight := EvaluateCleanupWorkerPreflight(readiness, revalidation, lease, evaluatedAt)
	if preflight.State != CleanupWorkerPreflightReady || preflight.ActionStatus != custody.CleanupNotAuthorized {
		t.Fatalf("cleanup worker preflight = %+v", preflight)
	}
	if len(preflight.Blockers) != 1 || !strings.Contains(preflight.Blockers[0], "worker is not implemented") {
		t.Fatalf("cleanup worker preflight blockers = %+v", preflight.Blockers)
	}
	if err := preflight.Validate(); err != nil {
		t.Fatal(err)
	}
	preflightBytes, err := MarshalCanonicalCleanupWorkerPreflight(preflight)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UnmarshalCanonicalCleanupWorkerPreflight(preflightBytes); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupWorkerPreflightFailsClosedOnFreshnessAndLeaseChanges(t *testing.T) {
	readiness, revalidation, lease := cleanupPreflightEvidenceFixture(t)
	evaluatedAt := time.Date(2026, 9, 17, 12, 30, 0, 0, time.UTC)

	changed := revalidation
	changed.ObservedDigest = "sha256:9999999999999999999999999999999999999999999999999999999999999999"
	if err := changed.Validate(); err == nil || !strings.Contains(err.Error(), "does not match target") {
		t.Fatalf("changed observed digest accepted: %v", err)
	}
	preflight := EvaluateCleanupWorkerPreflight(readiness, changed, lease, evaluatedAt)
	if preflight.State != CleanupWorkerPreflightBlocked || !strings.Contains(strings.Join(preflight.Blockers, " | "), "revalidation is invalid") {
		t.Fatalf("changed revalidation preflight = %+v", preflight)
	}

	changedLease := lease
	changedLease.State = CleanupLeaseExpired
	changedLease.Blockers = []string{"coordinator lease expired"}
	preflight = EvaluateCleanupWorkerPreflight(readiness, revalidation, changedLease, evaluatedAt)
	if preflight.State != CleanupWorkerPreflightBlocked || !strings.Contains(strings.Join(preflight.Blockers, " | "), "lease is not valid") {
		t.Fatalf("expired lease preflight = %+v", preflight)
	}

	late := evaluatedAt.Add(time.Minute)
	preflight = EvaluateCleanupWorkerPreflight(readiness, revalidation, lease, late)
	if preflight.State != CleanupWorkerPreflightBlocked || !strings.Contains(strings.Join(preflight.Blockers, " | "), "revalidation time") {
		t.Fatalf("stale revalidation preflight = %+v", preflight)
	}
}
