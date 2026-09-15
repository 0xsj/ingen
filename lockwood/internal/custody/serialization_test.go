package custody

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMarshalCanonicalUsesStableEncodingAndArrayParents(t *testing.T) {
	record := testRecord(t, "lockwood-canonical")
	record.Parents = nil
	encoded, err := MarshalCanonical(record)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"schema":"lockwood.custody/v1","custody_id":"lockwood-canonical","status":"accepted","artifact":{"schema":"lockwood.artifact/v1","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","size_bytes":12,"media_type":"application/json","logical_name":"result.json"},"received_at":"2026-09-15T12:00:00Z","producer":{"tool":"sorna","kind":"behavioral-verification","version":"alpha"},"custodian":{"tool":"lockwood","version":"draft"},"source":{"run_id":"run-0001","path":"result.json"},"integrity":{"status":"verified","method":"sha256","verified_at":"2026-09-15T12:00:01Z"},"parents":[],"handling":{"redaction":"none","retention_class":"default"}}`
	if string(encoded) != want {
		t.Fatalf("canonical record = %s, want %s", encoded, want)
	}
	if bytes.Contains(encoded, []byte("\n")) || bytes.Contains(encoded, []byte("  ")) {
		t.Fatalf("canonical record contains formatting whitespace: %s", encoded)
	}
}

func TestFilesystemRejectsNonCanonicalRecord(t *testing.T) {
	root := t.TempDir()
	store, err := NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	record := testRecord(t, "lockwood-noncanonical")
	record.Parents = []Lineage{}
	if err := store.Put(record); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "records", record.CustodyID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, data, "", "  "); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, formatted.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(record.CustodyID); err == nil || !strings.Contains(err.Error(), "not canonical JSON") {
		t.Fatalf("Get error = %v, want non-canonical JSON", err)
	}
}
