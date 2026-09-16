package store

import (
	"errors"
	"os"
	"strings"
	"testing"

	"ingen/hammond/internal/governance"
)

func TestFileArtifactStorePutsGetsAndDoesNotReplaceBytes(t *testing.T) {
	artifactStore, err := NewFileArtifactStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("immutable contract bytes\n")
	artifact, err := artifactStore.Put(data)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.URI == "" || len(artifact.SHA256) != 64 {
		t.Fatalf("artifact = %#v, want locator and digest", artifact)
	}
	duplicate, err := artifactStore.Put(data)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate != artifact {
		t.Fatalf("duplicate artifact = %#v, want %#v", duplicate, artifact)
	}

	loaded, err := artifactStore.Get(governance.Artifact{URI: "ignored-by-content-addressing", SHA256: artifact.SHA256})
	if err != nil {
		t.Fatal(err)
	}
	if string(loaded) != string(data) {
		t.Fatalf("loaded = %q, want %q", loaded, data)
	}

	if err := os.WriteFile(artifact.URI, []byte("tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := artifactStore.Get(artifact); err == nil || !strings.Contains(err.Error(), "do not match reference sha256") {
		t.Fatalf("tampered artifact error = %v, want digest failure", err)
	}
}

func TestFileArtifactStoreRejectsMissingAndInvalidDigests(t *testing.T) {
	artifactStore, err := NewFileArtifactStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	missing := governance.Artifact{SHA256: strings.Repeat("a", 64)}
	if _, err := artifactStore.Get(missing); err == nil || !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("missing artifact error = %v, want ErrArtifactNotFound", err)
	}
	for _, digest := range []string{"", strings.Repeat("A", 64), "not-a-digest"} {
		if _, err := artifactStore.Get(governance.Artifact{SHA256: digest}); err == nil || !strings.Contains(err.Error(), "lowercase SHA-256 digest") {
			t.Fatalf("digest %q error = %v, want validation failure", digest, err)
		}
	}
}
