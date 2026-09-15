package custody

import "encoding/json"

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

func normalizeRecord(record Record) Record {
	if record.Parents == nil {
		record.Parents = []Lineage{}
	}
	return record
}
