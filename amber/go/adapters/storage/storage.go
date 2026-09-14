// Package amberstorage provides an append-only provenance store contract and
// an in-memory reference implementation.
package amberstorage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	amber "github.com/0xsj/ingen/amber"
)

var (
	ErrNotFound = errors.New("amber provenance not found")
	ErrConflict = errors.New("amber provenance storage conflict")
)

// Store persists immutable provenance values by execution ID.
type Store interface {
	Put(ctx context.Context, provenance amber.Provenance) error
	Get(ctx context.Context, executionID amber.ID) (amber.Provenance, error)
}

// MemoryStore is a concurrency-safe process-local Store. It is useful as a
// reference implementation and for tests; it is not durable storage.
type MemoryStore struct {
	mu      sync.RWMutex
	records map[amber.ID][]byte
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{records: make(map[amber.ID][]byte)}
}

func (s *MemoryStore) Put(ctx context.Context, provenance amber.Provenance) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := provenance.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(provenance)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.records[provenance.ExecutionID()]; ok {
		if bytes.Equal(existing, data) {
			return nil
		}
		return fmt.Errorf("%w: execution_id %s", ErrConflict, provenance.ExecutionID())
	}
	s.records[provenance.ExecutionID()] = append([]byte(nil), data...)
	return nil
}

func (s *MemoryStore) Get(ctx context.Context, executionID amber.ID) (amber.Provenance, error) {
	if err := contextError(ctx); err != nil {
		return amber.Provenance{}, err
	}
	s.mu.RLock()
	data, ok := s.records[executionID]
	if ok {
		data = append([]byte(nil), data...)
	}
	s.mu.RUnlock()
	if !ok {
		return amber.Provenance{}, fmt.Errorf("%w: execution_id %s", ErrNotFound, executionID)
	}
	return amber.FromJSON(data)
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("%w: context cannot be nil", amber.ErrInvalidTransition)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
