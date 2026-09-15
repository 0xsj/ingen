package amberstorage

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	amber "github.com/0xsj/ingen/amber"
)

func TestStoreContract(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		runStoreContract(t, func(*testing.T) Store {
			return NewMemoryStore()
		})
	})
	t.Run("file", func(t *testing.T) {
		runStoreContract(t, func(t *testing.T) Store {
			store, err := NewFileStore(filepath.Join(t.TempDir(), "provenance.json"))
			if err != nil {
				t.Fatal(err)
			}
			return store
		})
	})
	t.Run("key-value", func(t *testing.T) {
		runStoreContract(t, func(t *testing.T) Store {
			store, err := NewKeyValueStore(NewMapKeyValueBackend(), "contract/provenance")
			if err != nil {
				t.Fatal(err)
			}
			return store
		})
	})
}

// runStoreContract is the shared semantic suite for every Store
// implementation. Values are deliberately inserted child-first so an
// implementation cannot accidentally pass by returning insertion order.
func runStoreContract(t *testing.T, newStore func(*testing.T) Store) {
	t.Helper()
	store := newStore(t)
	ctx := context.Background()

	root, err := amber.Start()
	if err != nil {
		t.Fatal(err)
	}
	retry, err := root.Retry()
	if err != nil {
		t.Fatal(err)
	}
	child, err := root.Child(amber.ChildOptions{Origin: amber.OriginIncoming})
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Put(ctx, amber.Provenance{}); !errors.Is(err, amber.ErrInvalidProvenance) {
		t.Fatalf("invalid provenance error = %v", err)
	}
	for _, value := range []amber.Provenance{child, retry, root} {
		if err := store.Put(ctx, value); err != nil {
			t.Fatalf("put %s: %v", value.ExecutionID(), err)
		}
	}
	if err := store.Put(ctx, root); err != nil {
		t.Fatalf("same value should be idempotent: %v", err)
	}

	stored, err := store.Get(ctx, root.ExecutionID())
	if err != nil {
		t.Fatalf("get root: %v", err)
	}
	if stored.ExecutionID() != root.ExecutionID() {
		t.Fatalf("stored execution ID = %s, want %s", stored.ExecutionID(), root.ExecutionID())
	}

	conflicting, err := contractConflictValue(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, conflicting); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting value error = %v", err)
	}
	stored, err = store.Get(ctx, root.ExecutionID())
	if err != nil || stored.Origin() != root.Origin() {
		t.Fatalf("conflict overwrote root: origin=%s err=%v", stored.Origin(), err)
	}

	workHistory, err := store.ListByWorkID(ctx, root.WorkID())
	if err != nil {
		t.Fatalf("work history: %v", err)
	}
	assertStoreHistory(t, "work", workHistory, root.ExecutionID(), retry.ExecutionID())
	causalHistory, err := store.ListByCausation(ctx, "execution", root.ExecutionID())
	if err != nil {
		t.Fatalf("causation history: %v", err)
	}
	assertStoreHistory(t, "causation", causalHistory, retry.ExecutionID(), child.ExecutionID())
	correlatedHistory, err := store.ListByCorrelationID(ctx, root.CorrelationID())
	if err != nil {
		t.Fatalf("correlation history: %v", err)
	}
	assertStoreHistory(t, "correlation", correlatedHistory, root.ExecutionID(), retry.ExecutionID(), child.ExecutionID())

	missingID := amber.ID("99999999-9999-4999-8999-999999999999")
	if _, err := store.Get(ctx, missingID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing value error = %v", err)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := store.Put(cancelled, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled put error = %v", err)
	}
	if _, err := store.Get(cancelled, root.ExecutionID()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled get error = %v", err)
	}
	if _, err := store.ListByWorkID(cancelled, root.WorkID()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled work query error = %v", err)
	}
}

func assertStoreHistory(t *testing.T, name string, history []amber.Provenance, expected ...amber.ID) {
	t.Helper()
	if len(history) != len(expected) {
		t.Fatalf("%s history length = %d, want %d", name, len(history), len(expected))
	}
	for index, expectedID := range expected {
		if history[index].ExecutionID() != expectedID {
			t.Fatalf("%s history[%d] = %s, want %s", name, index, history[index].ExecutionID(), expectedID)
		}
	}
}

func contractConflictValue(provenance amber.Provenance) (amber.Provenance, error) {
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
