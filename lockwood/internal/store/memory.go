package store

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"sync"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/integrity"
)

// Memory is a process-local content-addressed store. It has the same
// integrity and intake semantics as the filesystem store, but provides no
// durability guarantees and is intended for tests and short-lived workflows.
type Memory struct {
	mu         sync.RWMutex
	blobs      map[string][]byte
	references map[string]artifact.Reference
}

var _ Store = (*Filesystem)(nil)
var _ Store = (*Memory)(nil)
var _ ReferenceLister = (*Filesystem)(nil)
var _ ReferenceLister = (*Memory)(nil)

func NewMemory() *Memory {
	return &Memory{
		blobs:      make(map[string][]byte),
		references: make(map[string]artifact.Reference),
	}
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
	reference := artifact.Reference{
		Schema:      artifact.Schema,
		Digest:      digest,
		SizeBytes:   result.SizeBytes,
		MediaType:   options.MediaType,
		LogicalName: options.LogicalName,
	}
	key, err := referenceKey(reference)
	if err != nil {
		return artifact.Reference{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blobs == nil {
		s.blobs = make(map[string][]byte)
	}
	if s.references == nil {
		s.references = make(map[string]artifact.Reference)
	}
	if existing, ok := s.blobs[digest]; ok {
		if !bytes.Equal(existing, data) {
			return artifact.Reference{}, fmt.Errorf("existing artifact failed verification: digest collision")
		}
	} else {
		s.blobs[digest] = append([]byte(nil), data...)
	}
	s.references[key] = reference
	return reference, nil
}

func (s *Memory) ListReferences() ([]artifact.Reference, error) {
	if s == nil {
		return nil, fmt.Errorf("memory store is required")
	}
	s.mu.RLock()
	references := make([]artifact.Reference, 0, len(s.references))
	for _, reference := range s.references {
		references = append(references, reference)
	}
	s.mu.RUnlock()
	sort.Slice(references, func(i, j int) bool {
		if references[i].Digest != references[j].Digest {
			return references[i].Digest < references[j].Digest
		}
		if references[i].MediaType != references[j].MediaType {
			return references[i].MediaType < references[j].MediaType
		}
		return references[i].LogicalName < references[j].LogicalName
	})
	return references, nil
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
