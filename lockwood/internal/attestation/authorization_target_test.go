package attestation

import (
	"bytes"
	"strings"
	"testing"

	"ingen/lockwood/internal/custody"
)

func TestAuthorizationTargetConstructorsBindLocalObjects(t *testing.T) {
	_, _, source, event, promoted := redactionProvenanceFixture(t)
	redactionTarget, err := NewRedactionPromotionAuthorizationTarget(source, event, promoted)
	if err != nil {
		t.Fatal(err)
	}
	provenanceTarget, err := redactionProvenanceTarget(source, event, promoted)
	if err != nil {
		t.Fatal(err)
	}
	provenanceDigest, err := CanonicalRedactionProvenanceTargetDigest(provenanceTarget)
	if err != nil {
		t.Fatal(err)
	}
	if redactionTarget.Kind != AuthorizationActionRedactionPromotion || redactionTarget.Digest != provenanceDigest {
		t.Fatalf("redaction authorization target = %+v, want digest %q", redactionTarget, provenanceDigest)
	}

	handlingTarget, err := NewHandlingEventAuthorizationTarget(event)
	if err != nil {
		t.Fatal(err)
	}
	if handlingTarget.Kind != AuthorizationActionHandlingEvent || handlingTarget.Digest == "" {
		t.Fatalf("handling authorization target = %+v", handlingTarget)
	}
	changedEvent := event
	changedEvent.EventID = "event-redaction-promotion-target-0002"
	changedHandlingTarget, err := NewHandlingEventAuthorizationTarget(changedEvent)
	if err != nil {
		t.Fatal(err)
	}
	if changedHandlingTarget.Digest == handlingTarget.Digest {
		t.Fatal("handling authorization target did not change with event identity")
	}

	recordTarget, err := NewCustodyRecordAuthorizationTarget(source)
	if err != nil {
		t.Fatal(err)
	}
	if recordTarget.Kind != AuthorizationActionCustodyRecord || recordTarget.Digest == "" {
		t.Fatalf("custody-record authorization target = %+v", recordTarget)
	}
	changedSource := source
	changedSource.CustodyID = "lockwood-authorization-target-source-0002"
	changedRecordTarget, err := NewCustodyRecordAuthorizationTarget(changedSource)
	if err != nil {
		t.Fatal(err)
	}
	if changedRecordTarget.Digest == recordTarget.Digest {
		t.Fatal("custody-record authorization target did not change with record identity")
	}
}

func TestAuthorizationTargetConstructorsRejectInvalidObjects(t *testing.T) {
	invalidEvent := attestedHandlingEvent()
	invalidEvent.Type = custody.HandlingEventType("unknown")
	if _, err := NewHandlingEventAuthorizationTarget(invalidEvent); err == nil {
		t.Fatal("invalid handling event accepted")
	}
	_, _, source, _, _ := redactionProvenanceFixture(t)
	source.Schema = "invalid"
	if _, err := NewCustodyRecordAuthorizationTarget(source); err == nil {
		t.Fatal("invalid custody record accepted")
	}
}

func TestCleanupAuthorizationTargetBindsArtifactAndPlan(t *testing.T) {
	target, err := NewCleanupArtifactAuthorizationTarget(
		"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"2026-09-17T12:00:00Z",
	)
	if err != nil {
		t.Fatal(err)
	}
	if target.Kind != AuthorizationTargetArtifactBlob || target.Action != AuthorizationActionCleanupDelete {
		t.Fatalf("cleanup target = %+v", target)
	}
	encoded, err := MarshalCanonicalCleanupAuthorizationTarget(target)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := UnmarshalCanonicalCleanupAuthorizationTarget(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != target {
		t.Fatalf("decoded cleanup target = %+v, want %+v", decoded, target)
	}
	handoffTarget, err := target.AuthorizationTarget()
	if err != nil {
		t.Fatal(err)
	}
	if handoffTarget.Kind != AuthorizationTargetArtifactBlob || handoffTarget.Digest == "" {
		t.Fatalf("generic cleanup target = %+v", handoffTarget)
	}

	changed := target
	changed.CleanupPlanDigest = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	changedDigest, err := CleanupAuthorizationTargetDigest(changed)
	if err != nil {
		t.Fatal(err)
	}
	if changedDigest == handoffTarget.Digest {
		t.Fatal("cleanup target digest did not change with plan binding")
	}

	if _, err := UnmarshalCanonicalCleanupAuthorizationTarget(append([]byte(" \n"), encoded...)); err == nil || !strings.Contains(err.Error(), "not canonical JSON") {
		t.Fatalf("noncanonical cleanup target accepted: %v", err)
	}
	if !bytes.Equal(encoded, mustCanonicalCleanupTarget(t, target)) {
		t.Fatal("cleanup target canonical encoding was not stable")
	}
}

func mustCanonicalCleanupTarget(t *testing.T, target CleanupAuthorizationTarget) []byte {
	t.Helper()
	encoded, err := MarshalCanonicalCleanupAuthorizationTarget(target)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
