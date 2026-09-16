package main

import (
	"context"
	"errors"
	"sync"
	"testing"

	amber "github.com/0xsj/ingen/amber"
	amberstorage "github.com/0xsj/ingen/amber/adapters/storage"
)

func TestApplicationBackendIsAtomicAndCopiesValues(t *testing.T) {
	backend := newApplicationKeyValueBackend()
	const writers = 32
	inserted := make(chan bool, writers)
	var wait sync.WaitGroup
	for range writers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			ok, err := backend.PutIfAbsent(context.Background(), "race", []byte("value"))
			if err != nil {
				t.Errorf("PutIfAbsent: %v", err)
			}
			inserted <- ok
		}()
	}
	wait.Wait()
	close(inserted)

	count := 0
	for ok := range inserted {
		if ok {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("successful inserts = %d, want 1", count)
	}

	value, err := backend.Get(context.Background(), "race")
	if err != nil {
		t.Fatal(err)
	}
	value[0] = 'X'
	valueAgain, err := backend.Get(context.Background(), "race")
	if err != nil {
		t.Fatal(err)
	}
	if string(valueAgain) != "value" {
		t.Fatalf("stored value changed through returned buffer: %q", valueAgain)
	}
}

func TestApplicationBackendPreservesStoreSemantics(t *testing.T) {
	store, err := amberstorage.NewKeyValueStore(newApplicationKeyValueBackend(), "contract/provenance")
	if err != nil {
		t.Fatal(err)
	}
	root, err := amber.Start()
	if err != nil {
		t.Fatal(err)
	}
	retry, err := root.Retry()
	if err != nil {
		t.Fatal(err)
	}
	child, err := root.Child()
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []amber.Provenance{child, retry, root} {
		if err := store.Put(context.Background(), value); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Put(context.Background(), root); err != nil {
		t.Fatalf("duplicate put: %v", err)
	}
	conflicting, err := conflictingValue(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(context.Background(), conflicting); !errors.Is(err, amberstorage.ErrConflict) {
		t.Fatalf("conflict error = %v", err)
	}
	history, err := store.ListByWorkID(context.Background(), root.WorkID())
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].ExecutionID() != root.ExecutionID() || history[1].ExecutionID() != retry.ExecutionID() {
		t.Fatalf("history = %v, want root then retry", history)
	}
}
