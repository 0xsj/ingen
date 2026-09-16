package attestation

import (
	"bytes"
	"crypto/ed25519"
	"strings"
	"testing"
	"time"

	"ingen/lockwood/internal/custody"
	"ingen/lockwood/internal/store"
)

func TestHandlingAuthorizationPolicyCanonicalEncodingAndAuthorization(t *testing.T) {
	policy := HandlingAuthorizationPolicy{
		Schema: HandlingAuthorizationPolicySchema,
		Rules: []HandlingAuthorizationRule{
			{
				KeyID: "handling-key-2026-02",
				EventTypes: []custody.HandlingEventType{
					custody.LegalHoldReleasedEvent,
					custody.LegalHoldPlacedEvent,
				},
			},
			{
				KeyID:      "handling-key-2026-01",
				EventTypes: []custody.HandlingEventType{custody.RedactionEvent, custody.RetentionClassifiedEvent},
			},
		},
	}
	encoded, err := MarshalCanonicalHandlingAuthorizationPolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := UnmarshalCanonicalHandlingAuthorizationPolicy(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"schema":"lockwood.handling-event-policy/v1","rules":[{"key_id":"handling-key-2026-01","event_types":["redaction","retention-classified"]},{"key_id":"handling-key-2026-02","event_types":["legal-hold-placed","legal-hold-released"]}]}` {
		t.Fatalf("canonical handling policy = %s", encoded)
	}
	digest, err := HandlingAuthorizationPolicyDigest(decoded)
	if err != nil || digest == "" {
		t.Fatalf("handling policy digest = %q, err = %v", digest, err)
	}
	if err := decoded.Authorize("handling-key-2026-01", custody.RedactionEvent); err != nil {
		t.Fatalf("allowed redaction rejected: %v", err)
	}
	if err := decoded.Authorize("handling-key-2026-01", custody.LegalHoldPlacedEvent); err == nil || !strings.Contains(err.Error(), "not authorized") {
		t.Fatalf("disallowed event accepted: %v", err)
	}
	if err := decoded.Authorize("missing-key", custody.RedactionEvent); err == nil || !strings.Contains(err.Error(), "not authorized by policy") {
		t.Fatalf("unknown key accepted: %v", err)
	}
}

func TestHandlingAuthorizationPolicyRejectsInvalidRules(t *testing.T) {
	base := HandlingAuthorizationPolicy{
		Schema: HandlingAuthorizationPolicySchema,
		Rules: []HandlingAuthorizationRule{{
			KeyID:      "handling-key-2026-01",
			EventTypes: []custody.HandlingEventType{custody.RedactionEvent},
		}},
	}
	tests := []struct {
		name string
		edit func(*HandlingAuthorizationPolicy)
		want string
	}{
		{name: "duplicate key", edit: func(policy *HandlingAuthorizationPolicy) { policy.Rules = append(policy.Rules, policy.Rules[0]) }, want: "duplicate"},
		{name: "duplicate event type", edit: func(policy *HandlingAuthorizationPolicy) {
			policy.Rules[0].EventTypes = append(policy.Rules[0].EventTypes, custody.RedactionEvent)
		}, want: "repeats event type"},
		{name: "empty event types", edit: func(policy *HandlingAuthorizationPolicy) { policy.Rules[0].EventTypes = nil }, want: "at least one event type"},
		{name: "unsupported event type", edit: func(policy *HandlingAuthorizationPolicy) {
			policy.Rules[0].EventTypes = []custody.HandlingEventType{"unknown"}
		}, want: "unsupported event type"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy := base
			policy.Rules = append([]HandlingAuthorizationRule(nil), base.Rules...)
			policy.Rules[0].EventTypes = append([]custody.HandlingEventType(nil), base.Rules[0].EventTypes...)
			test.edit(&policy)
			if _, err := MarshalCanonicalHandlingAuthorizationPolicy(policy); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
	if _, err := MarshalCanonicalHandlingAuthorizationPolicy(HandlingAuthorizationPolicy{Schema: HandlingAuthorizationPolicySchema, Rules: []HandlingAuthorizationRule{}}); err != nil {
		t.Fatalf("empty deny-all policy rejected: %v", err)
	}
}

func TestVerifyPublishedHandlingEventWithPolicy(t *testing.T) {
	event := attestedHandlingEvent()
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{47}, ed25519.SeedSize))
	envelope, err := SignHandlingEvent(event, "handling-key-2026-01", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	artifacts := store.NewMemory()
	publication, err := PublishHandlingEvent(event, envelope, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	registry := TrustRegistry{
		Schema: TrustRegistrySchema,
		Keys:   []TrustedKey{trustedKey("handling-key-2026-01", privateKey.Public().(ed25519.PublicKey), KeyActive)},
	}
	policy := HandlingAuthorizationPolicy{
		Schema: HandlingAuthorizationPolicySchema,
		Rules: []HandlingAuthorizationRule{{
			KeyID:      "handling-key-2026-01",
			EventTypes: []custody.HandlingEventType{custody.RetentionClassifiedEvent},
		}},
	}
	receipt, err := VerifyPublishedHandlingEventWithRegistryAndPolicyReceipt(event, publication.Artifact.Digest, artifacts, registry, policy, time.Date(2026, 9, 16, 12, 1, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Authorized || !receipt.Trusted || receipt.PolicyDigest == "" || receipt.EventID != event.EventID {
		t.Fatalf("authorized handling event receipt = %+v", receipt)
	}
	denied := policy
	denied.Rules = []HandlingAuthorizationRule{{
		KeyID:      "handling-key-2026-01",
		EventTypes: []custody.HandlingEventType{custody.RedactionEvent},
	}}
	if _, err := VerifyPublishedHandlingEventWithRegistryAndPolicyReceipt(event, publication.Artifact.Digest, artifacts, registry, denied, time.Date(2026, 9, 16, 12, 1, 0, 0, time.UTC)); err == nil || !strings.Contains(err.Error(), "not authorized") {
		t.Fatalf("unauthorized handling event accepted: %v", err)
	}
}
