package ciresult

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/lockwood/internal/custody"
	"ingen/lockwood/internal/store"
)

const validCIResult = `{"schema":"ingen.ci-result/v1","tool":"sorna","kind":"behavioral-verification","status":"failed","exit_code":1,"created_at":"2026-09-15T12:00:00Z","source":{"root":"/workspace/service"},"report":{},"explanation":{}}`

func TestImporterPreservesProducerOwnedCIResult(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(t.TempDir(), "ci-result.json")
	if err := os.WriteFile(input, []byte(validCIResult), 0o600); err != nil {
		t.Fatal(err)
	}
	artifacts, err := store.NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	records, err := custody.NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	ingestor, err := custody.NewIngestor(artifacts, records)
	if err != nil {
		t.Fatal(err)
	}
	importer, err := NewImporter(ingestor)
	if err != nil {
		t.Fatal(err)
	}
	record, err := importer.Import(input, ImportRequest{CustodyID: "lockwood-ci-result-0001"})
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != custody.Accepted || record.Producer.Tool != "sorna" || record.Producer.Kind != "behavioral-verification" {
		t.Fatalf("custody record = %+v", record)
	}
	if record.Artifact.MediaType != MediaType || record.Source.Path != input {
		t.Fatalf("artifact/source = %+v / %+v", record.Artifact, record.Source)
	}
	stored, err := artifacts.Get(record.Artifact.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, []byte(validCIResult)) {
		t.Fatalf("stored CI result = %q, want original bytes", stored)
	}
	if _, err := custody.VerifyRecord(records, artifacts, record.CustodyID); err != nil {
		t.Fatal(err)
	}
}

func TestImporterRejectsInvalidCIResultBeforeCustody(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(t.TempDir(), "invalid-ci-result.json")
	if err := os.WriteFile(input, []byte(`{"schema":"ingen.ci-result/v1","tool":"sorna"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	artifacts, err := store.NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	records, err := custody.NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	ingestor, err := custody.NewIngestor(artifacts, records)
	if err != nil {
		t.Fatal(err)
	}
	importer, err := NewImporter(ingestor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := importer.Import(input, ImportRequest{CustodyID: "lockwood-ci-result-invalid"}); err == nil || !strings.Contains(err.Error(), "validate CI result") {
		t.Fatalf("Import error = %v, want validation failure", err)
	}
	if _, err := records.Get("lockwood-ci-result-invalid"); err == nil {
		t.Fatal("invalid CI result created a custody record")
	}
}

func TestImporterRejectsUnknownTopLevelCIResultFields(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(t.TempDir(), "unknown-ci-result.json")
	contents := strings.TrimSuffix(validCIResult, "}") + `,"unexpected":true}`
	if err := os.WriteFile(input, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	artifacts, err := store.NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	records, err := custody.NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	ingestor, err := custody.NewIngestor(artifacts, records)
	if err != nil {
		t.Fatal(err)
	}
	importer, err := NewImporter(ingestor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := importer.Import(input, ImportRequest{CustodyID: "lockwood-ci-result-unknown"}); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Import error = %v, want unknown-field rejection", err)
	}
}

func TestImporterRejectsOversizedCIResultBeforeCustody(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(t.TempDir(), "oversized-ci-result.json")
	contents := []byte(validCIResult)
	if err := os.WriteFile(input, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	artifacts, err := store.NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	records, err := custody.NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	ingestor, err := custody.NewIngestor(artifacts, records)
	if err != nil {
		t.Fatal(err)
	}
	importer, err := NewImporter(ingestor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := importer.Import(input, ImportRequest{
		CustodyID: "lockwood-ci-result-oversized",
		MaxBytes:  int64(len(contents) - 1),
	}); err == nil || !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Fatalf("Import error = %v, want maximum-size rejection", err)
	}
	if _, err := records.Get("lockwood-ci-result-oversized"); err == nil {
		t.Fatal("oversized CI result created a custody record")
	}
}
