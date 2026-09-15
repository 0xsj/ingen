package output

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomicallyReplacesExistingOutput(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "nested", "output.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, []byte("new\n")); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "new\n" {
		t.Fatalf("output = %q, want replacement contents", contents)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		t.Fatalf("output directory entries = %+v, want no temporary file", entries)
	}
}

func TestWriteFileDoesNotCreateMissingParent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "output.json")
	if err := WriteFile(path, []byte("data")); err == nil {
		t.Fatal("WriteFile() succeeded, want missing-parent error")
	}
}
