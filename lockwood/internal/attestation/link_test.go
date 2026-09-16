package attestation

import (
	"bytes"
	"crypto/ed25519"
	"strings"
	"testing"

	"ingen/lockwood/internal/store"
)

func TestInspectLinkKeepsAttestationRecordAndPayloadDigestsDistinct(t *testing.T) {
	artifacts := store.NewMemory()
	record := attestationRecord()
	if _, err := artifacts.Put(bytes.NewBufferString("attested artifact"), store.PutOptions{
		ExpectedDigest: record.Artifact.Digest,
		MediaType:      record.Artifact.MediaType,
		LogicalName:    record.Artifact.LogicalName,
	}); err != nil {
		t.Fatal(err)
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{36}, ed25519.SeedSize))
	envelope, err := Sign(record, "review-key-2026-01", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	attestationRef, err := Publish(envelope, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	link, err := InspectLink(record, attestationRef.Digest, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if link.Relation != CustodyRecordLinkRelation || link.AttestationDigest != attestationRef.Digest || link.CustodyID != record.CustodyID || link.CustodyRecordDigest != envelope.Target.Digest || link.PayloadArtifactDigest != record.Artifact.Digest {
		t.Fatalf("link inspection = %+v", link)
	}
	if link.CustodyRecordDigest == link.PayloadArtifactDigest || link.AttestationDigest == link.PayloadArtifactDigest {
		t.Fatalf("link inspection conflated digest domains: %+v", link)
	}
}

func TestInspectLinkRejectsMismatchedRecord(t *testing.T) {
	artifacts := store.NewMemory()
	record := attestationRecord()
	if _, err := artifacts.Put(bytes.NewBufferString("attested artifact"), store.PutOptions{
		ExpectedDigest: record.Artifact.Digest,
		MediaType:      record.Artifact.MediaType,
		LogicalName:    record.Artifact.LogicalName,
	}); err != nil {
		t.Fatal(err)
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{37}, ed25519.SeedSize))
	envelope, err := Sign(record, "review-key-2026-01", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := Publish(envelope, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	wrongRecord := record
	wrongRecord.Source.Path = "different.txt"
	if _, err := InspectLink(wrongRecord, ref.Digest, artifacts); err == nil || !strings.Contains(err.Error(), "target digest mismatch") {
		t.Fatalf("mismatched-record error = %v", err)
	}
}

func TestInspectLinkRequiresArtifactStore(t *testing.T) {
	if _, err := InspectLink(attestationRecord(), "sha256:"+strings.Repeat("a", 64), nil); err == nil || !strings.Contains(err.Error(), "artifact store is required") {
		t.Fatalf("missing-store error = %v", err)
	}
}
