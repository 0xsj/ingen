package attestation

import (
	"fmt"

	"ingen/lockwood/internal/custody"
	"ingen/lockwood/internal/store"
)

const CustodyRecordLinkRelation = "attests-custody-record"

// LinkInspection is a typed, read-only relationship projection. It keeps the
// detached envelope digest, custody-record representation digest, and payload
// artifact digest distinct; it is not a custody lineage edge or a trust
// verdict.
type LinkInspection struct {
	Relation              string `json:"relation"`
	AttestationDigest     string `json:"attestation_digest"`
	CustodyRecordDigest   string `json:"custody_record_digest"`
	CustodyID             string `json:"custody_id"`
	PayloadArtifactDigest string `json:"payload_artifact_digest"`
}

// InspectLink verifies the canonical detached envelope and its binding to the
// supplied custody record. It also verifies the record's directly referenced
// payload artifact, but does not verify the envelope signature, trust, or
// reachable parent lineage.
func InspectLink(record custody.Record, attestationDigest string, artifacts store.Store) (LinkInspection, error) {
	if artifacts == nil {
		return LinkInspection{}, fmt.Errorf("artifact store is required")
	}
	envelope, err := Load(artifacts, attestationDigest)
	if err != nil {
		return LinkInspection{}, err
	}
	recordDigest, err := custody.CanonicalDigest(record)
	if err != nil {
		return LinkInspection{}, err
	}
	if envelope.Target.Digest != recordDigest {
		return LinkInspection{}, fmt.Errorf("attestation target digest mismatch: got %s, want %s", envelope.Target.Digest, recordDigest)
	}
	if err := artifacts.VerifyReference(record.Artifact); err != nil {
		return LinkInspection{}, fmt.Errorf("verify linked payload artifact: %w", err)
	}
	return LinkInspection{
		Relation:              CustodyRecordLinkRelation,
		AttestationDigest:     attestationDigest,
		CustodyRecordDigest:   recordDigest,
		CustodyID:             record.CustodyID,
		PayloadArtifactDigest: record.Artifact.Digest,
	}, nil
}
