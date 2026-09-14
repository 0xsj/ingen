package scaffold_test

import (
	"os"
	"strings"
	"testing"

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

func writeFile(path string, contents []byte) error {
	return os.WriteFile(path, contents, 0o600)
}
