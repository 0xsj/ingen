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

func TestFindRedactionProvenanceByRelationshipAndKey(t *testing.T) {
	artifacts, records, source, event, promoted := redactionProvenanceFixture(t)
	firstKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{61}, ed25519.SeedSize))
	secondKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{62}, ed25519.SeedSize))
	for keyID, privateKey := range map[string]ed25519.PrivateKey{
		"provenance-key-2026-01": firstKey,
		"provenance-key-2026-02": secondKey,
	} {
		envelope, err := SignRedactionProvenance(source, event, promoted, keyID, privateKey)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := PublishRedactionProvenance(source, event, promoted, envelope, artifacts); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := artifacts.Put(strings.NewReader("ordinary inventory artifact"), store.PutOptions{MediaType: "text/plain"}); err != nil {
		t.Fatal(err)
	}

	results, err := FindRedactionProvenance(artifacts, RedactionProvenanceQuery{SourceCustodyID: source.CustodyID, EventID: event.EventID, PromotedCustodyID: promoted.CustodyID})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Envelope.KeyID != "provenance-key-2026-01" || results[1].Envelope.KeyID != "provenance-key-2026-02" {
		t.Fatalf("relationship inventory = %+v", results)
	}
	results, err = FindRedactionProvenance(artifacts, RedactionProvenanceQuery{KeyID: "provenance-key-2026-02"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Envelope.Target.SourceCustodyID != source.CustodyID {
		t.Fatalf("key inventory = %+v", results)
	}

	registry := TrustRegistry{
		Schema: TrustRegistrySchema,
		Keys: []TrustedKey{
			trustedKey("provenance-key-2026-01", firstKey.Public().(ed25519.PublicKey), KeyActive),
			trustedKey("provenance-key-2026-02", secondKey.Public().(ed25519.PublicKey), KeyActive),
		},
	}
	events, ok := records.(custody.HandlingEventStore)
	if !ok {
		t.Fatal("fixture records do not expose handling events")
	}
	trusted, err := FindTrustedRedactionProvenance(records, events, artifacts, RedactionProvenanceQuery{}, registry, time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(trusted) != 2 || !trusted[0].Verification.Trusted || !trusted[1].Verification.Trusted {
		t.Fatalf("trusted relationship inventory = %+v", trusted)
	}
}

func TestFindRedactionProvenanceRejectsInvalidQuery(t *testing.T) {
	if _, err := FindRedactionProvenance(store.NewMemory(), RedactionProvenanceQuery{EventID: "bad event"}); err == nil || !strings.Contains(err.Error(), "invalid redaction provenance event id") {
		t.Fatalf("invalid-event query error = %v", err)
	}
	if _, err := FindRedactionProvenance(store.NewMemory(), RedactionProvenanceQuery{KeyID: "bad key"}); err == nil || !strings.Contains(err.Error(), "invalid redaction provenance key id") {
		t.Fatalf("invalid-key query error = %v", err)
	}
}

func TestFindTrustedRedactionProvenanceFailsClosedOnMissingEvent(t *testing.T) {
	artifacts, records, source, event, promoted := redactionProvenanceFixture(t)
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{63}, ed25519.SeedSize))
	envelope, err := SignRedactionProvenance(source, event, promoted, "provenance-key-2026-03", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PublishRedactionProvenance(source, event, promoted, envelope, artifacts); err != nil {
		t.Fatal(err)
	}
	missingEvents := custody.NewMemory()
	registry := TrustRegistry{
		Schema: TrustRegistrySchema,
		Keys:   []TrustedKey{trustedKey("provenance-key-2026-03", privateKey.Public().(ed25519.PublicKey), KeyActive)},
	}
	if _, err := FindTrustedRedactionProvenance(records, missingEvents, artifacts, RedactionProvenanceQuery{}, registry, time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)); err == nil || !strings.Contains(err.Error(), "redaction provenance event") {
		t.Fatalf("missing-event error = %v", err)
	}
}
