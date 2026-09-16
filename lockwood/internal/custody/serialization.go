package custody

import (
	"encoding/json"

	"ingen/lockwood/internal/artifact"
)

// MarshalCanonical returns Lockwood's v1 custody-record encoding: compact JSON
// with the declared struct field order, normalized empty parents, and no
// trailing newline. This is a stable local storage encoding, not yet a claim
// of compatibility with an external signing canonicalization standard.
func MarshalCanonical(record Record) ([]byte, error) {
	record = normalizeRecord(record)
	if err := record.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(record)
}

// CanonicalDigest returns the SHA-256 digest of the canonical custody-record
// bytes. It identifies the record representation only; it is not a signature,
// attestation, or proof of the producer's claims.
func CanonicalDigest(record Record) (string, error) {
	encoded, err := MarshalCanonical(record)
	if err != nil {
		return "", err
	}
	return artifact.DigestBytes(encoded), nil
}

func normalizeRecord(record Record) Record {
	if record.Parents == nil {
		record.Parents = []Lineage{}
	}
	return record
}
