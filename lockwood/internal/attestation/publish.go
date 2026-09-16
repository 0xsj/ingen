package attestation

import (
	"bytes"
	"fmt"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/store"
)

const (
	// MediaType identifies canonical detached Lockwood attestation bytes.
	MediaType = "application/vnd.ingen.lockwood.attestation+json"
	// DefaultLogicalName is descriptive metadata only; the artifact digest is
	// the identity of the published envelope.
	DefaultLogicalName = "attestation.json"
)

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
