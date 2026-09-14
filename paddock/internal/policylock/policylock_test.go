package policylock

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildSaveLoadVerify(t *testing.T) {
	directory := t.TempDir()
	policyPath := filepath.Join(directory, "paddock.yaml")
	lockPath := filepath.Join(directory, "paddock.lock.json")
	contents := []byte(`schema: paddock.architecture/v1
project: lock-test
source:
  language: go
  roots: [internal]
components:
  domain:
    match: internal/domain/**
rules:
  - id: no-cycles
    kind: no-cycles
`)
	if err := os.WriteFile(policyPath, contents, 0o600); err != nil {
		t.Fatal(err)
	}

	sealed, err := Build(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(lockPath, sealed); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SourceSHA256 != sealed.SourceSHA256 || loaded.CanonicalSHA256 != sealed.CanonicalSHA256 {
		t.Fatalf("loaded lock hashes differ: %#v vs %#v", loaded, sealed)
	}
	if err := Verify(policyPath, lockPath); err != nil {
		t.Fatalf("verify sealed policy: %v", err)
	}

	if err := os.WriteFile(policyPath, append(contents, []byte("\n# comment changes the exact sealed input\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Verify(policyPath, lockPath); err == nil || !strings.Contains(err.Error(), "source hash") {
		t.Fatalf("verify after source change = %v, want source hash error", err)
	}
}
