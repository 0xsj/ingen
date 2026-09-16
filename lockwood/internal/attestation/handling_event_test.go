package attestation

import (
	"bytes"
	"crypto/ed25519"
	"strings"
	"testing"
	"time"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/custody"
	"ingen/lockwood/internal/store"
)

func attestedHandlingEvent() custody.HandlingEvent {
	return custody.HandlingEvent{
		Schema:         custody.HandlingEventSchema,
		EventID:        "event-authenticated-0001",
		CustodyID:      "lockwood-authenticated-event-record",
		Type:           custody.RetentionClassifiedEvent,
		RecordedAt:     time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC),
		Actor:          "operator@example",
		Reason:         "retention decision recorded",
		RetentionClass: "regulated-7y",
	}
}

func TestSignAndVerifyHandlingEventEnvelope(t *testing.T) {
	event := attestedHandlingEvent()
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{41}, ed25519.SeedSize))
	envelope, err := SignHandlingEvent(event, "handling-key-2026-01", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := envelope.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := VerifyHandlingEvent(event, envelope, privateKey.Public().(ed25519.PublicKey)); err != nil {
		t.Fatal(err)
	}
	message, err := handlingEventSigningMessage(event.CustodyID, event.EventID, envelope.Target.Digest, envelope.KeyID)
	if err != nil {
		t.Fatal(err)
	}
	expectedPrefix := "lockwood.handling-event-attestation/v1\nhandling-event\nhandling-key-2026-01\n"
	if !bytes.HasPrefix(message, []byte(expectedPrefix)) || !bytes.HasSuffix(message, []byte("\n")) {
		t.Fatalf("handling event signing message = %q", message)
	}

	encoded, err := MarshalCanonicalHandlingEventEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := UnmarshalCanonicalHandlingEventEnvelope(encoded)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := CanonicalHandlingEventEnvelopeDigest(decoded)
	if err != nil || digest == "" {
		t.Fatalf("handling event envelope digest = %q, err = %v", digest, err)
	}

	changed := event
	changed.Reason = "changed after signing"
	if err := VerifyHandlingEvent(changed, envelope, privateKey.Public().(ed25519.PublicKey)); err == nil || !strings.Contains(err.Error(), "target digest mismatch") {
		t.Fatalf("changed-event verification error = %v", err)
	}
	wrongKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{42}, ed25519.SeedSize))
	if err := VerifyHandlingEvent(event, envelope, wrongKey.Public().(ed25519.PublicKey)); err == nil || !strings.Contains(err.Error(), "verification failed") {
		t.Fatalf("wrong-key verification error = %v", err)
	}
}

func TestPublishAndVerifyHandlingEventThroughTrustRegistry(t *testing.T) {
	event := attestedHandlingEvent()
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{43}, ed25519.SeedSize))
	envelope, err := SignHandlingEvent(event, "handling-key-2026-01", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	artifacts := store.NewMemory()
	publication, err := PublishHandlingEvent(event, envelope, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	eventDigest, err := custody.HandlingEventDigest(event)
	if err != nil {
		t.Fatal(err)
	}
	if publication.EventDigest != eventDigest || publication.Artifact.MediaType != HandlingEventMediaType {
		t.Fatalf("handling event publication = %+v", publication)
	}
	loaded, err := LoadHandlingEventEnvelope(artifacts, publication.Artifact.Digest)
	if err != nil {
		t.Fatal(err)
	}
	registry := TrustRegistry{
		Schema: TrustRegistrySchema,
		Keys:   []TrustedKey{trustedKey("handling-key-2026-01", privateKey.Public().(ed25519.PublicKey), KeyActive)},
	}
	evaluatedAt := time.Date(2026, 9, 16, 12, 1, 0, 0, time.UTC)
	receipt, err := VerifyPublishedHandlingEventWithRegistryReceipt(event, publication.Artifact.Digest, artifacts, registry, evaluatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Verified || !receipt.Trusted || receipt.EventID != event.EventID || receipt.EventDigest != eventDigest || receipt.KeyID != loaded.KeyID || receipt.RegistryDigest == "" || receipt.EvaluatedAt != "2026-09-16T12:01:00Z" {
		t.Fatalf("handling event verification receipt = %+v", receipt)
	}
	revoked := registry
	revoked.Keys = []TrustedKey{trustedKey("handling-key-2026-01", privateKey.Public().(ed25519.PublicKey), KeyRevoked)}
	if err := VerifyHandlingEventWithRegistry(event, envelope, revoked, evaluatedAt); err == nil || !strings.Contains(err.Error(), "revoked") {
		t.Fatalf("revoked handling key error = %v", err)
	}
}

func TestHandlingEventEnvelopeRejectsCrossTargetAndNoncanonicalSignature(t *testing.T) {
	event := attestedHandlingEvent()
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{44}, ed25519.SeedSize))
	envelope, err := SignHandlingEvent(event, "handling-key-2026-01", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	changedTarget := envelope
	changedTarget.Target.EventID = "event-other"
	if err := VerifyHandlingEvent(event, changedTarget, privateKey.Public().(ed25519.PublicKey)); err == nil || !strings.Contains(err.Error(), "target identity mismatch") {
		t.Fatalf("cross-target error = %v", err)
	}
	noncanonical := envelope
	noncanonical.Signature = envelope.Signature + "\n"
	if _, err := MarshalCanonicalHandlingEventEnvelope(noncanonical); err == nil || !strings.Contains(err.Error(), "canonical standard-base64") {
		t.Fatalf("noncanonical signature error = %v", err)
	}
	if _, err := PublishHandlingEvent(event, changedTarget, store.NewMemory()); err == nil || !strings.Contains(err.Error(), "does not match event") {
		t.Fatalf("cross-target publication error = %v", err)
	}
}

func TestPublishHandlingEventRequiresValidInputs(t *testing.T) {
	event := attestedHandlingEvent()
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{45}, ed25519.SeedSize))
	envelope, err := SignHandlingEvent(event, "handling-key-2026-01", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PublishHandlingEvent(event, envelope, nil); err == nil || !strings.Contains(err.Error(), "artifact store is required") {
		t.Fatalf("missing artifact store error = %v", err)
	}
	invalid := envelope
	invalid.Target.Digest = artifact.DigestBytes([]byte("another event"))
	if err := VerifyHandlingEvent(event, invalid, privateKey.Public().(ed25519.PublicKey)); err == nil || !strings.Contains(err.Error(), "target digest mismatch") {
		t.Fatalf("invalid event target error = %v", err)
	}
}
