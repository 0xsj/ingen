package graph_test

import (
	"os"
	"path/filepath"
	"testing"

	"ingen/paddock/internal/graph"
)

func TestLoadPythonResolvesInternalAndStandardLibraryImports(t *testing.T) {
	root := t.TempDir()
	writePythonFile(t, root, "app/__init__.py", "")
	writePythonFile(t, root, "app/domain/order.py", `from dataclasses import dataclass
from app.ports.repository import OrderRepository
from app.ports.missing import MissingPort

@dataclass
class Order:
    identifier: str
`)
	writePythonFile(t, root, "app/ports/repository.py", "from typing import Protocol\n")
	writePythonFile(t, root, "app/adapters/memory.py", "from app.domain.order import Order\n")

	loaded, err := graph.LoadPython(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Packages) != 4 || len(loaded.Edges) != 5 {
		t.Fatalf("Python graph counts = %d packages, %d edges; want 4 packages, 5 edges", len(loaded.Packages), len(loaded.Edges))
	}
	var foundInternal, foundStandard, foundUnresolved bool
	for _, edge := range loaded.Edges {
		if edge.FromPath == "app/domain/order.py" && edge.ToPath == "app/ports/repository.py" && edge.TargetKind == "internal" {
			foundInternal = true
		}
		if edge.FromPath == "app/domain/order.py" && edge.ToImportPath == "dataclasses" && edge.TargetKind == "standard-library" {
			foundStandard = true
		}
		if edge.FromPath == "app/domain/order.py" && edge.ToImportPath == "app.ports.missing" && edge.TargetKind == "unresolved" {
			foundUnresolved = true
		}
	}
	if !foundInternal || !foundStandard || !foundUnresolved {
		for _, edge := range loaded.Edges {
			t.Logf("edge=%+v", *edge)
		}
		t.Fatalf("Python graph did not resolve expected edges")
	}
}

func writePythonFile(t *testing.T, root, rel, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
