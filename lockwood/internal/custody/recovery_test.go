package custody

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/lockwood/internal/store"
)

func TestIngestorRecoversPendingRecordWithoutRepublishingBlob(t *testing.T) {
	root := t.TempDir()
	artifacts, err := store.NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	records, err := NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	conflict := testRecord(t, "lockwood-recovery-conflict")
	if err := records.Put(conflict); err != nil {
		t.Fatal(err)
	}
	ingestor, err := NewIngestor(artifacts, records)
	if err != nil {
		t.Fatal(err)
	}

	_, err = ingestor.Accept(bytes.NewBufferString("recover me"), IntakeRequest{
		CustodyID: "lockwood-recovery-conflict",
		MediaType: "text/plain",
		Producer:  Producer{Tool: "example", Kind: "fixture"},
		Source:    Source{Path: "recover.txt"},
		Handling:  Handling{Redaction: "none", RetentionClass: "default"},
	})
	var intakeErr *IntakeError
	if !errors.As(err, &intakeErr) {
		t.Fatalf("error = %v, want IntakeError", err)
	}
	if intakeErr.Record.Artifact != intakeErr.Artifact {
		t.Fatalf("pending record artifact = %+v, error artifact = %+v", intakeErr.Record.Artifact, intakeErr.Artifact)
	}
	if err := os.Remove(filepath.Join(root, "records", "lockwood-recovery-conflict.json")); err != nil {
		t.Fatal(err)
	}
	recovered, err := ingestor.Recover(intakeErr.Record)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.CustodyID != intakeErr.Record.CustodyID || recovered.Artifact.Digest != intakeErr.Artifact.Digest {
		t.Fatalf("recovered record = %+v", recovered)
	}
	if _, err := VerifyRecord(records, artifacts, recovered.CustodyID); err != nil {
		t.Fatal(err)
	}
}

func TestIngestorRecoversWithMemoryBackends(t *testing.T) {
	artifacts := store.NewMemory()
	records := NewMemory()
	if err := records.Put(testRecord(t, "lockwood-memory-recovery-conflict")); err != nil {
		t.Fatal(err)
	}
	ingestor, err := NewIngestor(artifacts, records)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ingestor.Accept(bytes.NewBufferString("memory recovery"), IntakeRequest{
		CustodyID: "lockwood-memory-recovery-conflict",
		MediaType: "text/plain",
		Producer:  Producer{Tool: "example", Kind: "memory-recovery"},
		Source:    Source{Path: "memory-recovery.txt"},
		Handling:  Handling{Redaction: "none", RetentionClass: "default"},
	})
	var intakeErr *IntakeError
	if !errors.As(err, &intakeErr) {
		t.Fatalf("Accept error = %v, want IntakeError", err)
	}
	recoveredRecords := NewMemory()
	recovering, err := NewIngestor(artifacts, recoveredRecords)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recovering.Recover(intakeErr.Record); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyRecord(recoveredRecords, artifacts, intakeErr.Record.CustodyID); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileReportsOrphansDanglingReferencesAndCorruption(t *testing.T) {
	root := t.TempDir()
	artifacts, err := store.NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	records, err := NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}

	orphan, err := artifacts.Put(bytes.NewBufferString("orphan"), store.PutOptions{MediaType: "text/plain"})
	if err != nil {
		t.Fatal(err)
	}
	corrupt, err := artifacts.Put(bytes.NewBufferString("corrupt"), store.PutOptions{MediaType: "text/plain"})
	if err != nil {
		t.Fatal(err)
	}
	hexDigest := strings.TrimPrefix(corrupt.Digest, "sha256:")
	corruptPath := filepath.Join(root, "blobs", "sha256", hexDigest[:2], hexDigest[2:4], hexDigest)
	if err := os.WriteFile(corruptPath, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}

	dangling := testRecord(t, "lockwood-reconcile-dangling")
	dangling.Parents = nil
	if err := records.Put(dangling); err != nil {
		t.Fatal(err)
	}
	corruptRecord := testRecord(t, "lockwood-reconcile-corrupt")
	corruptRecord.Parents = nil
	corruptRecord.Artifact = corrupt
	if err := records.Put(corruptRecord); err != nil {
		t.Fatal(err)
	}

	report, err := Reconcile(artifacts, records)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Orphans) != 1 || report.Orphans[0].Digest != orphan.Digest {
		t.Fatalf("orphans = %+v, want %s", report.Orphans, orphan.Digest)
	}
	if len(report.DanglingReferences) != 2 {
		t.Fatalf("dangling references = %+v, want two", report.DanglingReferences)
	}
	if len(report.CorruptBlobs) != 1 || report.CorruptBlobs[0].Digest != corrupt.Digest {
		t.Fatalf("corrupt blobs = %+v, want %s", report.CorruptBlobs, corrupt.Digest)
	}
}
