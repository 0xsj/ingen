package scaffold_test

import (
	"os"
	"strings"
	"testing"

	"ingen/paddock/internal/checker"
	"ingen/paddock/internal/model"
	"ingen/paddock/internal/policy"
	"ingen/paddock/internal/scaffold"
)

func TestGenerateLayeredDraftIsLoadableAndNonBlocking(t *testing.T) {
	contents, err := scaffold.Generate(scaffold.Options{
		Language: "go",
		Template: "layered",
		Root:     ".",
		Graph: &model.Graph{
			ModulePath: "example.com/service",
			Packages: []*model.Package{
				{ImportPath: "example.com/service/cmd/api", RelPath: "cmd/api"},
				{ImportPath: "example.com/service/internal/domain", RelPath: "internal/domain"},
				{ImportPath: "example.com/service/internal/service", RelPath: "internal/service"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "# Paddock starter policy — DRAFT") || !strings.Contains(string(contents), "severity: warning") {
		t.Fatalf("draft markers missing:\n%s", contents)
	}
	path := t.TempDir() + "/paddock.yaml"
	if err := writeFile(path, contents); err != nil {
		t.Fatal(err)
	}
	loaded, err := policy.Load(path)
	if err != nil {
		t.Fatalf("generated policy is invalid: %v\n%s", err, contents)
	}
	if loaded.Source.Language != "go" || len(loaded.Components) != 3 || len(loaded.Rules) < 2 {
		t.Fatalf("unexpected generated policy: %#v", loaded)
	}
	for _, rule := range loaded.Rules {
		if rule.Severity != "warning" {
			t.Fatalf("generated rule severity = %q, want warning", rule.Severity)
		}
	}
}

func TestGenerateRejectsUnsupportedTemplate(t *testing.T) {
	_, err := scaffold.Generate(scaffold.Options{
		Language: "typescript",
		Template: "hexagonal",
		Graph:    &model.Graph{Packages: []*model.Package{{RelPath: "src/app.ts"}}},
	})
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("Generate error = %v, want unsupported template error", err)
	}
}

func TestGeneratePythonDraftHandlesPackageInitializers(t *testing.T) {
	contents, err := scaffold.Generate(scaffold.Options{
		Language: "python",
		Template: "layered",
		Root:     ".",
		Graph: &model.Graph{
			ModulePath: "example-python",
			Packages: []*model.Package{
				{ImportPath: "example-python/app", RelPath: "app/__init__.py"},
				{ImportPath: "example-python/app/domain", RelPath: "app/domain/__init__.py"},
				{ImportPath: "example-python/app/domain/order", RelPath: "app/domain/order.py"},
				{ImportPath: "example-python/app/ports/repository", RelPath: "app/ports/repository.py"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "language: python") ||
		!strings.Contains(string(contents), "unit: file") ||
		!strings.Contains(string(contents), "match: app/__init__.py") ||
		!strings.Contains(string(contents), "match: app/domain/**") {
		t.Fatalf("Python draft does not classify package initializers and directories:\n%s", contents)
	}
	path := t.TempDir() + "/paddock.yaml"
	if err := writeFile(path, contents); err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Load(path); err != nil {
		t.Fatalf("generated Python policy is invalid: %v\n%s", err, contents)
	}
}

func TestGenerateSupportsGenericExternalFileLanguage(t *testing.T) {
	contents, err := scaffold.Generate(scaffold.Options{
		Language: "rust",
		Unit:     "file",
		Template: "layered",
		Root:     ".",
		Graph: &model.Graph{
			ModulePath: "example-rust",
			Packages: []*model.Package{
				{ImportPath: "example-rust/src/domain", RelPath: "src/domain.rs"},
				{ImportPath: "example-rust/src/http", RelPath: "src/http.rs"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "language: rust") ||
		!strings.Contains(string(contents), "unit: file") ||
		!strings.Contains(string(contents), "match: src/**") {
		t.Fatalf("generic external file draft is incomplete:\n%s", contents)
	}
	if _, err := policy.Load(writePolicy(t, contents)); err != nil {
		t.Fatalf("generated external policy is invalid: %v\n%s", err, contents)
	}
}

func TestGenerateMakesNestedParentComponentsExact(t *testing.T) {
	graph := &model.Graph{
		ModulePath: "example.com/service",
		Packages: []*model.Package{
			{ImportPath: "example.com/service/internal/orders", RelPath: "internal/orders"},
			{ImportPath: "example.com/service/internal/orders/app", RelPath: "internal/orders/app"},
			{ImportPath: "example.com/service/internal/orders/domain", RelPath: "internal/orders/domain"},
		},
	}
	contents, err := scaffold.Generate(scaffold.Options{
		Language: "go",
		Template: "layered",
		Root:     t.TempDir(),
		Graph:    graph,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "match: internal/orders\n") {
		t.Fatalf("parent context should be exact when it has nested components:\n%s", contents)
	}
	if !strings.Contains(string(contents), "match: internal/orders/app/**") ||
		!strings.Contains(string(contents), "match: internal/orders/domain/**") {
		t.Fatalf("nested components should remain recursive:\n%s", contents)
	}

	path := writePolicy(t, contents)
	loaded, err := policy.Load(path)
	if err != nil {
		t.Fatalf("generated policy is invalid: %v\n%s", err, contents)
	}
	result, err := checker.CheckGraph(t.TempDir(), path, loaded, graph)
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range result.Findings {
		if strings.Contains(finding.Message, "matches multiple components") ||
			strings.Contains(finding.Message, "must match exactly one component") {
			t.Fatalf("generated policy has a coverage defect: %#v\n%s", finding, contents)
		}
	}
}

func TestGenerateQuotesRootGlobForYAML(t *testing.T) {
	contents, err := scaffold.Generate(scaffold.Options{
		Language: "typescript",
		Unit:     "file",
		Template: "feature-sliced",
		Root:     t.TempDir(),
		Graph: &model.Graph{
			ModulePath: "example-ui",
			Packages: []*model.Package{
				{ImportPath: "example-ui/main.ts", RelPath: "main.ts"},
				{ImportPath: "example-ui/src/app.ts", RelPath: "src/app.ts"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "match: \"*\"\n") {
		t.Fatalf("root file glob should be quoted and bounded in YAML:\n%s", contents)
	}
	if _, err := policy.Load(writePolicy(t, contents)); err != nil {
		t.Fatalf("generated root-file policy is invalid: %v\n%s", err, contents)
	}
}

func writePolicy(t *testing.T, contents []byte) string {
	t.Helper()
	path := t.TempDir() + "/paddock.yaml"
	if err := writeFile(path, contents); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeFile(path string, contents []byte) error {
	return os.WriteFile(path, contents, 0o600)
}
