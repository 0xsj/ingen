package store

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"ingen/lockwood/internal/artifact"
)

func TestStoreContract(t *testing.T) {
	for _, backend := range []struct {
		name string
		new  func(t *testing.T) Store
	}{
		{
			name: "filesystem",
			new: func(t *testing.T) Store {
				store, err := NewFilesystem(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				return store
			},
		},
		{
			name: "memory",
			new: func(t *testing.T) Store {
				return NewMemory()
			},
		},
	} {
		t.Run(backend.name, func(t *testing.T) {
			store := backend.new(t)
			contents := []byte("shared store contract")
			ref, err := store.Put(bytes.NewReader(contents), PutOptions{
				MediaType:   "text/plain",
				LogicalName: "contract.txt",
			})
			if err != nil {
				t.Fatal(err)
			}
			if ref.Digest != artifact.DigestBytes(contents) || ref.SizeBytes != int64(len(contents)) {
				t.Fatalf("reference = %+v", ref)
			}
			if err := store.Verify(ref.Digest); err != nil {
				t.Fatal(err)
			}
			if err := store.VerifyReference(ref); err != nil {
				t.Fatal(err)
			}
			got, err := store.Get(ref.Digest)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, contents) {
				t.Fatalf("retrieved contents = %q, want %q", got, contents)
			}
			got[0] = 'S'
			gotAgain, err := store.Get(ref.Digest)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(gotAgain, contents) {
				t.Fatalf("store exposed mutable bytes: got %q, want %q", gotAgain, contents)
			}

			duplicate, err := store.Put(bytes.NewReader(contents), PutOptions{MediaType: "application/octet-stream"})
			if err != nil {
				t.Fatal(err)
			}
			if duplicate.Digest != ref.Digest || duplicate.SizeBytes != ref.SizeBytes {
				t.Fatalf("duplicate reference = %+v, want %+v", duplicate, ref)
			}

			badSize := ref
			badSize.SizeBytes++
			if err := store.VerifyReference(badSize); err == nil || !strings.Contains(err.Error(), "size mismatch") {
				t.Fatalf("bad-size reference error = %v, want size mismatch", err)
			}

			fullDigest := artifact.DigestBytes([]byte("too large"))
			if _, err := store.Put(strings.NewReader("too large"), PutOptions{MediaType: "text/plain", MaxBytes: 3}); err == nil || !strings.Contains(err.Error(), "exceeds maximum size") {
				t.Fatalf("oversized Put error = %v, want maximum-size rejection", err)
			}
			if err := store.Verify(fullDigest); err == nil {
				t.Fatal("oversized artifact remained published")
			}

			mismatchedDigest := artifact.DigestBytes([]byte("digest mismatch"))
			if _, err := store.Put(strings.NewReader("digest mismatch"), PutOptions{
				MediaType:      "text/plain",
				ExpectedDigest: "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			}); err == nil || !strings.Contains(err.Error(), "expected digest") {
				t.Fatalf("expected-digest error = %v, want mismatch", err)
			}
			if err := store.Verify(mismatchedDigest); err == nil {
				t.Fatal("digest-mismatched artifact remained published")
			}
		})
	}
}

func TestStoreContractRejectsNilReader(t *testing.T) {
	for _, backend := range []struct {
		name string
		new  func(t *testing.T) Store
	}{
		{name: "filesystem", new: func(t *testing.T) Store {
			store, err := NewFilesystem(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			return store
		}},
		{name: "memory", new: func(t *testing.T) Store { return NewMemory() }},
	} {
		t.Run(backend.name, func(t *testing.T) {
			if _, err := backend.new(t).Put(nil, PutOptions{MediaType: "text/plain"}); err == nil || !strings.Contains(err.Error(), "reader is required") {
				t.Fatalf("nil-reader error = %v, want reader validation", err)
			}
		})
	}
}

func TestStoreConcurrentIdempotentPut(t *testing.T) {
	for _, backend := range []struct {
		name string
		new  func(t *testing.T) Store
	}{
		{
			name: "filesystem",
			new: func(t *testing.T) Store {
				store, err := NewFilesystem(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				return store
			},
		},
		{name: "memory", new: func(t *testing.T) Store { return NewMemory() }},
	} {
		t.Run(backend.name, func(t *testing.T) {
			store := backend.new(t)
			contents := []byte("concurrent artifact")
			const writers = 16
			refs := make(chan artifact.Reference, writers)
			errs := make(chan error, writers)
			var wait sync.WaitGroup
			wait.Add(writers)
			for range writers {
				go func() {
					defer wait.Done()
					ref, err := store.Put(bytes.NewReader(contents), PutOptions{MediaType: "text/plain"})
					if err != nil {
						errs <- err
						return
					}
					refs <- ref
				}()
			}
			wait.Wait()
			close(refs)
			close(errs)
			for err := range errs {
				t.Fatal(err)
			}
			var first artifact.Reference
			for ref := range refs {
				if first.Digest == "" {
					first = ref
					continue
				}
				if ref.Digest != first.Digest || ref.SizeBytes != first.SizeBytes {
					t.Fatalf("concurrent references differ: first=%+v ref=%+v", first, ref)
				}
			}
			if first.Digest == "" {
				t.Fatal("concurrent writes returned no references")
			}
			if err := store.VerifyReference(first); err != nil {
				t.Fatal(err)
			}
		})
	}
}
