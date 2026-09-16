package attestation

import (
	"bytes"
	"crypto/ed25519"
	"strings"
	"testing"

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

type storeWithoutReferenceLister struct {
	store.Store
}
