package custody

import (
	"fmt"
	"time"

	"ingen/lockwood/internal/store"
)

// OrphanArtifact is a valid published blob that has no custody record. It is
// reported for review; reconciliation never deletes it automatically.
type OrphanArtifact struct {
	Digest     string    `json:"digest"`
	SizeBytes  int64     `json:"size_bytes"`
	ModifiedAt time.Time `json:"modified_at"`
}

type DanglingReference struct {
	CustodyID string `json:"custody_id"`
	Digest    string `json:"digest"`
	Error     string `json:"error"`
}

type CorruptBlob struct {
	Digest string `json:"digest"`
	Error  string `json:"error"`
}

type Reconciliation struct {
	Orphans            []OrphanArtifact    `json:"orphans"`
	DanglingReferences []DanglingReference `json:"dangling_references"`
	CorruptBlobs       []CorruptBlob       `json:"corrupt_blobs"`
}

// Recover verifies an already-published blob and appends its pending custody
// record. It is intended for retrying a record publication after Accept
// returned an IntakeError; it never republishes or replaces blob bytes.
func (i *Ingestor) Recover(record Record) (Record, error) {
	if i == nil || i.artifacts == nil {
		return Record{}, fmt.Errorf("artifact store is required")
	}
	if i.records == nil {
		return Record{}, fmt.Errorf("custody record store is required")
	}
	if err := record.Validate(); err != nil {
		return Record{}, fmt.Errorf("validate recovery record: %w", err)
	}
	if err := i.artifacts.VerifyReference(record.Artifact); err != nil {
		return Record{}, fmt.Errorf("verify recovery artifact: %w", err)
	}
	if err := i.records.Put(record); err != nil {
		return Record{}, fmt.Errorf("recover custody record: %w", err)
	}
	return record, nil
}

// Reconcile produces a read-only view of the relationship between published
// blobs and custody records. Structural record or store inventory failures are
// returned; individual blob/reference integrity failures are reported.
func Reconcile(artifacts *store.Filesystem, records *Filesystem) (Reconciliation, error) {
	report := Reconciliation{
		Orphans:            make([]OrphanArtifact, 0),
		DanglingReferences: make([]DanglingReference, 0),
		CorruptBlobs:       make([]CorruptBlob, 0),
	}
	if artifacts == nil {
		return report, fmt.Errorf("artifact store is required")
	}
	if records == nil {
		return report, fmt.Errorf("custody record store is required")
	}
	allRecords, err := records.List()
	if err != nil {
		return report, err
	}
	referenced := make(map[string]bool, len(allRecords))
	for _, record := range allRecords {
		referenced[record.Artifact.Digest] = true
		if err := artifacts.VerifyReference(record.Artifact); err != nil {
			report.DanglingReferences = append(report.DanglingReferences, DanglingReference{
				CustodyID: record.CustodyID,
				Digest:    record.Artifact.Digest,
				Error:     err.Error(),
			})
		}
	}

	blobs, err := artifacts.ListBlobs()
	if err != nil {
		return report, err
	}
	for _, blob := range blobs {
		if err := artifacts.Verify(blob.Digest); err != nil {
			report.CorruptBlobs = append(report.CorruptBlobs, CorruptBlob{
				Digest: blob.Digest,
				Error:  err.Error(),
			})
			continue
		}
		if !referenced[blob.Digest] {
			report.Orphans = append(report.Orphans, OrphanArtifact{
				Digest:     blob.Digest,
				SizeBytes:  blob.SizeBytes,
				ModifiedAt: blob.ModifiedAt,
			})
		}
	}
	return report, nil
}
