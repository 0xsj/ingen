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

func TestFindPublishedAttestationsByTargetAndKey(t *testing.T) {
	artifacts := store.NewMemory()
	record := attestationRecord()
	firstKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{23}, ed25519.SeedSize))
	secondKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{24}, ed25519.SeedSize))
	for keyID, privateKey := range map[string]ed25519.PrivateKey{
		"review-key-2026-01": firstKey,
		"review-key-2026-02": secondKey,
	} {
		envelope, err := Sign(record, keyID, privateKey)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Publish(envelope, artifacts); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := artifacts.Put(strings.NewReader("ordinary artifact"), store.PutOptions{MediaType: "text/plain"}); err != nil {
		t.Fatal(err)
	}
	targetDigest, err := custody.CanonicalDigest(record)
	if err != nil {
		t.Fatal(err)
	}
	results, err := Find(artifacts, Query{TargetDigest: targetDigest})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("target results = %+v, want two attestations", results)
	}
	results, err = Find(artifacts, Query{KeyID: "review-key-2026-02"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Envelope.KeyID != "review-key-2026-02" {
		t.Fatalf("key results = %+v", results)
	}
	results, err = Find(artifacts, Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("all results = %+v, want two attestations", results)
	}
}

func TestFindRejectsInvalidTargetQuery(t *testing.T) {
	if _, err := Find(store.NewMemory(), Query{TargetDigest: "invalid"}); err == nil || !strings.Contains(err.Error(), "invalid artifact digest") {
		t.Fatalf("invalid-target error = %v", err)
	}
}

func TestFindRequiresReferenceInventory(t *testing.T) {
	var artifacts store.Store = storeWithoutReferenceLister{Store: store.NewMemory()}
	if _, err := Find(artifacts, Query{}); err == nil || !strings.Contains(err.Error(), "reference inventory is not supported") {
		t.Fatalf("missing-inventory error = %v", err)
	}
}

func TestFindTrustedVerifiesTargetRecordAndRegistry(t *testing.T) {
	artifacts := store.NewMemory()
	records := custody.NewMemory()
	record := attestationRecord()
	if err := records.Put(record); err != nil {
		t.Fatal(err)
	}
	if _, err := artifacts.Put(strings.NewReader("attested artifact"), store.PutOptions{
		ExpectedDigest: record.Artifact.Digest,
		MediaType:      record.Artifact.MediaType,
		LogicalName:    record.Artifact.LogicalName,
	}); err != nil {
		t.Fatal(err)
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{25}, ed25519.SeedSize))
	envelope, err := Sign(record, "review-key-2026-01", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := Publish(envelope, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	registry := TrustRegistry{
		Schema: TrustRegistrySchema,
		Keys:   []TrustedKey{trustedKey("review-key-2026-01", privateKey.Public().(ed25519.PublicKey), KeyActive)},
	}
	results, err := FindTrusted(records, artifacts, Query{TargetDigest: envelope.Target.Digest}, registry, time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Artifact.Digest != ref.Digest || !results[0].Verification.Trusted {
		t.Fatalf("trusted inventory results = %+v", results)
	}
}

func TestFindTrustedFailsClosedWhenTargetRecordIsMissing(t *testing.T) {
	artifacts := store.NewMemory()
	record := attestationRecord()
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{26}, ed25519.SeedSize))
	envelope, err := Sign(record, "review-key-2026-01", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Publish(envelope, artifacts); err != nil {
		t.Fatal(err)
	}
	registry := TrustRegistry{
		Schema: TrustRegistrySchema,
		Keys:   []TrustedKey{trustedKey("review-key-2026-01", privateKey.Public().(ed25519.PublicKey), KeyActive)},
	}
	if _, err := FindTrusted(custody.NewMemory(), artifacts, Query{}, registry, time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)); err == nil || !strings.Contains(err.Error(), "target custody record") {
		t.Fatalf("missing-target error = %v", err)
	}
}

type storeWithoutReferenceLister struct {
	store.Store
}
