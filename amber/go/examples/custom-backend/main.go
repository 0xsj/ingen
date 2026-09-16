package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"

	amber "github.com/0xsj/ingen/amber"
	amberstorage "github.com/0xsj/ingen/amber/adapters/storage"
)

// applicationKeyValueBackend stands in for an application's database, cache,
// or service-backed key-value implementation. The mutex only makes this small
// example safe for concurrent callers; a real backend must provide the same
// atomic insert-if-absent behavior in its own storage system.
type applicationKeyValueBackend struct {
	mu     sync.RWMutex
	values map[string][]byte
}

func newApplicationKeyValueBackend() *applicationKeyValueBackend {
	return &applicationKeyValueBackend{values: make(map[string][]byte)}
}

func (b *applicationKeyValueBackend) Get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b.mu.RLock()
	value, ok := b.values[key]
	if ok {
		value = append([]byte(nil), value...)
	}
	b.mu.RUnlock()
	if !ok {
		return nil, nil
	}
	return value, nil
}

func (b *applicationKeyValueBackend) PutIfAbsent(ctx context.Context, key string, value []byte) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.values[key]; ok {
		return false, nil
	}
	b.values[key] = append([]byte(nil), value...)
	return true, nil
}

func (b *applicationKeyValueBackend) List(ctx context.Context, prefix string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b.mu.RLock()
	keys := make([]string, 0, len(b.values))
	for key := range b.values {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			keys = append(keys, key)
		}
	}
	b.mu.RUnlock()
	sort.Strings(keys)
	return keys, nil
}

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {
	backend := newApplicationKeyValueBackend()
	store, err := amberstorage.NewKeyValueStore(backend, "orders/provenance")
	if err != nil {
		return err
	}

	root, err := amber.Start()
	if err != nil {
		return err
	}
	retry, err := root.Retry()
	if err != nil {
		return err
	}
	child, err := root.Child(amber.ChildOptions{Origin: amber.OriginIncoming})
	if err != nil {
		return err
	}

	// Insertion order is deliberately different from semantic history order.
	for _, value := range []amber.Provenance{child, retry, root} {
		if err := store.Put(context.Background(), value); err != nil {
			return err
		}
	}
	if err := store.Put(context.Background(), root); err != nil {
		return fmt.Errorf("same value was not idempotent: %w", err)
	}

	conflicting, err := conflictingValue(root)
	if err != nil {
		return err
	}
	if err := store.Put(context.Background(), conflicting); !errors.Is(err, amberstorage.ErrConflict) {
		return fmt.Errorf("conflicting value was accepted: %v", err)
	}

	history, err := store.ListByWorkID(context.Background(), root.WorkID())
	if err != nil {
		return err
	}
	if len(history) != 2 || history[0].ExecutionID() != root.ExecutionID() || history[1].ExecutionID() != retry.ExecutionID() {
		return fmt.Errorf("work history was not deterministic: %v", history)
	}
	correlated, err := store.ListByCorrelationID(context.Background(), root.CorrelationID())
	if err != nil {
		return err
	}
	if len(correlated) != 3 {
		return fmt.Errorf("correlation history length = %d, want 3", len(correlated))
	}

	fmt.Println("custom backend idempotency: true")
	fmt.Println("custom backend conflict protection: true")
	fmt.Printf("custom backend correlation records: %d\n", len(correlated))
	return nil
}

func conflictingValue(provenance amber.Provenance) (amber.Provenance, error) {
	data, err := provenance.MarshalJSON()
	if err != nil {
		return amber.Provenance{}, err
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		return amber.Provenance{}, err
	}
	object["origin"] = string(amber.OriginIncoming)
	conflictingData, err := json.Marshal(object)
	if err != nil {
		return amber.Provenance{}, err
	}
	return amber.FromJSON(conflictingData)
}

var _ amberstorage.KeyValueBackend = (*applicationKeyValueBackend)(nil)
