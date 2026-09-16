package attestation

import (
	"bytes"
	"crypto/ed25519"
	"strings"
	"testing"
)

func TestInspectRedactionProvenanceAndLink(t *testing.T) {
	artifacts, records, source, event, promoted := redactionProvenanceFixture(t)
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{64}, ed25519.SeedSize))
	envelope, err := SignRedactionProvenance(source, event, promoted, "provenance-key-inspection", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := PublishRedactionProvenance(source, event, promoted, envelope, artifacts)
	if err != nil {
		t.Fatal(err)
	}

	inspection, err := InspectRedactionProvenance(artifacts, publication.Artifact.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.AttestationDigest != publication.Artifact.Digest || inspection.Envelope.Target != envelope.Target || inspection.Envelope.KeyID != envelope.KeyID {
		t.Fatalf("provenance inspection = %+v", inspection)
	}
	link, err := InspectRedactionProvenanceLink(source, event, promoted, publication.Artifact.Digest, records, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if link.Relation != RedactionProvenanceLinkRelation || link.AttestationDigest != publication.Artifact.Digest || link.SourceCustodyID != source.CustodyID || link.EventID != event.EventID || link.PromotedCustodyID != promoted.CustodyID || link.SourceArtifactDigest != source.Artifact.Digest || link.PromotedArtifactDigest != promoted.Artifact.Digest {
		t.Fatalf("provenance link inspection = %+v", link)
	}

	changed := promoted
	changed.Source.Path = "changed-redacted.txt"
	if _, err := InspectRedactionProvenanceLink(source, event, changed, publication.Artifact.Digest, records, artifacts); err == nil || !strings.Contains(err.Error(), "target mismatch") {
		t.Fatalf("changed promoted record inspection error = %v", err)
	}
}

func TestInspectRedactionProvenanceDoesNotRequireSignerTrust(t *testing.T) {
	artifacts, _, source, event, promoted := redactionProvenanceFixture(t)
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{65}, ed25519.SeedSize))
	envelope, err := SignRedactionProvenance(source, event, promoted, "unregistered-provenance-key", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := PublishRedactionProvenance(source, event, promoted, envelope, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := InspectRedactionProvenance(artifacts, publication.Artifact.Digest); err != nil {
		t.Fatalf("inspection unexpectedly required trust: %v", err)
	}
}
