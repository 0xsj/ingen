package store

import (
	"bytes"
	"fmt"
	"io"
	"sync"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/integrity"
)

// Memory is a process-local content-addressed store. It has the same
// integrity and intake semantics as the filesystem store, but provides no
// durability guarantees and is intended for tests and short-lived workflows.
type Memory struct {
	mu    sync.RWMutex
	blobs map[string][]byte
}

var _ Store = (*Filesystem)(nil)
var _ Store = (*Memory)(nil)

func NewMemory() *Memory {
	return &Memory{blobs: make(map[string][]byte)}
}

func (s *Memory) Put(reader io.Reader, options PutOptions) (artifact.Reference, error) {
	if s == nil {
		return artifact.Reference{}, fmt.Errorf("memory store is required")
	}
	if reader == nil {
		return artifact.Reference{}, fmt.Errorf("artifact reader is required")
	}
	if options.MediaType == "" {
		return artifact.Reference{}, fmt.Errorf("media type is required")
	}
	if options.MaxBytes < 0 {
		return artifact.Reference{}, fmt.Errorf("maximum artifact size cannot be negative")
	}
	if options.ExpectedDigest != "" {
		if err := artifact.ValidateDigest(options.ExpectedDigest); err != nil {
			return artifact.Reference{}, err
		}
	}

	data, result, err := integrity.ReadAll(reader, options.MaxBytes)
	if err != nil {
		return artifact.Reference{}, err
	}
	if err := integrity.VerifyDigest(result.Digest, options.ExpectedDigest); err != nil {
		return artifact.Reference{}, err
	}
	digest := result.Digest

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blobs == nil {
		s.blobs = make(map[string][]byte)
	}
	if existing, ok := s.blobs[digest]; ok {
		if !bytes.Equal(existing, data) {
			return artifact.Reference{}, fmt.Errorf("existing artifact failed verification: digest collision")
		}
	} else {
		s.blobs[digest] = append([]byte(nil), data...)
	}
	return artifact.Reference{
		Schema:      artifact.Schema,
		Digest:      digest,
		SizeBytes:   result.SizeBytes,
		MediaType:   options.MediaType,
		LogicalName: options.LogicalName,
	}, nil
}

func (s *Memory) Get(digest string) ([]byte, error) {
	if err := s.Verify(digest); err != nil {
		return nil, err
	}
	s.mu.RLock()
	data := append([]byte(nil), s.blobs[digest]...)
	s.mu.RUnlock()
	return data, nil
}

func (s *Memory) Verify(digest string) error {
	if s == nil {
		return fmt.Errorf("memory store is required")
	}
	if err := artifact.ValidateDigest(digest); err != nil {
		return err
	}
	s.mu.RLock()
	data, ok := s.blobs[digest]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("artifact %s not found", digest)
	}
	if actual := artifact.DigestBytes(data); actual != digest {
		return fmt.Errorf("artifact digest mismatch: got %s, want %s", actual, digest)
	}
	return nil
}

func (s *Memory) VerifyReference(reference artifact.Reference) error {
	if reference.Schema != artifact.Schema {
		return fmt.Errorf("unexpected artifact schema %q", reference.Schema)
	}
	if reference.SizeBytes < 0 {
		return fmt.Errorf("artifact size cannot be negative")
	}
	data, err := s.Get(reference.Digest)
	if err != nil {
		return err
	}
	if int64(len(data)) != reference.SizeBytes {
		return fmt.Errorf("artifact size mismatch: got %d, want %d", len(data), reference.SizeBytes)
	}
	return nil
}
