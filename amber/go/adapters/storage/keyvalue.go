package amberstorage

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"sync"

	amber "github.com/0xsj/ingen/amber"
)

// KeyValueBackend is the minimal backend seam for a durable or externally
// managed key-value store. PutIfAbsent must be atomic for a given key; a
// read-then-write implementation does not preserve the Store contract.
type KeyValueBackend interface {
	Get(context.Context, string) ([]byte, error)
	PutIfAbsent(context.Context, string, []byte) (bool, error)
	List(context.Context, string) ([]string, error)
}

// MapKeyValueBackend is a concurrency-safe process-local reference backend for
// KeyValueStore tests and examples. It provides no durability guarantee.
type MapKeyValueBackend struct {
	mu      sync.RWMutex
	records map[string][]byte
}

// NewMapKeyValueBackend creates an empty reference key-value backend.
func NewMapKeyValueBackend() *MapKeyValueBackend {
	return &MapKeyValueBackend{records: make(map[string][]byte)}
}

func (b *MapKeyValueBackend) Get(ctx context.Context, key string) ([]byte, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	b.mu.RLock()
	value, ok := b.records[key]
	if ok {
		value = append([]byte(nil), value...)
	}
	b.mu.RUnlock()
	if !ok {
		return nil, nil
	}
	return value, nil
}

func (b *MapKeyValueBackend) PutIfAbsent(ctx context.Context, key string, value []byte) (bool, error) {
	if err := contextError(ctx); err != nil {
		return false, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.records[key]; ok {
		return false, nil
	}
	b.records[key] = append([]byte(nil), value...)
	return true, nil
}

func (b *MapKeyValueBackend) List(ctx context.Context, prefix string) ([]string, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	b.mu.RLock()
	keys := make([]string, 0, len(b.records))
	for key := range b.records {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			keys = append(keys, key)
		}
	}
	b.mu.RUnlock()
	sort.Strings(keys)
	return keys, nil
}

// KeyValueStore implements Store over a caller-supplied KeyValueBackend. It
// owns the Amber serialization and query semantics while the caller owns the
// backend's durability, transactions, and concurrency implementation.
type KeyValueStore struct {
	backend   KeyValueBackend
	namespace string
}

// NewKeyValueStore creates a Store over backend. An empty namespace uses the
// default "amber/provenance" prefix.
func NewKeyValueStore(backend KeyValueBackend, namespace string) (*KeyValueStore, error) {
	if backend == nil {
		return nil, fmt.Errorf("%w: key-value backend cannot be nil", amber.ErrInvalidTransition)
	}
	if namespace == "" {
		namespace = "amber/provenance"
	}
	return &KeyValueStore{backend: backend, namespace: namespace}, nil
}

func (s *KeyValueStore) Put(ctx context.Context, provenance amber.Provenance) error {
	if err := s.validate(); err != nil {
		return err
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := provenance.Validate(); err != nil {
		return err
	}
	data, err := provenance.MarshalJSON()
	if err != nil {
		return err
	}
	key := s.key(provenance.ExecutionID())
	inserted, err := s.backend.PutIfAbsent(ctx, key, data)
	if err != nil {
		return err
	}
	if inserted {
		return nil
	}
	existing, err := s.backend.Get(ctx, key)
	if err != nil {
		return err
	}
	if bytes.Equal(existing, data) {
		return nil
	}
	return fmt.Errorf("%w: execution_id %s", ErrConflict, provenance.ExecutionID())
}

func (s *KeyValueStore) Get(ctx context.Context, executionID amber.ID) (amber.Provenance, error) {
	if err := s.validate(); err != nil {
		return amber.Provenance{}, err
	}
	if err := contextError(ctx); err != nil {
		return amber.Provenance{}, err
	}
	data, err := s.backend.Get(ctx, s.key(executionID))
	if err != nil {
		return amber.Provenance{}, err
	}
	if data == nil {
		return amber.Provenance{}, fmt.Errorf("%w: execution_id %s", ErrNotFound, executionID)
	}
	return amber.FromJSON(data)
}

func (s *KeyValueStore) ListByWorkID(ctx context.Context, workID amber.ID) ([]amber.Provenance, error) {
	return s.list(ctx, func(provenance amber.Provenance) bool {
		return provenance.WorkID() == workID
	})
}

func (s *KeyValueStore) ListByCausation(ctx context.Context, kind string, id amber.ID) ([]amber.Provenance, error) {
	return s.list(ctx, func(provenance amber.Provenance) bool {
		causation, ok := provenance.Causation()
		return ok && causation.Kind == kind && causation.ID == id
	})
}

func (s *KeyValueStore) ListByCorrelationID(ctx context.Context, correlationID amber.ID) ([]amber.Provenance, error) {
	return s.list(ctx, func(provenance amber.Provenance) bool {
		return provenance.CorrelationID() == correlationID
	})
}

func (s *KeyValueStore) list(ctx context.Context, matches func(amber.Provenance) bool) ([]amber.Provenance, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	keys, err := s.backend.List(ctx, s.namespace+"/")
	if err != nil {
		return nil, err
	}
	data := make([][]byte, 0, len(keys))
	for _, key := range keys {
		value, err := s.backend.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		if value != nil {
			data = append(data, value)
		}
	}
	history := make([]amber.Provenance, 0, len(data))
	for _, value := range data {
		provenance, err := amber.FromJSON(value)
		if err != nil {
			return nil, err
		}
		if matches(provenance) {
			history = append(history, provenance)
		}
	}
	sortHistory(history)
	return history, nil
}

func (s *KeyValueStore) key(executionID amber.ID) string {
	return s.namespace + "/" + executionID.String()
}

func (s *KeyValueStore) validate() error {
	if s == nil || s.backend == nil {
		return fmt.Errorf("%w: key-value backend cannot be nil", amber.ErrInvalidTransition)
	}
	return nil
}

var _ KeyValueBackend = (*MapKeyValueBackend)(nil)
var _ Store = (*KeyValueStore)(nil)
