package attestation

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"ingen/lockwood/internal/store"
)

func TestTrustRegistryCanonicalEncodingAndDigest(t *testing.T) {
	firstPrivate := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{31}, ed25519.SeedSize))
	secondPrivate := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{32}, ed25519.SeedSize))
	registry := TrustRegistry{
		Schema: TrustRegistrySchema,
		Keys: []TrustedKey{
			trustedKey("review-key-2026-02", secondPrivate.Public().(ed25519.PublicKey), KeyActive),
			trustedKey("review-key-2026-01", firstPrivate.Public().(ed25519.PublicKey), KeyActive),
		},
	}
	encoded, err := MarshalCanonicalTrustRegistry(registry)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"review-key-2026-01"`) || strings.Index(string(encoded), `"review-key-2026-01"`) > strings.Index(string(encoded), `"review-key-2026-02"`) {
		t.Fatalf("registry keys are not canonically sorted: %s", encoded)
	}
	decoded, err := UnmarshalCanonicalTrustRegistry(encoded)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := TrustRegistryDigest(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if digest == "" {
		t.Fatal("trust registry digest is empty")
	}
	if _, err := UnmarshalCanonicalTrustRegistry([]byte(`{"schema":"lockwood.attestation-trust/v1","keys":[]}`)); err != nil {
		t.Fatalf("empty registry should be structurally valid: %v", err)
	}
}

func TestTrustRegistryRejectsNonCanonicalOrInvalidEntries(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{33}, ed25519.SeedSize))
	valid := TrustRegistry{
		Schema: TrustRegistrySchema,
		Keys:   []TrustedKey{trustedKey("review-key-2026-01", privateKey.Public().(ed25519.PublicKey), KeyActive)},
	}
	encoded, err := MarshalCanonicalTrustRegistry(valid)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		data []byte
		want string
	}{
		{name: "whitespace", data: append([]byte(" \n"), encoded...), want: "not canonical JSON"},
		{name: "unknown field", data: []byte(`{"schema":"lockwood.attestation-trust/v1","keys":[],"extra":true}`), want: "decode attestation trust registry"},
		{name: "invalid timestamp", data: []byte(`{"schema":"lockwood.attestation-trust/v1","keys":[{"key_id":"review-key-2026-01","algorithm":"ed25519","public_key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","status":"active","not_before":"2026-01-01T00:00:00+00:00"}]}`), want: "canonical RFC3339 UTC"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := UnmarshalCanonicalTrustRegistry(test.data); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
	duplicate := valid
	duplicate.Keys = append(append([]TrustedKey(nil), valid.Keys...), valid.Keys[0])
	if _, err := MarshalCanonicalTrustRegistry(duplicate); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate-key error = %v", err)
	}
	noncanonicalKey := valid
	noncanonicalKey.Keys = append([]TrustedKey(nil), valid.Keys...)
	noncanonicalKey.Keys[0].PublicKey += "\n"
	if _, err := MarshalCanonicalTrustRegistry(noncanonicalKey); err == nil || !strings.Contains(err.Error(), "canonical standard-base64") {
		t.Fatalf("noncanonical-key error = %v", err)
	}
}

func TestTrustRegistryResolveValidityAndRevocation(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{34}, ed25519.SeedSize))
	registry := TrustRegistry{
		Schema: TrustRegistrySchema,
		Keys: []TrustedKey{{
			KeyID:     "review-key-2026-01",
			Algorithm: Algorithm,
			PublicKey: base64.StdEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey)),
			Status:    KeyActive,
			NotBefore: "2026-01-01T00:00:00Z",
			NotAfter:  "2027-01-01T00:00:00Z",
		}, {
			KeyID:     "review-key-2025-01",
			Algorithm: Algorithm,
			PublicKey: base64.StdEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey)),
			Status:    KeyRevoked,
		}},
	}
	at := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	resolved, err := registry.Resolve("review-key-2026-01", at)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(resolved, privateKey.Public().(ed25519.PublicKey)) {
		t.Fatal("resolved public key differs from registry")
	}
	for _, test := range []struct {
		name  string
		keyID string
		at    time.Time
		want  string
	}{
		{name: "before activation", keyID: "review-key-2026-01", at: time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC), want: "not yet valid"},
		{name: "after expiry", keyID: "review-key-2026-01", at: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), want: "expired"},
		{name: "revoked", keyID: "review-key-2025-01", at: at, want: "revoked"},
		{name: "missing", keyID: "review-key-missing", at: at, want: "not found"},
		{name: "missing time", keyID: "review-key-2026-01", want: "evaluation time is required"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := registry.Resolve(test.keyID, test.at); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestVerifyPublishedWithTrustRegistry(t *testing.T) {
	record := attestationRecord()
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{35}, ed25519.SeedSize))
	envelope, err := Sign(record, "review-key-2026-01", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	artifacts := store.NewMemory()
	ref, err := Publish(envelope, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	registry := TrustRegistry{
		Schema: TrustRegistrySchema,
		Keys:   []TrustedKey{trustedKey("review-key-2026-01", privateKey.Public().(ed25519.PublicKey), KeyActive)},
	}
	evaluatedAt := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	receipt, err := VerifyPublishedWithRegistryReceipt(record, ref.Digest, artifacts, registry, evaluatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Verified || !receipt.Trusted || receipt.KeyID != envelope.KeyID || receipt.EvaluatedAt != "2026-09-16T12:00:00Z" || receipt.RegistryDigest == "" {
		t.Fatalf("trusted verification receipt = %+v", receipt)
	}
	revoked := registry
	revoked.Keys = []TrustedKey{trustedKey("review-key-2026-01", privateKey.Public().(ed25519.PublicKey), KeyRevoked)}
	if err := VerifyPublishedWithRegistry(record, ref.Digest, artifacts, revoked, evaluatedAt); err == nil || !strings.Contains(err.Error(), "revoked") {
		t.Fatalf("revoked-key verification error = %v", err)
	}
}

func trustedKey(keyID string, publicKey ed25519.PublicKey, status string) TrustedKey {
	return TrustedKey{
		KeyID:     keyID,
		Algorithm: Algorithm,
		PublicKey: base64.StdEncoding.EncodeToString(publicKey),
		Status:    status,
	}
}
