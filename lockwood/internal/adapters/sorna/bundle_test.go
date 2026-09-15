package sorna

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"ingen/lockwood/internal/custody"
	"ingen/lockwood/internal/store"
)

func TestImporterArchivesVerifiedEvidenceDeterministically(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(t.TempDir(), "run-bundle")
	writeSornaFixture(t, bundle, EvidenceSchema, "run-0001", map[string][]byte{
		"run.json":               []byte(`{"schema":"ingen.run/v1","run_id":"run-0001"}`),
		"events/lifecycle.jsonl": []byte(`{"sequence":1}` + "\n"),
	})
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

	first, err := importer.Import(bundle, ImportRequest{CustodyID: "lockwood-sorna-0001"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := importer.Import(bundle, ImportRequest{CustodyID: "lockwood-sorna-0002"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Artifact.Digest != second.Artifact.Digest {
		t.Fatalf("deterministic archive digests differ: %s vs %s", first.Artifact.Digest, second.Artifact.Digest)
	}
	if first.Artifact.MediaType != EvidenceMediaType || first.Producer.Kind != "evidence-bundle" || first.Source.RunID != "run-0001" {
		t.Fatalf("imported record = %+v", first)
	}
	if err := artifacts.Verify(first.Artifact.Digest); err != nil {
		t.Fatal(err)
	}
	archive, err := artifacts.Get(first.Artifact.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(archive, []byte("manifest.json")) || !bytes.Contains(archive, []byte("checksums.sha256")) {
		t.Fatal("archive does not contain expected bundle entries")
	}
}

func TestImporterRejectsTamperedAndUnchecksummedBundles(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(t *testing.T, bundle string)
		message string
	}{
		{
			name: "tampered",
			mutate: func(t *testing.T, bundle string) {
				if err := os.WriteFile(filepath.Join(bundle, "run.json"), []byte("tampered"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			message: "checksum mismatch",
		},
		{
			name: "unchecksummed",
			mutate: func(t *testing.T, bundle string) {
				if err := os.WriteFile(filepath.Join(bundle, "extra.txt"), []byte("extra"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			message: "unchecksummed file",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle := filepath.Join(t.TempDir(), "run-bundle")
			writeSornaFixture(t, bundle, EvidenceSchema, "run-0001", map[string][]byte{
				"run.json": []byte(`{"schema":"ingen.run/v1","run_id":"run-0001"}`),
			})
			test.mutate(t, bundle)
			if _, err := readSnapshot(bundle); err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("readSnapshot error = %v, want %q", err, test.message)
			}
		})
	}
}

func TestImporterRejectsSymlinksAndUnsupportedManifestSchemas(t *testing.T) {
	bundle := filepath.Join(t.TempDir(), "run-bundle")
	writeSornaFixture(t, bundle, "unknown/v1", "", map[string][]byte{
		"run.json": []byte("run"),
	})
	if _, err := readSnapshot(bundle); err == nil || !strings.Contains(err.Error(), "unsupported Sorna manifest schema") {
		t.Fatalf("unsupported schema error = %v", err)
	}

	symlinkBundle := filepath.Join(t.TempDir(), "symlink-bundle")
	writeSornaFixture(t, symlinkBundle, EvidenceSchema, "run-0002", map[string][]byte{
		"run.json": []byte("run"),
	})
	if err := os.Symlink(filepath.Join(symlinkBundle, "run.json"), filepath.Join(symlinkBundle, "events-link")); err != nil {
		t.Skipf("symlink test unavailable: %v", err)
	}
	if _, err := readSnapshot(symlinkBundle); err == nil || !strings.Contains(err.Error(), "contains symlink") {
		t.Fatalf("symlink error = %v", err)
	}
}

func writeSornaFixture(t *testing.T, directory, schema, runID string, files map[string][]byte) {
	t.Helper()
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]string{"schema": schema}
	if runID != "" {
		manifest["run_id"] = runID
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	files["manifest.json"] = append(manifestBytes, '\n')
	checksums := make([]string, 0, len(files))
	for name, data := range files {
		path := filepath.Join(directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		checksums = append(checksums, hex.EncodeToString(digest[:])+"  "+name)
	}
	sort.Strings(checksums)
	if err := os.WriteFile(filepath.Join(directory, "checksums.sha256"), []byte(strings.Join(checksums, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
