package main

import (
	"context"
	"testing"

	amber "github.com/0xsj/ingen/amber"
	amberstorage "github.com/0xsj/ingen/amber/adapters/storage"
)

func TestProcessJobStoresChildAndRetry(t *testing.T) {
	store := amberstorage.NewMemoryStore()
	parent, err := amber.Start()
	if err != nil {
		t.Fatal(err)
	}

	result, err := processJob(context.Background(), store, parent)
	if err != nil {
		t.Fatal(err)
	}
	if result.child.Origin() != amber.OriginIncoming {
		t.Fatalf("child origin = %q, want %q", result.child.Origin(), amber.OriginIncoming)
	}
	if result.retry.Origin() != amber.OriginRetry {
		t.Fatalf("retry origin = %q, want %q", result.retry.Origin(), amber.OriginRetry)
	}
	if result.retry.WorkID() != result.child.WorkID() {
		t.Fatal("retry did not preserve logical work ID")
	}
	causation, ok := result.retry.Causation()
	if !ok || causation.ID != result.child.ExecutionID() {
		t.Fatalf("retry causation = %+v, want child execution", causation)
	}
	if len(result.history) != 2 || result.history[0].ExecutionID() != result.child.ExecutionID() || result.history[1].ExecutionID() != result.retry.ExecutionID() {
		t.Fatalf("history = %v, want child then retry", result.history)
	}
}

func TestProcessJobHonorsCancelledContextBeforeWriting(t *testing.T) {
	store := amberstorage.NewMemoryStore()
	parent, err := amber.Start()
	if err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := processJob(cancelled, store, parent); err == nil {
		t.Fatal("cancelled worker accepted a write")
	}
}
