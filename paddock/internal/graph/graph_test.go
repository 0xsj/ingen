package graph_test

import (
	"testing"

	"ingen/paddock/internal/graph"
	"ingen/paddock/internal/model"
)

func TestDefaultRegistryExposesLanguageCapabilities(t *testing.T) {
	registry := graph.DefaultRegistry()
	for _, test := range []struct {
		language string
		unit     string
	}{
		{language: "go", unit: "package"},
		{language: "typescript", unit: "file"},
		{language: "python", unit: "file"},
	} {
		adapter, err := registry.Lookup(test.language)
		if err != nil {
			t.Fatal(err)
		}
		if adapter.Language() != test.language {
			t.Fatalf("adapter language = %q, want %q", adapter.Language(), test.language)
		}
		if len(adapter.Capabilities().SourceUnits) != 1 || adapter.Capabilities().SourceUnits[0] != test.unit {
			t.Fatalf("%s capabilities = %#v, want source unit %q", test.language, adapter.Capabilities(), test.unit)
		}
	}
}

func TestRegistryRejectsDuplicateLanguages(t *testing.T) {
	adapter := testAdapter{language: "example"}
	if _, err := graph.NewRegistry(adapter, adapter); err == nil {
		t.Fatal("NewRegistry accepted duplicate adapter language")
	}
}

func TestStableCopySortsGraphWithoutMutatingInput(t *testing.T) {
	first := &model.Package{ImportPath: "example/b", RelPath: "b"}
	second := &model.Package{ImportPath: "example/a", RelPath: "a"}
	input := &model.Graph{
		Packages: []*model.Package{first, second},
		Edges: []*model.Edge{
			{FromPath: "b", File: "b.go", Line: 2, ToImportPath: "example/a"},
			{FromPath: "a", File: "a.go", Line: 1, ToImportPath: "example/b"},
		},
	}
	stable := graph.StableCopy(input)
	if stable.Packages[0] != second || stable.Edges[0].FromPath != "a" {
		t.Fatalf("stable graph was not sorted: %#v", stable)
	}
	if input.Packages[0] != first || input.Edges[0].FromPath != "b" {
		t.Fatalf("StableCopy mutated input: %#v", input)
	}
}

type testAdapter struct {
	language string
}

func (a testAdapter) Language() string {
	return a.language
}

func (testAdapter) Capabilities() graph.Capabilities {
	return graph.Capabilities{SourceUnits: []string{"test"}}
}

func (testAdapter) Load(graph.LoadRequest) (*model.Graph, error) {
	return &model.Graph{}, nil
}
