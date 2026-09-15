package custody

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"ingen/lockwood/internal/store"
)

func TestIngestorAcceptsArtifactAndPublishesMatchingRecord(t *testing.T) {
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

	receivedAt := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	record, err := ingestor.Accept(bytes.NewBufferString(`{"schema":"ingen.ci-result/v1","status":"passed"}`), IntakeRequest{
		CustodyID:   "lockwood-intake-0001",
		MediaType:   "application/json",
		LogicalName: "ci-result.json",
		ReceivedAt:  receivedAt,
		Producer:    Producer{Tool: "sorna", Kind: "behavioral-verification"},
		Source:      Source{RunID: "run-0001", Path: "ci-result.json"},
		Handling:    Handling{Redaction: "none", RetentionClass: "default"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != Accepted || record.Artifact.SizeBytes == 0 {
		t.Fatalf("record = %+v", record)
	}
	if record.Parents == nil {
		t.Fatal("accepted record has nil parents; want an empty array")
	}
	if err := artifacts.Verify(record.Artifact.Digest); err != nil {
		t.Fatal(err)
	}
	loaded, err := records.Get(record.CustodyID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Artifact.Digest != record.Artifact.Digest || loaded.Artifact.SizeBytes != record.Artifact.SizeBytes {
		t.Fatalf("loaded record = %+v, want %+v", loaded, record)
	}
}

func TestIngestorRejectsExpectedDigestMismatchBeforeRecordPublication(t *testing.T) {
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

	_, err = ingestor.Accept(strings.NewReader("data"), IntakeRequest{
		CustodyID:      "lockwood-intake-mismatch",
		ExpectedDigest: "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		MediaType:      "text/plain",
		Producer:       Producer{Tool: "example", Kind: "fixture"},
		Source:         Source{Path: "data.txt"},
		Handling:       Handling{Redaction: "none", RetentionClass: "default"},
	})
	if err == nil {
		t.Fatal("Accept succeeded with mismatched expected digest")
	}
	if _, err := records.Get("lockwood-intake-mismatch"); err == nil {
		t.Fatal("failed intake published a custody record")
	}
}

func TestIngestorReportsOrphanWhenRecordPublicationFails(t *testing.T) {
	root := t.TempDir()
	artifacts, err := store.NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	records, err := NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := records.Put(testRecord(t, "lockwood-intake-conflict")); err != nil {
		t.Fatal(err)
	}
	ingestor, err := NewIngestor(artifacts, records)
	if err != nil {
		t.Fatal(err)
	}

	_, err = ingestor.Accept(strings.NewReader("new bytes"), IntakeRequest{
		CustodyID: "lockwood-intake-conflict",
		MediaType: "text/plain",
		Producer:  Producer{Tool: "example", Kind: "fixture"},
		Source:    Source{Path: "new.txt"},
		Handling:  Handling{Redaction: "none", RetentionClass: "default"},
	})
	var intakeErr *IntakeError
	if !errors.As(err, &intakeErr) {
		t.Fatalf("error = %v, want IntakeError", err)
	}
	if intakeErr.Artifact.Digest == "" {
		t.Fatal("IntakeError did not preserve orphan artifact reference")
	}
}
