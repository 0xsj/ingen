package store

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListBlobsReportsPublishedArtifacts(t *testing.T) {
	root := t.TempDir()
	store, err := NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := store.Put(bytes.NewBufferString("inventory"), PutOptions{MediaType: "text/plain"})
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := store.ListBlobs()
	if err != nil {
		t.Fatal(err)
	}
	if len(blobs) != 1 || blobs[0].Digest != ref.Digest || blobs[0].SizeBytes != int64(len("inventory")) {
		t.Fatalf("blobs = %+v, want one entry for %+v", blobs, ref)
	}
	if blobs[0].ModifiedAt.IsZero() {
		t.Fatal("ListBlobs returned zero modification time")
	}
}

func TestListBlobsRejectsUnexpectedEntries(t *testing.T) {
	root := t.TempDir()
	store, err := NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "blobs", "sha256", "unexpected"), []byte("not a partition"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListBlobs(); err == nil || !strings.Contains(err.Error(), "unexpected artifact blob directory") {
		t.Fatalf("ListBlobs error = %v, want unexpected directory", err)
	}
}
