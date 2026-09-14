package amber

import (
	"context"
	"testing"
)

func TestContextPropagationAndRestoration(t *testing.T) {
	root, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	child, err := root.Child()
	if err != nil {
		t.Fatal(err)
	}

	base := context.Background()
	if _, ok := ProvenanceFromContext(base); ok {
		t.Fatal("base context must not contain provenance")
	}
	rootContext, err := WithProvenance(base, root)
	if err != nil {
		t.Fatal(err)
	}
	current, ok := ProvenanceFromContext(rootContext)
	if !ok || current.ExecutionID() != root.ExecutionID() {
		t.Fatal("root provenance was not installed")
	}
	childContext, err := WithProvenance(rootContext, child)
	if err != nil {
		t.Fatal(err)
	}
	current, ok = ProvenanceFromContext(childContext)
	if !ok || current.ExecutionID() != child.ExecutionID() {
		t.Fatal("child provenance was not installed")
	}

	// Restoring means retaining the exact parent context; no mutation or global
	// cleanup is needed.
	current, ok = ProvenanceFromContext(rootContext)
	if !ok || current.ExecutionID() != root.ExecutionID() {
		t.Fatal("parent context was not preserved for restoration")
	}
}

func TestContextRejectsInvalidAndNilInputs(t *testing.T) {
	if _, err := WithProvenance(nil, Provenance{}); err == nil {
		t.Fatal("nil context must be rejected")
	}
	if _, ok := ProvenanceFromContext(nil); ok {
		t.Fatal("nil context must not contain provenance")
	}
}
