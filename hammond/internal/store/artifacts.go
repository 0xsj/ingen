package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"ingen/hammond/internal/governance"
)

var ErrArtifactNotFound = errors.New("Hammond artifact not found")

// ArtifactStore persists immutable bytes by their SHA-256 identity.
type ArtifactStore interface {
	Put([]byte) (governance.Artifact, error)
	Get(governance.Artifact) ([]byte, error)
}

// FileArtifactStore is a local content-addressed artifact store. The URI
// returned by Put is a locator; the digest-derived filename is the storage
// identity used by Get.
type FileArtifactStore struct {
	root string
	mu   sync.Mutex
}

func NewFileArtifactStore(root string) (*FileArtifactStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("Hammond artifact store root is required")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve Hammond artifact store root: %w", err)
	}
	if err := os.MkdirAll(absoluteRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create Hammond artifact store root: %w", err)
	}
	return &FileArtifactStore{root: absoluteRoot}, nil
}

// Put publishes data idempotently under its SHA-256 digest. Existing bytes
// are never replaced, even when the caller supplies the same digest path.
func (s *FileArtifactStore) Put(data []byte) (governance.Artifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockFile(false)
	if err != nil {
		return governance.Artifact{}, err
	}
	defer unlock()

	digest := sha256.Sum256(data)
	digestText := hex.EncodeToString(digest[:])
	path := s.pathFor(digestText)
	if existing, err := os.ReadFile(path); err == nil {
		if !bytesMatchDigest(existing, digestText) {
			return governance.Artifact{}, fmt.Errorf("Hammond artifact store contains bytes under the wrong digest")
		}
		return governance.Artifact{URI: path, SHA256: digestText}, nil
	} else if !os.IsNotExist(err) {
		return governance.Artifact{}, fmt.Errorf("read existing Hammond artifact: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return governance.Artifact{}, fmt.Errorf("create Hammond artifact directory: %w", err)
	}
	if err := writeBytes(path, data); err != nil {
		return governance.Artifact{}, fmt.Errorf("publish Hammond artifact: %w", err)
	}
	return governance.Artifact{URI: path, SHA256: digestText}, nil
}

// Get loads bytes by digest, verifies the stored bytes, and ignores any URI
// that could redirect the lookup away from this content-addressed store.
func (s *FileArtifactStore) Get(reference governance.Artifact) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockFile(true)
	if err != nil {
		return nil, err
	}
	defer unlock()

	if !validArtifactDigest(reference.SHA256) {
		return nil, fmt.Errorf("Hammond artifact sha256 must be a lowercase SHA-256 digest")
	}
	path := s.pathFor(reference.SHA256)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrArtifactNotFound, reference.SHA256)
		}
		return nil, fmt.Errorf("read Hammond artifact: %w", err)
	}
	if !bytesMatchDigest(data, reference.SHA256) {
		return nil, fmt.Errorf("Hammond artifact bytes do not match reference sha256")
	}
	return data, nil
}

func (s *FileArtifactStore) pathFor(digest string) string {
	return filepath.Join(s.root, digest[:2], digest)
}

func (s *FileArtifactStore) lockFile(shared bool) (func(), error) {
	path := filepath.Join(s.root, ".hammond-artifacts.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open Hammond artifact store lock: %w", err)
	}
	operation := syscall.LOCK_EX
	if shared {
		operation = syscall.LOCK_SH
	}
	if err := syscall.Flock(int(file.Fd()), operation); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock Hammond artifact store: %w", err)
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}

func bytesMatchDigest(data []byte, expected string) bool {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]) == expected
}

func validArtifactDigest(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
