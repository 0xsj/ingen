package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/core/ciresult"
)

func TestLoadFileHashesTheValidatedBytes(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "result.json")
	result := ciresult.Artifact{
		Schema:      ciresult.Schema,
		Tool:        "sorna",
		Kind:        "test",
		Status:      "passed",
		ExitCode:    0,
		CreatedAt:   "2026-09-15T12:00:00Z",
		Source:      ciresult.Source{Root: "."},
		Report:      []byte(`{"ok":true}`),
		Explanation: []byte(`{"ok":true}`),
	}
	contents, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(contents)
	if loaded.SHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("SHA256 = %q, want hash of file bytes", loaded.SHA256)
	}
	if loaded.Artifact.Tool != "sorna" || !strings.Contains(string(loaded.Artifact.Report), "ok") {
		t.Fatalf("loaded artifact = %+v, want validated producer artifact", loaded.Artifact)
	}
}
