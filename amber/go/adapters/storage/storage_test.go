package amberstorage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	amber "github.com/0xsj/ingen/amber"
)

func TestMemoryStoreIsIdempotentAndAppendOnly(t *testing.T) {
	store := NewMemoryStore()
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

	// Build a distinct value with the same execution ID by changing a
	// descriptive field only; the store must reject the overwrite.
	data := mustJSON(root)
	data = replaceJSONField(data, "origin", "incoming")
	conflictingValue, err := amber.FromJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, conflictingValue); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}

	stored, err := store.Get(ctx, root.ExecutionID())
	if err != nil || stored.ExecutionID() != root.ExecutionID() {
		t.Fatalf("stored value could not be read: %v", err)
	}
}

func TestMemoryStoreReportsMissingAndCancelled(t *testing.T) {
	store := NewMemoryStore()
	if _, err := store.Get(context.Background(), amber.ID("11111111-1111-4111-8111-111111111111")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Put(cancelled, amber.Provenance{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

type storageConformanceFixture struct {
	Version            int             `json:"version"`
	Value              json.RawMessage `json:"value"`
	ConflictValue      json.RawMessage `json:"conflict_value"`
	MissingExecutionID amber.ID        `json:"missing_execution_id"`
}

func TestStorageConformanceFixture(t *testing.T) {
	data, err := os.ReadFile("../../../conformance/storage-v1.json")
	if err != nil {
		t.Fatalf("read storage fixture: %v", err)
	}
	var fixture storageConformanceFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode storage fixture: %v", err)
	}
	if fixture.Version != amber.Version {
		t.Fatalf("fixture version = %d, want %d", fixture.Version, amber.Version)
	}
	value, err := amber.FromJSON(fixture.Value)
	if err != nil {
		t.Fatalf("decode value: %v", err)
	}
	conflict, err := amber.FromJSON(fixture.ConflictValue)
	if err != nil {
		t.Fatalf("decode conflict value: %v", err)
	}
	store := NewMemoryStore()
	if err := store.Put(context.Background(), value); err != nil {
		t.Fatalf("put value: %v", err)
	}
	if err := store.Put(context.Background(), conflict); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	if _, err := store.Get(context.Background(), fixture.MissingExecutionID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func mustJSON(provenance amber.Provenance) []byte {
	data, err := provenance.MarshalJSON()
	if err != nil {
		panic(err)
	}
	return data
}

func replaceJSONField(data []byte, field, value string) []byte {
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		panic(err)
	}
	object[field] = value
	updated, err := json.Marshal(object)
	if err != nil {
		panic(err)
	}
	return updated
}
