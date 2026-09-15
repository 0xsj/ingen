package amberstorage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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

func TestFileStorePersistsAndPreservesConflicts(t *testing.T) {
	root, err := amber.Start()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "provenance.json")
	store, err := NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Put(ctx, root); err != nil {
		t.Fatal(err)
	}

	// A new instance demonstrates that the record is read from disk rather
	// than only from the first instance's process memory.
	reopened, err := NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := reopened.Get(ctx, root.ExecutionID())
	if err != nil || stored.ExecutionID() != root.ExecutionID() {
		t.Fatalf("reopened file store could not read value: %v", err)
	}
	if err := reopened.Put(ctx, root); err != nil {
		t.Fatalf("same value should be idempotent after reopen: %v", err)
	}
	history, err := reopened.ListByWorkID(ctx, root.WorkID())
	if err != nil || len(history) != 1 || history[0].ExecutionID() != root.ExecutionID() {
		t.Fatalf("reopened file store returned wrong history: len=%d err=%v", len(history), err)
	}

	data := mustJSON(root)
	data = replaceJSONField(data, "origin", "incoming")
	conflictingValue, err := amber.FromJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.Put(ctx, conflictingValue); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected persisted conflict, got %v", err)
	}
}

func TestFileStoreRejectsEmptyPath(t *testing.T) {
	if _, err := NewFileStore(""); !errors.Is(err, amber.ErrInvalidTransition) {
		t.Fatalf("expected invalid path error, got %v", err)
	}
}

func TestFileStoreRejectsUnsupportedSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provenance.json")
	if err := os.WriteFile(path, []byte(`{"version":99,"records":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(context.Background(), amber.ID("99999999-9999-4999-8999-999999999999")); !errors.Is(err, ErrUnsupportedSchemaVersion) {
		t.Fatalf("expected unsupported schema version error, got %v", err)
	}
}

type storageConformanceFixture struct {
	Version            int                `json:"version"`
	Value              json.RawMessage    `json:"value"`
	ConflictValue      json.RawMessage    `json:"conflict_value"`
	MissingExecutionID amber.ID           `json:"missing_execution_id"`
	History            storageHistory     `json:"history"`
	Causation          storageCausation   `json:"causation"`
	Correlation        storageCorrelation `json:"correlation"`
}

type storageHistory struct {
	WorkID               amber.ID          `json:"work_id"`
	Values               []json.RawMessage `json:"values"`
	ExpectedExecutionIDs []amber.ID        `json:"expected_execution_ids"`
}

type storageCausation struct {
	Kind                 string     `json:"kind"`
	ID                   amber.ID   `json:"id"`
	ExpectedExecutionIDs []amber.ID `json:"expected_execution_ids"`
}

type storageCorrelation struct {
	ID                   amber.ID   `json:"id"`
	ExpectedExecutionIDs []amber.ID `json:"expected_execution_ids"`
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
	historyStore := NewMemoryStore()
	for _, rawValue := range fixture.History.Values {
		provenance, err := amber.FromJSON(rawValue)
		if err != nil {
			t.Fatalf("decode history value: %v", err)
		}
		if err := historyStore.Put(context.Background(), provenance); err != nil {
			t.Fatalf("put history value: %v", err)
		}
	}
	history, err := historyStore.ListByWorkID(context.Background(), fixture.History.WorkID)
	if err != nil {
		t.Fatalf("list history: %v", err)
	}
	if len(history) != len(fixture.History.ExpectedExecutionIDs) {
		t.Fatalf("history length=%d, want %d", len(history), len(fixture.History.ExpectedExecutionIDs))
	}
	for index, expectedID := range fixture.History.ExpectedExecutionIDs {
		if history[index].ExecutionID() != expectedID {
			t.Fatalf("history[%d]=%s, want %s", index, history[index].ExecutionID(), expectedID)
		}
	}
	causal, err := historyStore.ListByCausation(context.Background(), fixture.Causation.Kind, fixture.Causation.ID)
	if err != nil {
		t.Fatalf("list causation: %v", err)
	}
	if len(causal) != len(fixture.Causation.ExpectedExecutionIDs) {
		t.Fatalf("causal history length=%d, want %d", len(causal), len(fixture.Causation.ExpectedExecutionIDs))
	}
	for index, expectedID := range fixture.Causation.ExpectedExecutionIDs {
		if causal[index].ExecutionID() != expectedID {
			t.Fatalf("causal[%d]=%s, want %s", index, causal[index].ExecutionID(), expectedID)
		}
	}
	correlated, err := historyStore.ListByCorrelationID(context.Background(), fixture.Correlation.ID)
	if err != nil {
		t.Fatalf("list correlation: %v", err)
	}
	if len(correlated) != len(fixture.Correlation.ExpectedExecutionIDs) {
		t.Fatalf("correlation history length=%d, want %d", len(correlated), len(fixture.Correlation.ExpectedExecutionIDs))
	}
	for index, expectedID := range fixture.Correlation.ExpectedExecutionIDs {
		if correlated[index].ExecutionID() != expectedID {
			t.Fatalf("correlated[%d]=%s, want %s", index, correlated[index].ExecutionID(), expectedID)
		}
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
