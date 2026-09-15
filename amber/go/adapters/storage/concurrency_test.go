package amberstorage

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	amber "github.com/0xsj/ingen/amber"
)

func TestMemoryStoreConcurrentWritesAndQueries(t *testing.T) {
	store := NewMemoryStore()
	values := storageTestValues(t, 24)
	ctx := context.Background()
	if err := store.Put(ctx, values[0]); err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error, len(values)*2+8)
	var writers sync.WaitGroup
	for _, value := range values[1:] {
		for duplicate := 0; duplicate < 2; duplicate++ {
			value := value
			writers.Add(1)
			go func() {
				defer writers.Done()
				if err := store.Put(ctx, value); err != nil {
					errCh <- err
				}
			}()
		}
	}

	var readers sync.WaitGroup
	for i := 0; i < 8; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for j := 0; j < 32; j++ {
				if _, err := store.ListByCorrelationID(ctx, values[0].CorrelationID()); err != nil {
					errCh <- err
					return
				}
			}
		}()
	}

	writers.Wait()
	readers.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}

	history, err := store.ListByCorrelationID(ctx, values[0].CorrelationID())
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != len(values) {
		t.Fatalf("concurrent writes stored %d records, want %d", len(history), len(values))
	}
}

func TestFileStoreConcurrentWritesRemainReadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provenance.json")
	store, err := NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	values := storageTestValues(t, 12)
	ctx := context.Background()
	errCh := make(chan error, len(values))
	var writers sync.WaitGroup
	for _, value := range values {
		value := value
		writers.Add(1)
		go func() {
			defer writers.Done()
			if err := store.Put(ctx, value); err != nil {
				errCh <- err
			}
		}()
	}
	writers.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}

	reopened, err := NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	history, err := reopened.ListByCorrelationID(ctx, values[0].CorrelationID())
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != len(values) {
		t.Fatalf("concurrent writes persisted %d records, want %d", len(history), len(values))
	}
}

func TestKeyValueStoreConcurrentWritesAndQueries(t *testing.T) {
	backend := NewMapKeyValueBackend()
	store, err := NewKeyValueStore(backend, "concurrent/provenance")
	if err != nil {
		t.Fatal(err)
	}
	values := storageTestValues(t, 24)
	ctx := context.Background()

	errCh := make(chan error, len(values)*2+8)
	var writers sync.WaitGroup
	for _, value := range values {
		value := value
		for duplicate := 0; duplicate < 2; duplicate++ {
			writers.Add(1)
			go func() {
				defer writers.Done()
				if err := store.Put(ctx, value); err != nil {
					errCh <- err
				}
			}()
		}
	}

	var readers sync.WaitGroup
	for i := 0; i < 8; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for j := 0; j < 32; j++ {
				if _, err := store.ListByCorrelationID(ctx, values[0].CorrelationID()); err != nil {
					errCh <- err
					return
				}
			}
		}()
	}

	writers.Wait()
	readers.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}

	history, err := store.ListByCorrelationID(ctx, values[0].CorrelationID())
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != len(values) {
		t.Fatalf("concurrent key-value writes stored %d records, want %d", len(history), len(values))
	}
}

func storageTestValues(t *testing.T, count int) []amber.Provenance {
	t.Helper()
	root, err := amber.Start()
	if err != nil {
		t.Fatal(err)
	}
	values := make([]amber.Provenance, 0, count)
	for i := 0; i < count; i++ {
		value, err := root.Child()
		if err != nil {
			t.Fatal(fmt.Errorf("create test value %d: %w", i, err))
		}
		values = append(values, value)
	}
	return values
}
