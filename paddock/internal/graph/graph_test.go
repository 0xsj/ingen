package graph_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestLoadExternalAdapterNegotiatesRequestAndResponse(t *testing.T) {
	directory := t.TempDir()
	root := filepath.Join(directory, "source")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	requestPath := filepath.Join(directory, "request.json")
	responsePath := filepath.Join(directory, "response.json")
	scriptPath := filepath.Join(directory, "adapter.sh")
	script := "#!/bin/sh\nset -eu\ncat > \"$1\"\ncat \"$2\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	document := graph.Document{
		Schema:       graph.DocumentSchema,
		Language:     "rust",
		Unit:         "file",
		Root:         root,
		ModulePath:   "example",
		Capabilities: graph.Capabilities{SourceUnits: []string{"file"}, EdgeKinds: []string{"import"}},
		Packages: []*model.Package{{
			ImportPath: "example::domain",
			RelPath:    "src/domain.rs",
		}},
	}
	document.PackageCount = len(document.Packages)
	document.EdgeCount = len(document.Edges)
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(responsePath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, received, err := graph.LoadExternal(context.Background(), scriptPath, []string{requestPath, responsePath}, graph.Request{
		Schema:   graph.RequestSchema,
		Language: "rust",
		Unit:     "file",
		Root:     root,
		RequiredCapabilities: graph.Capabilities{
			SourceUnits: []string{"file"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Packages) != 1 || received.Language != "rust" {
		t.Fatalf("unexpected external graph: loaded=%#v document=%#v", loaded, received)
	}
	requestData, err := os.ReadFile(requestPath)
	if err != nil {
		t.Fatal(err)
	}
	var receivedRequest graph.Request
	if err := json.Unmarshal(requestData, &receivedRequest); err != nil {
		t.Fatal(err)
	}
	if receivedRequest.Schema != graph.RequestSchema || receivedRequest.Root != root || receivedRequest.RequiredCapabilities.SourceUnits[0] != "file" || receivedRequest.RequiredCapabilities.EdgeKinds == nil {
		t.Fatalf("unexpected adapter request: %#v", receivedRequest)
	}
}

func TestLoadExternalAdapterReportsProcessDiagnostics(t *testing.T) {
	directory := t.TempDir()
	scriptPath := filepath.Join(directory, "adapter.sh")
	script := "#!/bin/sh\necho adapter-broke >&2\nexit 7\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	_, _, err := graph.LoadExternal(context.Background(), scriptPath, nil, graph.Request{
		Schema:   graph.RequestSchema,
		Language: "rust",
		Root:     directory,
	})
	if err == nil || !strings.Contains(err.Error(), "status 7") || !strings.Contains(err.Error(), "adapter-broke") {
		t.Fatalf("adapter error = %v, want exit status and stderr", err)
	}
}

func TestCapabilitiesRejectMissingRequiredValues(t *testing.T) {
	err := (graph.Capabilities{SourceUnits: []string{"file"}, EdgeKinds: []string{"import"}}).Supports(
		graph.Capabilities{SourceUnits: []string{"package"}},
	)
	if err == nil || !strings.Contains(err.Error(), `required source unit "package"`) {
		t.Fatalf("capability error = %v, want missing source unit", err)
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
