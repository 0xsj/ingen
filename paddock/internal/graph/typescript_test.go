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

func TestTypeScriptAdapterResolvesWorkspaceAliases(t *testing.T) {
	repoRoot := repositoryRoot(t)
	root := filepath.Join(repoRoot, "paddock", "examples", "services", "monorepo-ts", "good")
	loaded, err := graph.LoadTypeScript(root)
	if err != nil {
		t.Fatal(err)
	}

	wantEdges := []struct {
		from string
		to   string
	}{
		{from: "packages/orders/src/domain/order.ts", to: "packages/shared/src/id.ts"},
		{from: "packages/orders/src/application/service.ts", to: "packages/orders/src/domain/order.ts"},
		{from: "packages/orders/src/index.ts", to: "packages/orders/src/application/service.ts"},
	}
	for _, want := range wantEdges {
		if !hasEdge(loaded, want.from, want.to, "internal") {
			t.Fatalf("workspace alias was not resolved from %s to %s: %#v", want.from, want.to, loaded.Edges)
		}
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

func TestTypeScriptAdapterResolvesAliasesRelativeToExtendedConfig(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".svelte-kit", "types"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src", "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".svelte-kit", "tsconfig.json"), []byte(`{
  "compilerOptions": {
    "paths": {
      "$lib": ["../src/lib"],
      "$lib/*": ["../src/lib/*"]
    }
  }
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tsconfig.json"), []byte(`{
  "extends": "./.svelte-kit/tsconfig.json"
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "main.ts"), []byte(`import { value } from "$lib/value";
export const main = value;
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "lib", "value.ts"), []byte("export const value = 1;\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := graph.LoadTypeScript(root)
	if err != nil {
		t.Fatal(err)
	}
	if !hasEdge(loaded, "src/main.ts", "src/lib/value.ts", "internal") {
		t.Fatalf("alias from an extended config was not resolved relative to that config: %#v", loaded.Edges)
	}
}

func TestTypeScriptAdapterLoadsSvelteSourcesAndResolvesSvelteImports(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src", "routes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src", "lib", "components"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tsconfig.json"), []byte(`{
  "compilerOptions": {
    "baseUrl": ".",
    "paths": {"$lib/*": ["src/lib/*"]}
  }
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "routes", "+page.svelte"), []byte(`<script lang="ts">
  import Card from "$lib/components/Card.svelte";
</script>

<Card />
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "lib", "components", "Card.svelte"), []byte(`<script lang="ts">
  import { value } from "./value";
</script>

<p>{value}</p>
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "lib", "components", "value.ts"), []byte("export const value = 1;\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := graph.LoadTypeScript(root)
	if err != nil {
		t.Fatal(err)
	}
	if !hasPackage(loaded, "src/routes/+page.svelte") || !hasPackage(loaded, "src/lib/components/Card.svelte") {
		t.Fatalf("Svelte sources were not included in the graph: %#v", loaded.Packages)
	}
	if !hasEdge(loaded, "src/routes/+page.svelte", "src/lib/components/Card.svelte", "internal") {
		t.Fatalf("Svelte alias import was not resolved: %#v", loaded.Edges)
	}
	if !hasEdge(loaded, "src/lib/components/Card.svelte", "src/lib/components/value.ts", "internal") {
		t.Fatalf("Svelte script import was not resolved: %#v", loaded.Edges)
	}
}

func TestTypeScriptAdapterSkipsFrameworkBuildOutput(t *testing.T) {
	root := t.TempDir()
	for _, directory := range []string{
		filepath.Join(root, ".next", "server"),
		filepath.Join(root, ".nuxt"),
		filepath.Join(root, ".svelte-kit"),
		filepath.Join(root, ".turbo"),
		filepath.Join(root, ".vercel", "output"),
		filepath.Join(root, ".output"),
		filepath.Join(root, "storybook-static"),
		filepath.Join(root, "coverage"),
		filepath.Join(root, "node_modules", "generated-package"),
	} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "generated.ts"), []byte("export const generated = true;\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "main.ts"), []byte("export const main = true;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "lib", "coverage"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "lib", "coverage", "source.ts"), []byte("export const source = true;\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := graph.LoadTypeScript(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Packages) != 2 || !hasPackage(loaded, "main.ts") || !hasPackage(loaded, "lib/coverage/source.ts") {
		t.Fatalf("framework build output leaked into graph: %#v", loaded.Packages)
	}
}

func TestTypeScriptAdapterResolvesDottedBasenamesAndClassifiesAssets(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.ts"), []byte(`import { variants } from "./button.variants";
import styles from "./button.module.css";
import "./missing";
import "./.next/dev/types/routes.d.ts";
export { variants, styles };
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "button.variants.ts"), []byte("export const variants = {};\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "button.module.css"), []byte(".button {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := graph.LoadTypeScript(root)
	if err != nil {
		t.Fatal(err)
	}
	if !hasEdge(loaded, "main.ts", "button.variants.ts", "internal") {
		t.Fatalf("dotted basename import did not resolve: %#v", loaded.Edges)
	}
	if !hasEdge(loaded, "main.ts", "./button.module.css", "asset") {
		t.Fatalf("asset import was not classified as asset: %#v", loaded.Edges)
	}
	if !hasEdge(loaded, "main.ts", "./missing", "unresolved") {
		t.Fatalf("missing source import was not left unresolved: %#v", loaded.Edges)
	}
	if !hasEdge(loaded, "main.ts", "./.next/dev/types/routes.d.ts", "generated") {
		t.Fatalf("framework-generated import was not classified as generated: %#v", loaded.Edges)
	}
}

func hasPackage(graph *model.Graph, path string) bool {
	for _, pkg := range graph.Packages {
		if pkg.RelPath == path {
			return true
		}
	}
	return false
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
