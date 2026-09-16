package attestation

import (
	"bytes"
	"crypto/ed25519"
	"fmt"
	"strings"
	"testing"
	"time"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/custody"
	"ingen/lockwood/internal/store"
)

func TestSignAndVerifyCanonicalCustodyRecord(t *testing.T) {
	record := attestationRecord()
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)

	envelope, err := Sign(record, "review-key-2026-01", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := envelope.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := Verify(record, envelope, publicKey); err != nil {
		t.Fatal(err)
	}
	message, err := SigningMessage(record, envelope.KeyID)
	if err != nil {
		t.Fatal(err)
	}
	expectedPrefix := "lockwood.attestation/v1\ncustody-record\nreview-key-2026-01\n"
	if !bytes.HasPrefix(message, []byte(expectedPrefix)) || !bytes.HasSuffix(message, []byte("\n")) {
		t.Fatalf("signing message = %q", message)
	}
}

func TestVerifyRejectsRecordChangesAndWrongKeys(t *testing.T) {
	record := attestationRecord()
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{8}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	envelope, err := Sign(record, "review-key-2026-01", privateKey)
	if err != nil {
		t.Fatal(err)
	}

	changed := record
	changed.Source.Path = "changed.json"
	if err := Verify(changed, envelope, publicKey); err == nil || !strings.Contains(err.Error(), "target digest mismatch") {
		t.Fatalf("changed-record verification error = %v", err)
	}
	wrongPrivateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{9}, ed25519.SeedSize))
	wrongPublicKey := wrongPrivateKey.Public().(ed25519.PublicKey)
	if err := Verify(record, envelope, wrongPublicKey); err == nil || !strings.Contains(err.Error(), "verification failed") {
		t.Fatalf("wrong-key verification error = %v", err)
	}
	relabeled := envelope
	relabeled.KeyID = "other-review-key"
	if err := Verify(record, relabeled, publicKey); err == nil || !strings.Contains(err.Error(), "verification failed") {
		t.Fatalf("relabelled-key verification error = %v", err)
	}
}

func TestVerifyWithKeySetUsesExactEnvelopeKeyID(t *testing.T) {
	record := attestationRecord()
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{11}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	envelope, err := Sign(record, "review-key-2026-01", privateKey)
	if err != nil {
		t.Fatal(err)
	}

	keys := PublicKeySet{"review-key-2026-01": publicKey}
	if err := VerifyWithKeySet(record, envelope, keys); err != nil {
		t.Fatalf("key-set verification failed: %v", err)
	}

	missing := envelope
	missing.KeyID = "missing-key"
	if err := VerifyWithKeySet(record, missing, keys); err == nil || !strings.Contains(err.Error(), "not found in supplied key set") {
		t.Fatalf("missing-key error = %v", err)
	}

	wrong := PublicKeySet{"review-key-2026-01": ed25519.NewKeyFromSeed(bytes.Repeat([]byte{12}, ed25519.SeedSize)).Public().(ed25519.PublicKey)}
	if err := VerifyWithKeySet(record, envelope, wrong); err == nil || !strings.Contains(err.Error(), "verification failed") {
		t.Fatalf("wrong-key-set error = %v", err)
	}
}

func TestCanonicalEnvelopeRoundTripsWithStableDigest(t *testing.T) {
	record := attestationRecord()
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{13}, ed25519.SeedSize))
	envelope, err := Sign(record, "review-key-2026-01", privateKey)
	if err != nil {
		t.Fatal(err)
	}

	encoded, err := MarshalCanonical(envelope)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := CanonicalDigest(envelope)
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf(`{"schema":"lockwood.attestation/v1","target":{"kind":"custody-record","digest":"%s"},"algorithm":"ed25519","key_id":"review-key-2026-01","signature":"%s"}`, envelope.Target.Digest, envelope.Signature)
	if string(encoded) != want {
		t.Fatalf("canonical envelope = %s, want %s", encoded, want)
	}
	decoded, err := UnmarshalCanonical(encoded)
	if err != nil {
		t.Fatal(err)
	}
	decodedDigest, err := CanonicalDigest(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, mustMarshalCanonical(t, decoded)) {
		t.Fatal("canonical envelope changed after round trip")
	}
	if decodedDigest != digest {
		t.Fatalf("canonical envelope digest changed: %s vs %s", decodedDigest, digest)
	}
}

func TestUnmarshalCanonicalRejectsNonCanonicalEnvelope(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{14}, ed25519.SeedSize))
	envelope, err := Sign(attestationRecord(), "review-key-2026-01", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := MarshalCanonical(envelope)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{name: "formatted", data: append([]byte("{\n  "), encoded[1:]...), want: "not canonical JSON"},
		{name: "unknown field", data: append(append([]byte{}, encoded[:len(encoded)-1]...), []byte(`,"extra":true}`)...), want: "unknown field"},
		{name: "multiple values", data: append(append([]byte{}, encoded...), encoded...), want: "multiple JSON values"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := UnmarshalCanonical(test.data); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestPublishStoresCanonicalEnvelope(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{15}, ed25519.SeedSize))
	envelope, err := Sign(attestationRecord(), "review-key-2026-01", privateKey)
	if err != nil {
		t.Fatal(err)
	}

	artifacts := store.NewMemory()
	ref, err := Publish(envelope, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if ref.MediaType != MediaType || ref.LogicalName != DefaultLogicalName {
		t.Fatalf("published reference metadata = %#v", ref)
	}
	wantDigest, err := CanonicalDigest(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if ref.Digest != wantDigest {
		t.Fatalf("published digest = %s, want %s", ref.Digest, wantDigest)
	}
	data, err := artifacts.Get(ref.Digest)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := UnmarshalCanonical(data)
	if err != nil {
		t.Fatal(err)
	}
	publicKey := privateKey.Public().(ed25519.PublicKey)
	if err := Verify(attestationRecord(), decoded, publicKey); err != nil {
		t.Fatalf("published envelope failed verification: %v", err)
	}
}

func TestPublishForRecordBindsEnvelopeToRecord(t *testing.T) {
	record := attestationRecord()
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{21}, ed25519.SeedSize))
	envelope, err := Sign(record, "review-key-2026-01", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := PublishForRecord(record, envelope, store.NewMemory())
	if err != nil {
		t.Fatal(err)
	}
	if publication.CustodyID != record.CustodyID || publication.Artifact.Digest == "" {
		t.Fatalf("publication receipt = %+v", publication)
	}

	wrongTarget := envelope
	wrongTarget.Target.Digest = artifact.DigestBytes([]byte("different custody record"))
	if _, err := PublishForRecord(record, wrongTarget, store.NewMemory()); err == nil || !strings.Contains(err.Error(), "target digest mismatch") {
		t.Fatalf("wrong-target publication error = %v", err)
	}
}

func TestPublishRequiresArtifactStore(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{16}, ed25519.SeedSize))
	envelope, err := Sign(attestationRecord(), "review-key-2026-01", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Publish(envelope, nil); err == nil || !strings.Contains(err.Error(), "artifact store is required") {
		t.Fatalf("nil-store error = %v", err)
	}
}

