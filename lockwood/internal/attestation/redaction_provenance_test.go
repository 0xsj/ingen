package attestation

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"ingen/lockwood/internal/custody"
	"ingen/lockwood/internal/store"
)

func redactionProvenanceFixture(t *testing.T) (store.Store, custody.RecordStore, custody.Record, custody.HandlingEvent, custody.Record) {
	t.Helper()
	artifacts := store.NewMemory()
	records := custody.NewMemory()
	original, err := artifacts.Put(strings.NewReader("provenance original bytes"), store.PutOptions{MediaType: "text/plain", LogicalName: "original.txt"})
	if err != nil {
		t.Fatal(err)
	}
	resulting, err := artifacts.Put(strings.NewReader("provenance redacted bytes"), store.PutOptions{MediaType: "text/plain", LogicalName: "redacted.txt"})
	if err != nil {
		t.Fatal(err)
	}
	verifiedAt := time.Date(2026, 9, 15, 12, 0, 1, 0, time.UTC)
	source := custody.Record{
		Schema:     custody.SchemaV1,
		CustodyID:  "lockwood-redaction-provenance-source",
		Status:     custody.Accepted,
		Artifact:   original,
		ReceivedAt: time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC),
		Producer:   custody.Producer{Tool: "example", Kind: "source"},
		Custodian:  custody.Custodian{Tool: "lockwood"},
		Source:     custody.Source{Path: "original.txt"},
		Integrity:  custody.Integrity{Status: custody.IntegrityVerified, Method: "sha256", VerifiedAt: &verifiedAt},
		Parents:    []custody.Lineage{},
		Handling:   custody.Handling{Redaction: "none", RetentionClass: "default"},
	}
	if err := records.Put(source); err != nil {
		t.Fatal(err)
	}
	event := custody.HandlingEvent{
		Schema:          custody.HandlingEventSchema,
		EventID:         "event-redaction-provenance-0001",
		CustodyID:       source.CustodyID,
		Type:            custody.RedactionEvent,
		RecordedAt:      time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC),
		Actor:           "operator@example",
		Reason:          "removed restricted fields",
		OriginalDigest:  original.Digest,
		ResultingDigest: resulting.Digest,
	}
	if _, err := custody.RegisterRedaction(artifacts, records, records, event); err != nil {
		t.Fatal(err)
	}
	promoted, err := custody.PromoteRedactionResult(artifacts, records, records, source.CustodyID, event.EventID, custody.RedactionPromotionRequest{
		CustodyID:  "lockwood-redaction-provenance-result",
		Artifact:   resulting,
		ReceivedAt: time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC),
		Producer:   custody.Producer{Tool: "redactor", Kind: "sanitized-export"},
		Source:     custody.Source{Path: "redacted.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return artifacts, records, source, event, promoted
}

func TestRedactionProvenanceSignsAndVerifiesPromotion(t *testing.T) {
	artifacts, records, source, event, promoted := redactionProvenanceFixture(t)
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{57}, ed25519.SeedSize))
	envelope, err := SignRedactionProvenance(source, event, promoted, "provenance-key-2026-01", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := MarshalCanonicalRedactionProvenance(envelope)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := UnmarshalCanonicalRedactionProvenance(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyRedactionProvenance(source, event, promoted, decoded, privateKey.Public().(ed25519.PublicKey)); err != nil {
		t.Fatal(err)
	}
	publication, err := PublishRedactionProvenance(source, event, promoted, envelope, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if publication.Artifact.MediaType != RedactionProvenanceMediaType || publication.SourceCustodyID != source.CustodyID || publication.PromotedCustodyID != promoted.CustodyID {
		t.Fatalf("publication = %+v", publication)
	}
	receipt, err := VerifyPublishedRedactionProvenanceReceipt(source, event, promoted, publication.Artifact.Digest, records, artifacts, privateKey.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Verified || receipt.SourceCustodyID != source.CustodyID || receipt.EventID != event.EventID || receipt.PromotedCustodyID != promoted.CustodyID || receipt.AttestationDigest != publication.Artifact.Digest {
		t.Fatalf("verification receipt = %+v", receipt)
	}
	registry := TrustRegistry{
		Schema: TrustRegistrySchema,
		Keys: []TrustedKey{{
			KeyID:     envelope.KeyID,
			Algorithm: Algorithm,
			PublicKey: base64.StdEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey)),
			Status:    KeyActive,
		}},
	}
	trusted, err := VerifyPublishedRedactionProvenanceWithRegistryReceipt(source, event, promoted, publication.Artifact.Digest, records, artifacts, registry, time.Date(2026, 9, 17, 13, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if !trusted.Verified || !trusted.Trusted || trusted.RegistryDigest == "" || trusted.EvaluatedAt != "2026-09-17T13:00:00Z" {
		t.Fatalf("trusted verification receipt = %+v", trusted)
	}

	changed := promoted
	changed.Source.Path = "different-redacted.txt"
	if err := VerifyRedactionProvenance(source, event, changed, envelope, privateKey.Public().(ed25519.PublicKey)); err == nil || !strings.Contains(err.Error(), "target mismatch") {
		t.Fatalf("changed promoted record verification error = %v", err)
	}
}

func TestRedactionProvenanceRejectsInvalidRelationship(t *testing.T) {
	_, _, source, event, promoted := redactionProvenanceFixture(t)
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{58}, ed25519.SeedSize))
	changed := promoted
	changed.Parents = []custody.Lineage{}
	if _, err := SignRedactionProvenance(source, event, changed, "provenance-key-2026-02", privateKey); err == nil || !strings.Contains(err.Error(), "lacks derived-from") {
		t.Fatalf("invalid relationship error = %v", err)
	}
}
