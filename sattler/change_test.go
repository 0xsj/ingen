package sattler

import "testing"

func TestNewChangeProvidesStableFieldKey(t *testing.T) {
	change := NewChange("verdict", "status", "passed", "failed")
	if change.ID != "verdict.status" {
		t.Fatalf("change ID = %q, want verdict.status", change.ID)
	}
	if change.StableID() != change.ID {
		t.Fatalf("StableID() = %q, want stored ID %q", change.StableID(), change.ID)
	}

	manual := Change{Category: "context", Field: "source.root"}
	if manual.StableID() != "context.source.root" {
		t.Fatalf("manual StableID() = %q, want context.source.root", manual.StableID())
	}
}