func TestEnvelopeRejectsMalformedValues(t *testing.T) {
	record := attestationRecord()
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{10}, ed25519.SeedSize))
	envelope, err := Sign(record, "review-key-2026-01", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		edit func(*Envelope)
	}{
		{name: "schema", edit: func(envelope *Envelope) { envelope.Schema = "unknown" }},
		{name: "key id", edit: func(envelope *Envelope) { envelope.KeyID = "bad key" }},
		{name: "signature", edit: func(envelope *Envelope) { envelope.Signature = "not-base64" }},
		{name: "noncanonical signature", edit: func(envelope *Envelope) { envelope.Signature += "\n" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invalid := envelope
			test.edit(&invalid)
			if err := invalid.Validate(); err == nil {
				t.Fatal("malformed envelope was accepted")
			}
		})
	}
}

func TestSignRejectsWrongPrivateKeySize(t *testing.T) {
	if _, err := Sign(attestationRecord(), "review-key-2026-01", ed25519.PrivateKey{}); err == nil || !strings.Contains(err.Error(), "private key has size") {
		t.Fatalf("invalid private-key error = %v", err)
	}
}

func TestLoadAndVerifyPublishedAttestation(t *testing.T) {
	record := attestationRecord()
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{17}, ed25519.SeedSize))
	envelope, err := Sign(record, "review-key-2026-01", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	artifacts := store.NewMemory()
	ref, err := Publish(envelope, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	keys := PublicKeySet{"review-key-2026-01": privateKey.Public().(ed25519.PublicKey)}
	receipt, err := VerifyPublishedReceipt(record, ref.Digest, artifacts, keys)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.CustodyID != record.CustodyID || receipt.AttestationDigest != ref.Digest || receipt.TargetDigest != envelope.Target.Digest || receipt.KeyID != envelope.KeyID || receipt.Algorithm != Algorithm || !receipt.Verified {
		t.Fatalf("verification receipt = %+v", receipt)
	}
	if err := VerifyPublished(record, ref.Digest, artifacts, keys); err != nil {
		t.Fatalf("verification without receipt failed: %v", err)
	}

	changed := record
	changed.Source.Path = "changed.json"
	if err := VerifyPublished(changed, ref.Digest, artifacts, keys); err == nil || !strings.Contains(err.Error(), "target digest mismatch") {
		t.Fatalf("changed-record verification error = %v", err)
	}
}

func TestLoadRejectsMalformedPublishedArtifact(t *testing.T) {
	artifacts := store.NewMemory()
	ref, err := artifacts.Put(strings.NewReader(`{"schema":`), store.PutOptions{MediaType: MediaType})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Load(artifacts, ref.Digest); err == nil || !strings.Contains(err.Error(), "decode attestation envelope") {
		t.Fatalf("malformed-artifact error = %v", err)
	}
}

func TestLoadRejectsInvalidDigestBeforeStoreAccess(t *testing.T) {
	if _, err := Load(store.NewMemory(), "not-a-digest"); err == nil || !strings.Contains(err.Error(), "invalid artifact digest") {
		t.Fatalf("invalid-digest error = %v", err)
	}
}

func attestationRecord() custody.Record {
	verifiedAt := time.Date(2026, 9, 16, 12, 0, 1, 0, time.UTC)
	return custody.Record{
		Schema:    custody.SchemaV1,
		CustodyID: "lockwood-attestation-record-0001",
		Status:    custody.Accepted,
		Artifact: artifact.Reference{
			Schema:      artifact.Schema,
			Digest:      artifact.DigestBytes([]byte("attested artifact")),
			SizeBytes:   int64(len("attested artifact")),
			MediaType:   "text/plain",
			LogicalName: "attested.txt",
		},
		ReceivedAt: time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC),
		Producer:   custody.Producer{Tool: "example", Kind: "attestation-fixture"},
		Custodian:  custody.Custodian{Tool: "lockwood"},
		Source:     custody.Source{Path: "attested.txt"},
		Integrity:  custody.Integrity{Status: custody.IntegrityVerified, Method: artifact.SHA256Algorithm, VerifiedAt: &verifiedAt},
		Parents:    []custody.Lineage{},
		Handling:   custody.Handling{Redaction: "none", RetentionClass: "default"},
	}
}

func mustMarshalCanonical(t *testing.T, envelope Envelope) []byte {
	t.Helper()
	encoded, err := MarshalCanonical(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
