package evidence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ingen/sorna/internal/lifecycle"
	"ingen/sorna/internal/runner"
)

func TestWriteBundleAndVerify(t *testing.T) {
	directory := t.TempDir()
	now := time.Date(2026, 9, 13, 20, 0, 0, 0, time.UTC)
	record := runner.RunRecord{
		Schema:    "ingen.run/v1",
		RunID:     "run-evidence-test",
		CreatedAt: now,
		Contract:  runner.ContractReference{ID: "document-pipeline", Version: 1, SHA256: strings.Repeat("a", 64)},
		Subject:   runner.SubjectReference{BaseURL: "http://subject.invalid", Adapter: "http-json-v1"},
		Lifecycle: &lifecycle.Record{Mode: "managed-process", Outcome: "stopped", Events: []lifecycle.Event{{
			Sequence:  1,
			Timestamp: now,
			Kind:      "subject.process.started",
			Detail:    "command launched",
		}}},
	}
	bundle, err := WriteBundle(directory, record)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{bundle.RunPath, bundle.ManifestPath, bundle.LifecyclePath, bundle.ChecksumsPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
	}

	manifestBytes, err := os.ReadFile(bundle.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Schema != "sorna.evidence/v1" || manifest.RunID != record.RunID {
		t.Fatalf("manifest = %+v, want evidence schema and run ID", manifest)
	}
	if len(manifest.ArtifactsSHA256) != 2 {
		t.Fatalf("manifest artifacts = %+v, want run and lifecycle hashes", manifest.ArtifactsSHA256)
	}
	lifecycleBytes, err := os.ReadFile(bundle.LifecyclePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(lifecycleBytes), `"run_id":"run-evidence-test"`) || !strings.Contains(string(lifecycleBytes), `"actor":"sorna"`) {
		t.Fatalf("lifecycle JSONL = %s, want run identity and actor", lifecycleBytes)
	}
	if err := Verify(directory); err != nil {
		t.Fatalf("Verify() = %v, want valid bundle", err)
	}

	if err := os.WriteFile(filepath.Join(directory, "run.json"), []byte("tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Verify(directory); err == nil || !strings.Contains(err.Error(), "checksum mismatch for run.json") {
		t.Fatalf("Verify() after tamper = %v, want run checksum mismatch", err)
	}
}

func TestVerifyRejectsUnsafeChecksumPath(t *testing.T) {
	directory := t.TempDir()
	checksums := strings.Repeat("a", 64) + "  ../outside\n"
	if err := os.WriteFile(filepath.Join(directory, "checksums.sha256"), []byte(checksums), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Verify(directory); err == nil || !strings.Contains(err.Error(), "inside the evidence bundle") {
		t.Fatalf("Verify() = %v, want unsafe path error", err)
	}
}
