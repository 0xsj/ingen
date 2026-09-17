package attestation

import (
	"strings"
	"testing"
	"time"

	"ingen/lockwood/internal/custody"
)

func cleanupReadinessFixture(t *testing.T, state custody.CleanupPlanState) (custody.CleanupPlan, CleanupAuthorizationTarget, CleanupAuthorizationRequest, CleanupAuthorizationResult) {
	t.Helper()
	asOf := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	digest := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	plan, err := custody.BuildCleanupPlan(custody.Reconciliation{
		Orphans:           []custody.OrphanArtifact{{Digest: digest, SizeBytes: 10, ModifiedAt: asOf.Add(-48 * time.Hour)}},
		CleanupCandidates: []custody.OrphanArtifact{{Digest: digest, SizeBytes: 10, ModifiedAt: asOf.Add(-48 * time.Hour)}},
	}, custody.CleanupPlanOptions{OrphanGrace: 24 * time.Hour, AsOf: asOf})
	if err != nil {
		t.Fatal(err)
	}
	if state != custody.CleanupCandidateState {
		plan.Entries[0].State = state
	}
	planDigest, err := custody.CleanupPlanDigest(plan)
	if err != nil {
		t.Fatal(err)
	}
	target, err := NewCleanupArtifactAuthorizationTarget(digest, planDigest, asOf.Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	policy := CleanupPolicyReference{
		Schema:          CleanupPolicyReferenceSchema,
		TargetDigest:    digest,
		PolicyID:        "governance.cleanup",
		PolicyVersion:   "v1",
		SnapshotDigest:  "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		RetentionRule:   "orphan-grace-v1",
		SourceTimestamp: asOf.Format(time.RFC3339),
		ExpiresAt:       asOf.Add(2 * time.Hour).Format(time.RFC3339),
		EvaluatedAt:     asOf.Add(30 * time.Minute).Format(time.RFC3339),
		Status:          CleanupEvidenceStatusComplete,
	}
	hold := CleanupHoldReference{
		Schema:         CleanupHoldReferenceSchema,
		TargetDigest:   digest,
		SnapshotDigest: "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		Decision:       CleanupHoldDecisionNotHeld,
		EvaluatedAt:    asOf.Add(30 * time.Minute).Format(time.RFC3339),
		Status:         CleanupEvidenceStatusComplete,
	}
	request, err := NewCleanupAuthorizationRequest(target, policy, hold,
		AuthorizationPrincipalReference{Authority: "amber", Reference: "amber:principal:cleanup-ready"},
		AuthorizationEvidence{
			Authority:                "governance.example",
			Decision:                 AuthorizationDecisionAuthorized,
			AssertionDigest:          "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
			PolicyDigest:             policy.SnapshotDigest,
			IssuedAt:                 asOf.Format(time.RFC3339),
			NotBefore:                asOf.Format(time.RFC3339),
			ExpiresAt:                asOf.Add(time.Hour).Format(time.RFC3339),
			RevocationSnapshotDigest: "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		}, asOf.Add(30*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	handoffDigest, err := AuthorizationHandoffDigest(request.Handoff)
	if err != nil {
		t.Fatal(err)
	}
	result := CleanupAuthorizationResult{
		Schema:             CleanupAuthorizationResultSchema,
		HandoffDigest:      handoffDigest,
		HoldSnapshotDigest: hold.SnapshotDigest,
		HoldDecision:       hold.Decision,
		Authorization: AuthorizationVerificationResult{
			TargetDigest:             request.Handoff.Target.Digest,
			Action:                   AuthorizationActionCleanupDelete,
			PrincipalReference:       request.Handoff.Principal.Reference,
			AuthorizationDecision:    AuthorizationDecisionAuthorized,
			AuthorizationVerified:    true,
			AssertionDigest:          request.Handoff.Authorization.AssertionDigest,
			PolicyDigest:             policy.SnapshotDigest,
			RevocationSnapshotDigest: request.Handoff.Authorization.RevocationSnapshotDigest,
			EvaluatedAt:              request.EvaluatedAt,
		},
	}
	return plan, target, request, result
}

func TestEvaluateCleanupReadinessStopsBeforeDestructiveEligibility(t *testing.T) {
	plan, target, request, result := cleanupReadinessFixture(t, custody.CleanupCandidateState)
	report := EvaluateCleanupReadiness(plan, target, request, result)
	if report.State != CleanupReadinessReadyForRevalidation || report.ActionStatus != custody.CleanupNotAuthorized {
		t.Fatalf("cleanup readiness report = %+v", report)
	}
	if len(report.Blockers) != 1 || !strings.Contains(report.Blockers[0], "fresh exclusive local revalidation") {
		t.Fatalf("cleanup readiness blockers = %+v", report.Blockers)
	}
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
	encoded, err := MarshalCanonicalCleanupReadinessReport(report)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := UnmarshalCanonicalCleanupReadinessReport(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.State != report.State || decoded.CleanupPlanDigest != report.CleanupPlanDigest {
		t.Fatalf("decoded cleanup readiness report = %+v, want %+v", decoded, report)
	}
}

func TestEvaluateCleanupReadinessReportsPlanAndEvidenceBlockers(t *testing.T) {
	plan, target, request, result := cleanupReadinessFixture(t, custody.ReportedOrphanState)
	report := EvaluateCleanupReadiness(plan, target, request, result)
	if report.State != CleanupReadinessBlocked || !strings.Contains(strings.Join(report.Blockers, " | "), "not a cleanup-candidate") {
		t.Fatalf("reported orphan readiness = %+v", report)
	}

	plan, target, request, result = cleanupReadinessFixture(t, custody.CleanupCandidateState)
	target.CleanupPlanDigest = "sha256:9999999999999999999999999999999999999999999999999999999999999999"
	report = EvaluateCleanupReadiness(plan, target, request, result)
	if report.State != CleanupReadinessBlocked || !strings.Contains(strings.Join(report.Blockers, " | "), "plan digest") {
		t.Fatalf("plan mismatch readiness = %+v", report)
	}
}
