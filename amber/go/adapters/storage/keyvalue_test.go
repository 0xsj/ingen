package amberstorage

import (
	"context"
	"errors"
	"testing"

	amber "github.com/0xsj/ingen/amber"
)

func TestKeyValueStorePreservesAppendOnlyContract(t *testing.T) {
	backend := NewMapKeyValueBackend()
	store, err := NewKeyValueStore(backend, "test/amber")
	if err != nil {
		t.Fatal(err)
	}
	root, err := amber.Start()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Put(ctx, root); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, root); err != nil {
		t.Fatalf("same value should be idempotent: %v", err)
	}
	stored, err := store.Get(ctx, root.ExecutionID())
	if err != nil || stored.ExecutionID() != root.ExecutionID() {
		t.Fatalf("stored value=%s err=%v", stored.ExecutionID(), err)
	}

	conflictingData := replaceJSONField(mustJSON(root), "origin", "incoming")
	conflicting, err := amber.FromJSON(conflictingData)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, conflicting); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}

	reopened, err := NewKeyValueStore(backend, "test/amber")
	if err != nil {
		t.Fatal(err)
	}
	retry, err := root.Retry()
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.Put(ctx, retry); err != nil {
		t.Fatal(err)
	}
	causal, err := reopened.ListByCausation(ctx, "execution", root.ExecutionID())
	if err != nil || len(causal) != 1 || causal[0].ExecutionID() != retry.ExecutionID() {
		t.Fatalf("causal history=%v err=%v", causal, err)
	}
	correlated, err := reopened.ListByCorrelationID(ctx, root.CorrelationID())
	if err != nil || len(correlated) != 2 {
		t.Fatalf("correlation history=%v err=%v", correlated, err)
	}
}

func TestMapKeyValueBackendCopiesValuesAndListsPrefix(t *testing.T) {
	backend := NewMapKeyValueBackend()
	ctx := context.Background()
	value := []byte("value")
	inserted, err := backend.PutIfAbsent(ctx, "amber/b", value)
	if err != nil || !inserted {
		t.Fatalf("first insert: inserted=%v err=%v", inserted, err)
	}
	value[0] = 'X'
	inserted, err = backend.PutIfAbsent(ctx, "amber/a", []byte("other"))
	if err != nil || !inserted {
		t.Fatalf("second insert: inserted=%v err=%v", inserted, err)
	}
	inserted, err = backend.PutIfAbsent(ctx, "amber/b", []byte("replacement"))
	if err != nil || inserted {
		t.Fatalf("duplicate insert: inserted=%v err=%v", inserted, err)
	}
	got, err := backend.Get(ctx, "amber/b")
	if err != nil || string(got) != "value" {
		t.Fatalf("stored value=%q err=%v", got, err)
	}
	keys, err := backend.List(ctx, "amber/")
	if err != nil || len(keys) != 2 || keys[0] != "amber/a" || keys[1] != "amber/b" {
		t.Fatalf("keys=%v err=%v", keys, err)
	}
}

func TestKeyValueStoreRejectsNilBackendAndCancelledContext(t *testing.T) {
	if _, err := NewKeyValueStore(nil, "test/amber"); !errors.Is(err, amber.ErrInvalidTransition) {
		t.Fatalf("expected invalid backend error, got %v", err)
	}
	store, err := NewKeyValueStore(NewMapKeyValueBackend(), "")
	if err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Put(cancelled, amber.Provenance{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}
