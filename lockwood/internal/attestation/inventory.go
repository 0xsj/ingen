package attestation

import (
	"fmt"
	"sort"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/store"
)

// Query filters persisted detached attestation references. Empty fields match
// all values.
type Query struct {
	TargetDigest string
	KeyID        string
}

// StoredAttestation combines a verified artifact reference with its canonical
// envelope. It is inventory data, not evidence that the signature is trusted.
type StoredAttestation struct {
	Artifact artifact.Reference `json:"artifact"`
	Envelope Envelope           `json:"envelope"`
}

// Find returns persisted attestation envelopes matching query. The backend
// must expose reference inventory; each matching reference and blob is
// verified before it is returned.
func Find(artifacts store.Store, query Query) ([]StoredAttestation, error) {
	if artifacts == nil {
		return nil, fmt.Errorf("artifact store is required")
	}
	if query.TargetDigest != "" {
		if err := artifact.ValidateDigest(query.TargetDigest); err != nil {
			return nil, err
		}
	}
	lister, ok := artifacts.(store.ReferenceLister)
	if !ok {
		return nil, fmt.Errorf("artifact reference inventory is not supported")
	}
	references, err := lister.ListReferences()
	if err != nil {
		return nil, err
	}
	results := make([]StoredAttestation, 0)
	for _, reference := range references {
		if reference.MediaType != MediaType {
			continue
		}
		if err := artifacts.VerifyReference(reference); err != nil {
			return nil, fmt.Errorf("verify attestation reference %s: %w", reference.Digest, err)
		}
		envelope, err := Load(artifacts, reference.Digest)
		if err != nil {
			return nil, fmt.Errorf("load attestation reference %s: %w", reference.Digest, err)
		}
		if query.TargetDigest != "" && envelope.Target.Digest != query.TargetDigest {
			continue
		}
		if query.KeyID != "" && envelope.KeyID != query.KeyID {
			continue
		}
		results = append(results, StoredAttestation{Artifact: reference, Envelope: envelope})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Envelope.Target.Digest != results[j].Envelope.Target.Digest {
			return results[i].Envelope.Target.Digest < results[j].Envelope.Target.Digest
		}
		if results[i].Envelope.KeyID != results[j].Envelope.KeyID {
			return results[i].Envelope.KeyID < results[j].Envelope.KeyID
		}
		return results[i].Artifact.Digest < results[j].Artifact.Digest
	})
	return results, nil
}
