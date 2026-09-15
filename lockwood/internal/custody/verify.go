package custody

import (
	"fmt"

	"ingen/lockwood/internal/store"
)

func VerifyRecord(records *Filesystem, artifacts *store.Filesystem, custodyID string) (Record, error) {
	if records == nil {
		return Record{}, fmt.Errorf("custody record store is required")
	}
	if artifacts == nil {
		return Record{}, fmt.Errorf("artifact store is required")
	}
	record, err := records.Get(custodyID)
	if err != nil {
		return Record{}, err
	}
	if err := artifacts.VerifyReference(record.Artifact); err != nil {
		return Record{}, fmt.Errorf("verify custody record %q: %w", custodyID, err)
	}
	if err := VerifyLineage(records, artifacts, record); err != nil {
		return Record{}, fmt.Errorf("verify custody record %q lineage: %w", custodyID, err)
	}
	return record, nil
}
