package attestation

import (
	"fmt"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/custody"
	"ingen/lockwood/internal/store"
)

// Load reads one published attestation artifact, verifies its content digest,
// and decodes its canonical envelope. It does not verify the signature.
func Load(artifacts store.Store, digest string) (Envelope, error) {
	if artifacts == nil {
		return Envelope{}, fmt.Errorf("artifact store is required")
	}
	if err := artifact.ValidateDigest(digest); err != nil {
		return Envelope{}, err
	}
	data, err := artifacts.Get(digest)
	if err != nil {
		return Envelope{}, fmt.Errorf("load attestation artifact: %w", err)
	}
	actualDigest := artifact.DigestBytes(data)
	if actualDigest != digest {
		return Envelope{}, fmt.Errorf("attestation artifact digest mismatch: got %s, want %s", actualDigest, digest)
	}
	envelope, err := UnmarshalCanonical(data)
	if err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

// VerifyPublished loads a canonical attestation artifact and verifies it
// against a custody record using the caller-supplied key set. Key trust and
// authorization remain outside Lockwood.
func VerifyPublished(record custody.Record, digest string, artifacts store.Store, keys PublicKeySet) error {
	envelope, err := Load(artifacts, digest)
	if err != nil {
		return err
	}
	if err := VerifyWithKeySet(record, envelope, keys); err != nil {
		return fmt.Errorf("verify published attestation: %w", err)
	}
	return nil
}
