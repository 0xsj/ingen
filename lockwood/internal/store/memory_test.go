package store

import (
	"bytes"
	"strings"
	"testing"
)

func TestMemoryStorePutGetVerifyAndReference(t *testing.T) {
	var memory Memory
	contents := []byte("memory artifact")
	ref, err := memory.Put(bytes.NewReader(contents), PutOptions{
		MediaType:   "text/plain",
		LogicalName: "memory.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := memory.VerifyReference(ref); err != nil {
		t.Fatal(err)
	}
	got, err := memory.Get(ref.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, contents) {
		t.Fatalf("retrieved contents = %q, want %q", got, contents)
	}
	got[0] = 'M'
	if err := memory.Verify(ref.Digest); err != nil {
		t.Fatalf("mutating returned bytes affected store: %v", err)
	}
}

func TestMemoryStoreRejectsOversizedArtifactAndExpectedDigestMismatch(t *testing.T) {
	store := NewMemory()
	if _, err := store.Put(strings.NewReader("1234"), PutOptions{MediaType: "text/plain", MaxBytes: 3}); err == nil || !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Fatalf("oversized Put error = %v, want maximum-size rejection", err)
	}
	if _, err := store.Put(strings.NewReader("data"), PutOptions{
		MediaType:      "text/plain",
		ExpectedDigest: "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
	}); err == nil || !strings.Contains(err.Error(), "expected digest") {
		t.Fatalf("digest-mismatch Put error = %v, want expected-digest rejection", err)
	}
}
