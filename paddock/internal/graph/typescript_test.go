package graph_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"ingen/paddock/internal/graph"
	"ingen/paddock/internal/model"
)

func TestTypeScriptAdapterResolvesAliasesAndReportsUnresolvedEdges(t *testing.T) {
	repoRoot := repositoryRoot(t)
	good, err := graph.LoadTypeScript(filepath.Join(repoRoot, "paddock", "examples", "services", "feature-sliced-ts", "good"))
	if err != nil {
		t.Fatal(err)
	}
	if !hasEdge(good, "src/entities/order.ts", "src/shared/config.ts", "internal") {
		t.Fatalf("path alias was not resolved as an internal edge: %#v", good.Edges)
	}

	violating, err := graph.LoadTypeScript(filepath.Join(repoRoot, "paddock", "examples", "services", "feature-sliced-ts", "violating"))
	if err != nil {
		t.Fatal(err)
	}
	if !hasEdge(violating, "src/pages/orders.ts", "../missing/widget", "unresolved") {
		t.Fatalf("missing relative import was not marked unresolved: %#v", violating.Edges)
	}
}

func TestTypeScriptAdapterLoadsJSONCAndExtendedConfigs(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "config", "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "tsconfig.base.json"), []byte(`{
  // The base config lives one directory below the project root.
  "compilerOptions": {
    "baseUrl": "./",
    "paths": {
      "@/*": ["src/*"],
    },
  },
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tsconfig.json"), []byte(`{
  "extends": "./config/tsconfig.base.json",
  // JSONC and trailing commas are accepted.
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "src", "main.ts"), []byte(`import { value } from "@/value";
export const main = value;
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "src", "value.ts"), []byte(`export const value = 1;
`), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := graph.LoadTypeScript(root)
	if err != nil {
		t.Fatal(err)
	}
	if !hasEdge(loaded, "config/src/main.ts", "config/src/value.ts", "internal") {
		t.Fatalf("extended JSONC config did not resolve alias: %#v", loaded.Edges)
	}
}

func hasEdge(edgesGraph *model.Graph, from, to, kind string) bool {
	for _, edge := range edgesGraph.Edges {
		if edge.FromPath == from && edge.TargetKind == kind && (edge.ToPath == to || edge.ToImportPath == to) {
			return true
		}
	}
	return false
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../.."))
}
