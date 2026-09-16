package attestation

import (
	"fmt"
	"sort"
	"time"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/custody"
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

// TrustedStoredAttestation combines an attestation inventory result with the
// transient evidence that its target record, lineage, signature, and trust
// registry resolution all verified successfully.
type TrustedStoredAttestation struct {
	Artifact     artifact.Reference         `json:"artifact"`
	Envelope     Envelope                   `json:"envelope"`
	Verification TrustedVerificationReceipt `json:"verification"`
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

// FindTrusted returns only persisted attestations whose target custody record
// can be resolved and fully verified through the supplied trust registry. The
// operation is read-only and fails closed on a missing target, damaged record,
// unresolved lineage, invalid signature, or untrusted key.
func FindTrusted(records custody.RecordStore, artifacts store.Store, query Query, registry TrustRegistry, evaluatedAt time.Time) ([]TrustedStoredAttestation, error) {
	if records == nil {
		return nil, fmt.Errorf("custody record store is required")
	}
	stored, err := Find(artifacts, query)
	if err != nil {
		return nil, err
	}
	allRecords, err := records.List()
	if err != nil {
		return nil, err
	}
	byDigest := make(map[string]custody.Record, len(allRecords))
	for _, record := range allRecords {
		digest, err := custody.CanonicalDigest(record)
		if err != nil {
			return nil, fmt.Errorf("digest custody record %q: %w", record.CustodyID, err)
		}
		byDigest[digest] = record
	}
	results := make([]TrustedStoredAttestation, 0, len(stored))
	for _, item := range stored {
		record, ok := byDigest[item.Envelope.Target.Digest]
		if !ok {
			return nil, fmt.Errorf("attestation target custody record %s was not found", item.Envelope.Target.Digest)
		}
		verifiedRecord, err := custody.VerifyRecord(records, artifacts, record.CustodyID)
		if err != nil {
			return nil, fmt.Errorf("verify attestation target custody record %q: %w", record.CustodyID, err)
		}
		receipt, err := VerifyPublishedWithRegistryReceipt(verifiedRecord, item.Artifact.Digest, artifacts, registry, evaluatedAt)
		if err != nil {
			return nil, fmt.Errorf("verify attestation %s: %w", item.Artifact.Digest, err)
		}
		results = append(results, TrustedStoredAttestation{
			Artifact:     item.Artifact,
			Envelope:     item.Envelope,
			Verification: receipt,
		})
	}
	return results, nil
}
