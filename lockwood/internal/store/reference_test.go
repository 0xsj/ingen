package store

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/lockwood/internal/artifact"
)

func TestFilesystemPersistsReferenceMetadataAcrossReopen(t *testing.T) {
	root := t.TempDir()
	filesystem, err := NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	contents := []byte("reference metadata")
	first, err := filesystem.Put(bytes.NewReader(contents), PutOptions{
		MediaType:   "text/plain",
		LogicalName: "result.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary entries after reference publication = %+v", entries)
	}

	reopened, err := NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	references, err := reopened.ListReferences()
	if err != nil {
		t.Fatal(err)
	}
	if len(references) != 1 || references[0] != first {
		t.Fatalf("reopened references = %+v, want [%+v]", references, first)
	}

	second, err := reopened.Put(bytes.NewReader(contents), PutOptions{
		MediaType:   "application/octet-stream",
		LogicalName: "result.bin",
	})
	if err != nil {
		t.Fatal(err)
	}
	references, err = reopened.ListReferences()
	if err != nil {
		t.Fatal(err)
	}
	if len(references) != 2 || !containsReference(references, first) || !containsReference(references, second) {
		t.Fatalf("reference variants = %+v, want both metadata variants", references)
	}
}

func TestFilesystemListReferencesFailsClosedOnCorruptMetadata(t *testing.T) {
	root := t.TempDir()
	filesystem, err := NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := filesystem.Put(bytes.NewBufferString("corrupt reference metadata"), PutOptions{MediaType: "text/plain"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := marshalReference(ref)
	if err != nil {
		t.Fatal(err)
	}
	path, err := filesystem.referencePath(ref, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := filesystem.ListReferences(); err == nil || !strings.Contains(err.Error(), "decode artifact reference") {
		t.Fatalf("ListReferences error = %v, want corrupt metadata error", err)
	}
}

func TestMemoryStoreListsReferenceVariants(t *testing.T) {
	memory := NewMemory()
	contents := []byte("memory reference metadata")
	first, err := memory.Put(bytes.NewReader(contents), PutOptions{MediaType: "text/plain", LogicalName: "one.txt"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := memory.Put(bytes.NewReader(contents), PutOptions{MediaType: "application/json", LogicalName: "two.json"})
	if err != nil {
		t.Fatal(err)
	}
	references, err := memory.ListReferences()
	if err != nil {
		t.Fatal(err)
	}
	if len(references) != 2 || !containsReference(references, first) || !containsReference(references, second) {
		t.Fatalf("memory references = %+v, want both metadata variants", references)
	}
}

func containsReference(references []artifact.Reference, want artifact.Reference) bool {
	for _, reference := range references {
		if reference == want {
			return true
		}
	}
	return false
}
