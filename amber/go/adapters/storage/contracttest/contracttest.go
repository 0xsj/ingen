// Package contracttest provides an opt-in semantic test helper for custom
// Amber Store implementations.
package contracttest

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	amber "github.com/0xsj/ingen/amber"
	amberstorage "github.com/0xsj/ingen/amber/adapters/storage"
)

// StoreFactory creates the Store under test. The testing handle lets callers
// use temporary directories or other test-scoped resources.
type StoreFactory func(testing.TB) amberstorage.Store

// RunStoreContract checks the portable append-only and history semantics that
// every custom Store implementation should preserve.
func RunStoreContract(t testing.TB, newStore StoreFactory) {
	t.Helper()
	store := newStore(t)
	ctx := context.Background()

	root, err := amber.Start()
	if err != nil {
		t.Fatalf("start root: %v", err)
	}
	retry, err := root.Retry()
	if err != nil {
		t.Fatalf("create retry: %v", err)
	}
	child, err := root.Child(amber.ChildOptions{Origin: amber.OriginIncoming})
	if err != nil {
		t.Fatalf("create child: %v", err)
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

	conflicting, err := conflictingValue(root)
	if err != nil {
		t.Fatalf("make conflict: %v", err)
	}
	if err := store.Put(ctx, conflicting); !errors.Is(err, amberstorage.ErrConflict) {
		t.Fatalf("conflicting value error = %v", err)
	}
	stored, err := store.Get(ctx, root.ExecutionID())
	if err != nil {
		t.Fatalf("get root: %v", err)
	}
	if stored.Origin() != root.Origin() {
		t.Fatalf("conflict changed stored origin to %q", stored.Origin())
	}

	workHistory, err := store.ListByWorkID(ctx, root.WorkID())
	if err != nil {
		t.Fatalf("work history: %v", err)
	}
	assertHistory(t, workHistory, root.ExecutionID(), retry.ExecutionID())
	causalHistory, err := store.ListByCausation(ctx, "execution", root.ExecutionID())
	if err != nil {
		t.Fatalf("causation history: %v", err)
	}
	assertHistory(t, causalHistory, retry.ExecutionID(), child.ExecutionID())
	correlatedHistory, err := store.ListByCorrelationID(ctx, root.CorrelationID())
	if err != nil {
		t.Fatalf("correlation history: %v", err)
	}
	assertHistory(t, correlatedHistory, root.ExecutionID(), retry.ExecutionID(), child.ExecutionID())

	missingID := amber.ID("99999999-9999-4999-8999-999999999999")
	if _, err := store.Get(ctx, missingID); !errors.Is(err, amberstorage.ErrNotFound) {
		t.Fatalf("missing value error = %v", err)
	}
	if _, err := store.ListByWorkID(ctx, missingID); err != nil {
		t.Fatalf("missing work history: %v", err)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := store.Put(cancelled, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled put error = %v", err)
	}
}

func assertHistory(t testing.TB, history []amber.Provenance, expected ...amber.ID) {
	t.Helper()
	if len(history) != len(expected) {
		t.Fatalf("history length = %d, want %d", len(history), len(expected))
	}
	for index, expectedID := range expected {
		if history[index].ExecutionID() != expectedID {
			t.Fatalf("history[%d] = %s, want %s", index, history[index].ExecutionID(), expectedID)
		}
	}
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
