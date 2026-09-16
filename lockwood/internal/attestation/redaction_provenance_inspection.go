package attestation

import (
	"fmt"

	"ingen/lockwood/internal/custody"
	"ingen/lockwood/internal/store"
)

const RedactionProvenanceLinkRelation = "attests-redaction-promotion"

// RedactionProvenanceInspection is read-only metadata for a known detached
// provenance envelope. Loading it verifies content identity and canonical
// structure but does not verify the signature or establish signer trust.
type RedactionProvenanceInspection struct {
	AttestationDigest string                      `json:"attestation_digest"`
	Envelope          RedactionProvenanceEnvelope `json:"envelope"`
}

// RedactionProvenanceLinkInspection is a typed relationship projection. It
// keeps the detached envelope, records, event, and payload identities distinct
// and is not a signature or authorization verdict.
type RedactionProvenanceLinkInspection struct {
	Relation               string `json:"relation"`
	AttestationDigest      string `json:"attestation_digest"`
	SourceCustodyID        string `json:"source_custody_id"`
	SourceRecordDigest     string `json:"source_record_digest"`
	SourceArtifactDigest   string `json:"source_artifact_digest"`
	EventID                string `json:"event_id"`
	EventDigest            string `json:"event_digest"`
	OriginalDigest         string `json:"original_digest"`
	ResultingDigest        string `json:"resulting_digest"`
	PromotedCustodyID      string `json:"promoted_custody_id"`
	PromotedRecordDigest   string `json:"promoted_record_digest"`
	PromotedArtifactDigest string `json:"promoted_artifact_digest"`
}

// InspectRedactionProvenance loads one published provenance envelope by
// content digest without performing cryptographic verification.
func InspectRedactionProvenance(artifacts store.Store, digest string) (RedactionProvenanceInspection, error) {
	envelope, err := LoadRedactionProvenance(artifacts, digest)
	if err != nil {
		return RedactionProvenanceInspection{}, err
	}
	return RedactionProvenanceInspection{AttestationDigest: digest, Envelope: envelope}, nil
}

// InspectRedactionProvenanceLink checks a known envelope against the supplied
// source record, event, and promoted record. If records is supplied, both
// records and the promoted record's reachable lineage are verified as well.
// It never verifies the envelope signature or resolves signer trust.
func InspectRedactionProvenanceLink(source custody.Record, event custody.HandlingEvent, promoted custody.Record, digest string, records custody.RecordStore, artifacts store.Store) (RedactionProvenanceLinkInspection, error) {
	envelope, err := LoadRedactionProvenance(artifacts, digest)
	if err != nil {
		return RedactionProvenanceLinkInspection{}, err
	}
	target, err := redactionProvenanceTarget(source, event, promoted)
	if err != nil {
		return RedactionProvenanceLinkInspection{}, err
	}
	if envelope.Target != target {
		return RedactionProvenanceLinkInspection{}, fmt.Errorf("redaction provenance target mismatch")
	}
	if records != nil {
		if _, err := custody.VerifyRecord(records, artifacts, source.CustodyID); err != nil {
			return RedactionProvenanceLinkInspection{}, fmt.Errorf("verify redaction provenance source custody record: %w", err)
		}
	}
	if err := verifyRedactionProvenanceRecords(source, event, promoted, records, artifacts); err != nil {
		return RedactionProvenanceLinkInspection{}, err
	}
	return RedactionProvenanceLinkInspection{
		Relation:               RedactionProvenanceLinkRelation,
		AttestationDigest:      digest,
		SourceCustodyID:        target.SourceCustodyID,
		SourceRecordDigest:     target.SourceRecordDigest,
		SourceArtifactDigest:   source.Artifact.Digest,
		EventID:                target.EventID,
		EventDigest:            target.EventDigest,
		OriginalDigest:         target.OriginalDigest,
		ResultingDigest:        target.ResultingDigest,
		PromotedCustodyID:      target.PromotedCustodyID,
		PromotedRecordDigest:   target.PromotedRecordDigest,
		PromotedArtifactDigest: promoted.Artifact.Digest,
	}, nil
}
