package attestation

import (
	"bytes"
	"fmt"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/custody"
	"ingen/lockwood/internal/store"
)

const (
	// MediaType identifies canonical detached Lockwood attestation bytes.
	MediaType = "application/vnd.ingen.lockwood.attestation+json"
	// DefaultLogicalName is descriptive metadata only; the artifact digest is
	// the identity of the published envelope.
	DefaultLogicalName = "attestation.json"
)

// Publication is an operator-facing receipt for a detached envelope. The
// custody ID is contextual metadata; the signed target identity is the
// envelope's target digest.
type Publication struct {
	CustodyID string             `json:"custody_id,omitempty"`
	Artifact  artifact.Reference `json:"artifact"`
	Envelope  Envelope           `json:"envelope"`
}

// Publish stores one canonical attestation envelope as an immutable artifact.
// It does not create or mutate a custody record and does not establish trust
// in the signing key.
func Publish(envelope Envelope, artifacts store.Store) (artifact.Reference, error) {
	if artifacts == nil {
		return artifact.Reference{}, fmt.Errorf("artifact store is required")
	}
	encoded, err := MarshalCanonical(envelope)
	if err != nil {
		return artifact.Reference{}, err
	}
	return artifacts.Put(bytes.NewReader(encoded), store.PutOptions{
		ExpectedDigest: artifact.DigestBytes(encoded),
		MediaType:      MediaType,
		LogicalName:    DefaultLogicalName,
	})
}

// PublishForRecord verifies that an envelope targets the exact canonical
// representation of record before publishing it. It does not verify the
// signature or establish trust in its key.
func PublishForRecord(record custody.Record, envelope Envelope, artifacts store.Store) (Publication, error) {
	digest, err := custody.CanonicalDigest(record)
	if err != nil {
		return Publication{}, err
	}
	if envelope.Target.Digest != digest {
		return Publication{}, fmt.Errorf("attestation target digest mismatch: got %s, want %s", envelope.Target.Digest, digest)
	}
	ref, err := Publish(envelope, artifacts)
	if err != nil {
		return Publication{}, err
	}
	return Publication{CustodyID: record.CustodyID, Artifact: ref, Envelope: envelope}, nil
}
