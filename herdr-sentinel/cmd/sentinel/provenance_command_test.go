package main

import (
	"os"
	"path/filepath"
	"testing"

	"ingen/herdr-sentinel/internal/provenance"
)

func TestProvenanceExecuteReturnsErrorWhenReceiptClaimWasTampered(t *testing.T) {
	root := t.TempDir()
	if _, _, err := provenance.Start(root, ".ingen/provenance/root.json"); err != nil {
		t.Fatal(err)
	}
	receipt := filepath.Join(root, ".ingen/provenance/execution.json")
	t.Setenv("SENTINEL_PROVENANCE_HELPER", "tamper-receipt")
	t.Setenv("SENTINEL_PROVENANCE_RECEIPT", receipt)
	child := []string{os.Args[0], "-test.run=^TestSentinelProvenanceCLIHelper$"}
	args := []string{"--root", root, "--parent", ".ingen/provenance/root.json", "--output", ".ingen/provenance/child.json", "--receipt", ".ingen/provenance/execution.json", "--operation", "test-operation", "--"}
	args = append(args, child...)
	if got := provenanceExecuteCommand(args); got != 2 {
		t.Fatalf("publication failure CLI exit = %d, want 2", got)
	}
}

func TestProvenanceExecuteRequiresExplicitSeparator(t *testing.T) {
	if got := provenanceExecuteCommand([]string{"--root", t.TempDir(), "--parent", "parent.json", "--output", "child.json", "--receipt", "receipt.json", "--operation", "op", "/bin/true"}); got != 2 {
		t.Fatalf("missing -- separator CLI exit = %d, want 2", got)
	}
}

func TestSentinelProvenanceCLIHelper(t *testing.T) {
	if os.Getenv("SENTINEL_PROVENANCE_HELPER") != "tamper-receipt" {
		return
	}
	if err := os.Remove(os.Getenv("SENTINEL_PROVENANCE_RECEIPT")); err != nil {
		os.Exit(9)
	}
}
