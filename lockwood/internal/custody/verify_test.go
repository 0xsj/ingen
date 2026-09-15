package custody

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"ingen/lockwood/internal/store"
)

func TestVerifyRecordBindsRecordToStoredBlob(t *testing.T) {
	root := t.TempDir()
	artifacts, err := store.NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	records, err := NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	ingestor, err := NewIngestor(artifacts, records)
	if err != nil {
		t.Fatal(err)
	}
	record, err := ingestor.Accept(bytes.NewBufferString("verified bytes"), IntakeRequest{
		CustodyID: "lockwood-verify-0001",
		MediaType: "text/plain",
		Producer:  Producer{Tool: "example", Kind: "fixture"},
		Source:    Source{Path: "verified.txt"},
		Handling:  Handling{Redaction: "none", RetentionClass: "default"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyRecord(records, artifacts, record.CustodyID); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyRecordRejectsMissingOrMismatchedBlob(t *testing.T) {
	root := t.TempDir()
	artifacts, err := store.NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	records, err := NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	record := testRecord(t, "lockwood-verify-mismatch")
	if err := records.Put(record); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyRecord(records, artifacts, record.CustodyID); err == nil {
		t.Fatal("VerifyRecord accepted a missing blob")
	}

	sizeRecord := testRecord(t, "lockwood-verify-size")
	ref, err := artifacts.Put(bytes.NewBufferString("wrong bytes"), store.PutOptions{MediaType: "application/json"})
	if err != nil {
		t.Fatal(err)
	}
	sizeRecord.Artifact = ref
	if err := records.Put(sizeRecord); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(root + "/records/" + sizeRecord.CustodyID + ".json")
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte(`"size_bytes":11`), []byte(`"size_bytes":12`), 1)
	if err := os.WriteFile(root+"/records/"+sizeRecord.CustodyID+".json", data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyRecord(records, artifacts, sizeRecord.CustodyID); err == nil || !strings.Contains(err.Error(), "artifact size mismatch") {
		t.Fatalf("VerifyRecord error = %v, want size mismatch", err)
	}
}
