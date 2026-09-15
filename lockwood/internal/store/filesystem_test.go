package store

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilesystemPutGetAndVerify(t *testing.T) {
	store, err := NewFilesystem(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	contents := []byte(`{"schema":"ingen.ci-result/v1","status":"passed"}`)
	ref, err := store.Put(bytes.NewReader(contents), PutOptions{
		MediaType:   "application/json",
		LogicalName: "ci-result.json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ref.SizeBytes != int64(len(contents)) || ref.MediaType != "application/json" {
		t.Fatalf("reference = %+v", ref)
	}
	if err := store.Verify(ref.Digest); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ref.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, contents) {
		t.Fatalf("retrieved contents = %q, want %q", got, contents)
	}
}

func TestFilesystemPutIsIdempotentAndDoesNotReplaceCorruptBytes(t *testing.T) {
	root := t.TempDir()
	store, err := NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}

	contents := []byte("same bytes")
	first, err := store.Put(bytes.NewReader(contents), PutOptions{MediaType: "text/plain"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Put(bytes.NewReader(contents), PutOptions{MediaType: "application/octet-stream"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest != second.Digest || first.SizeBytes != second.SizeBytes {
		t.Fatalf("duplicate references differ: first=%+v second=%+v", first, second)
	}

	path, err := store.blobPath(first.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Verify(first.Digest); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("Verify error = %v, want digest mismatch", err)
	}
	if _, err := store.Put(bytes.NewReader(contents), PutOptions{MediaType: "text/plain"}); err == nil || !strings.Contains(err.Error(), "failed verification") {
		t.Fatalf("Put error = %v, want existing verification failure", err)
	}
}

func TestFilesystemPutChecksExpectedDigestBeforePublish(t *testing.T) {
	root := t.TempDir()
	store, err := NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Put(strings.NewReader("data"), PutOptions{
		ExpectedDigest: "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		MediaType:      "text/plain",
	})
	if err == nil || !strings.Contains(err.Error(), "expected digest") {
		t.Fatalf("Put error = %v, want expected digest mismatch", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "blobs", "sha256"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("published blob directories = %d, want 0", len(entries))
	}
}

func TestFilesystemRejectsUnsafeDigest(t *testing.T) {
	store, err := NewFilesystem(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("sha256:../../artifact"); err == nil {
		t.Fatal("Get accepted an unsafe digest")
	}
}
