package attestation

import (
	"fmt"
	"sort"
	"time"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/custody"
	"ingen/lockwood/internal/store"
)

// RedactionProvenanceQuery filters persisted redaction-provenance references.
// Empty fields match all values.
type RedactionProvenanceQuery struct {
	SourceCustodyID   string
	EventID           string
	PromotedCustodyID string
	KeyID             string
}

// StoredRedactionProvenance combines a verified artifact reference with its
// canonical provenance envelope. It is inventory data, not trusted evidence.
type StoredRedactionProvenance struct {
	Artifact artifact.Reference          `json:"artifact"`
	Envelope RedactionProvenanceEnvelope `json:"envelope"`
}

// TrustedStoredRedactionProvenance combines inventory data with a transient
// receipt proving the relationship and signing key verified successfully.
type TrustedStoredRedactionProvenance struct {
	Artifact     artifact.Reference                            `json:"artifact"`
	Envelope     RedactionProvenanceEnvelope                   `json:"envelope"`
	Verification TrustedRedactionProvenanceVerificationReceipt `json:"verification"`
}

// FindRedactionProvenance returns persisted redaction-provenance envelopes
// matching query. The backend must expose reference inventory; each matching
// reference and canonical envelope is verified before it is returned.
func FindRedactionProvenance(artifacts store.Store, query RedactionProvenanceQuery) ([]StoredRedactionProvenance, error) {
	if artifacts == nil {
		return nil, fmt.Errorf("artifact store is required")
	}
	if err := validateRedactionProvenanceQuery(query); err != nil {
		return nil, err
	}
	lister, ok := artifacts.(store.ReferenceLister)
	if !ok {
		return nil, fmt.Errorf("artifact reference inventory is not supported")
	}
	references, err := lister.ListReferences()
	if err != nil {
		return nil, err
	}
	results := make([]StoredRedactionProvenance, 0)
	for _, reference := range references {
		if reference.MediaType != RedactionProvenanceMediaType {
			continue
		}
		if err := artifacts.VerifyReference(reference); err != nil {
			return nil, fmt.Errorf("verify redaction provenance reference %s: %w", reference.Digest, err)
		}
		envelope, err := LoadRedactionProvenance(artifacts, reference.Digest)
		if err != nil {
			return nil, fmt.Errorf("load redaction provenance reference %s: %w", reference.Digest, err)
		}
		if !matchesRedactionProvenanceQuery(envelope, query) {
			continue
		}
		results = append(results, StoredRedactionProvenance{Artifact: reference, Envelope: envelope})
	}
	sort.Slice(results, func(i, j int) bool {
		left, right := results[i].Envelope.Target, results[j].Envelope.Target
		if left.SourceCustodyID != right.SourceCustodyID {
			return left.SourceCustodyID < right.SourceCustodyID
		}
		if left.EventID != right.EventID {
			return left.EventID < right.EventID
		}
		if left.PromotedCustodyID != right.PromotedCustodyID {
			return left.PromotedCustodyID < right.PromotedCustodyID
		}
		if results[i].Envelope.KeyID != results[j].Envelope.KeyID {
			return results[i].Envelope.KeyID < results[j].Envelope.KeyID
		}
		return results[i].Artifact.Digest < results[j].Artifact.Digest
	})
	return results, nil
}

// FindTrustedRedactionProvenance returns only persisted envelopes whose source
// record, event, promoted record, payload references, signature, and signing
// key all verify. It is read-only and fails closed on any missing or damaged
// relationship.
func FindTrustedRedactionProvenance(records custody.RecordStore, events custody.HandlingEventStore, artifacts store.Store, query RedactionProvenanceQuery, registry TrustRegistry, evaluatedAt time.Time) ([]TrustedStoredRedactionProvenance, error) {
	if records == nil {
		return nil, fmt.Errorf("custody record store is required")
	}
	if events == nil {
		return nil, fmt.Errorf("handling event store is required")
	}
	stored, err := FindRedactionProvenance(artifacts, query)
	if err != nil {
		return nil, err
	}
	results := make([]TrustedStoredRedactionProvenance, 0, len(stored))
	for _, item := range stored {
		target := item.Envelope.Target
		source, err := custody.VerifyRecord(records, artifacts, target.SourceCustodyID)
		if err != nil {
			return nil, fmt.Errorf("verify redaction provenance source custody record %q: %w", target.SourceCustodyID, err)
		}
		event, err := events.GetEvent(target.SourceCustodyID, target.EventID)
		if err != nil {
			return nil, fmt.Errorf("read redaction provenance event %q: %w", target.EventID, err)
		}
		promoted, err := custody.VerifyRecord(records, artifacts, target.PromotedCustodyID)
		if err != nil {
			return nil, fmt.Errorf("verify redaction provenance promoted custody record %q: %w", target.PromotedCustodyID, err)
		}
		receipt, err := VerifyPublishedRedactionProvenanceWithRegistryReceipt(source, event, promoted, item.Artifact.Digest, records, artifacts, registry, evaluatedAt)
		if err != nil {
			return nil, fmt.Errorf("verify redaction provenance %s: %w", item.Artifact.Digest, err)
		}
		results = append(results, TrustedStoredRedactionProvenance{
			Artifact:     item.Artifact,
			Envelope:     item.Envelope,
			Verification: receipt,
		})
	}
	return results, nil
}

func validateRedactionProvenanceQuery(query RedactionProvenanceQuery) error {
	if query.SourceCustodyID != "" && !handlingCustodyIDPattern.MatchString(query.SourceCustodyID) {
		return fmt.Errorf("invalid redaction provenance source custody id %q", query.SourceCustodyID)
	}
	if query.EventID != "" && !handlingEventIDPattern.MatchString(query.EventID) {
		return fmt.Errorf("invalid redaction provenance event id %q", query.EventID)
	}
	if query.PromotedCustodyID != "" && !handlingCustodyIDPattern.MatchString(query.PromotedCustodyID) {
		return fmt.Errorf("invalid redaction provenance promoted custody id %q", query.PromotedCustodyID)
	}
	if query.KeyID != "" && !keyIDPattern.MatchString(query.KeyID) {
		return fmt.Errorf("invalid redaction provenance key id %q", query.KeyID)
	}
	return nil
}

func matchesRedactionProvenanceQuery(envelope RedactionProvenanceEnvelope, query RedactionProvenanceQuery) bool {
	target := envelope.Target
	return (query.SourceCustodyID == "" || target.SourceCustodyID == query.SourceCustodyID) &&
		(query.EventID == "" || target.EventID == query.EventID) &&
		(query.PromotedCustodyID == "" || target.PromotedCustodyID == query.PromotedCustodyID) &&
		(query.KeyID == "" || envelope.KeyID == query.KeyID)
}
