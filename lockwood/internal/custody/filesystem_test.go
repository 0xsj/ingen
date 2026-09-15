package custody

import (
	"os"
	"strings"
	"testing"
	"time"

	"ingen/lockwood/internal/artifact"
)

func testRecord(t *testing.T, custodyID string) Record {
	t.Helper()
	verifiedAt := time.Date(2026, 9, 15, 12, 0, 1, 0, time.UTC)
	return Record{
		Schema:    Schema,
		CustodyID: custodyID,
		Status:    Accepted,
		Artifact: artifact.Reference{
			Schema:      artifact.Schema,
			Digest:      "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			SizeBytes:   12,
			MediaType:   "application/json",
			LogicalName: "result.json",
		},
		ReceivedAt: time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC),
		Producer:   Producer{Tool: "sorna", Kind: "behavioral-verification", Version: "alpha"},
		Custodian:  Custodian{Tool: "lockwood", Version: "draft"},
		Source:     Source{RunID: "run-0001", Path: "result.json"},
		Integrity:  Integrity{Status: IntegrityVerified, Method: "sha256", VerifiedAt: &verifiedAt},
		Parents: []Lineage{{
			Relation: References,
			Digest:   "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		}},
		Handling: Handling{Redaction: "none", RetentionClass: "default"},
	}
}

func TestFilesystemPutGetAndIdempotency(t *testing.T) {
	store, err := NewFilesystem(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record := testRecord(t, "lockwood-record-0001")

	if err := store.Put(record); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(record); err != nil {
		t.Fatalf("identical record was not idempotent: %v", err)
	}
	got, err := store.Get(record.CustodyID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CustodyID != record.CustodyID || got.Artifact.Digest != record.Artifact.Digest {
		t.Fatalf("loaded record = %+v, want %+v", got, record)
	}
}

func TestFilesystemRejectsConflictingRecordID(t *testing.T) {
	store, err := NewFilesystem(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first := testRecord(t, "lockwood-record-conflict")
	if err := store.Put(first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.Source.Path = "different.json"
	if err := store.Put(second); err == nil || !strings.Contains(err.Error(), "different contents") {
		t.Fatalf("conflicting Put error = %v", err)
	}
}

func TestFilesystemRejectsUnsafeIDsAndInvalidRecords(t *testing.T) {
	store, err := NewFilesystem(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	unsafe := testRecord(t, "../escape")
	if err := store.Put(unsafe); err == nil {
		t.Fatal("Put accepted an unsafe custody id")
	}
	if _, err := store.Get("../escape"); err == nil {
		t.Fatal("Get accepted an unsafe custody id")
	}

	invalid := testRecord(t, "lockwood-record-invalid")
	invalid.Status = Accepted
	invalid.Integrity.Status = IntegrityNotChecked
	invalid.Integrity.VerifiedAt = nil
	if err := store.Put(invalid); err == nil || !strings.Contains(err.Error(), "verified integrity") {
		t.Fatalf("invalid record error = %v", err)
	}
}

func TestFilesystemRejectsUnknownRecordFields(t *testing.T) {
	root := t.TempDir()
	store, err := NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	record := testRecord(t, "lockwood-record-unknown")
	if err := store.Put(record); err != nil {
		t.Fatal(err)
	}
	path := root + "/records/" + record.CustodyID + ".json"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data[:len(data)-1], []byte(`,"unexpected":true}`)...)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(record.CustodyID); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown-field Get error = %v", err)
	}
}
