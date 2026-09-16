package graph_test

import (
	"context"
	"encoding/json"
	"errors"
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

func TestFilterGraphAppliesIncludeExcludeScope(t *testing.T) {
	input := &model.Graph{
		ModulePath: "example",
		Packages: []*model.Package{
			{ImportPath: "example/app", RelPath: "src/app/main.ts"},
			{ImportPath: "example/domain", RelPath: "src/domain/order.ts"},
			{ImportPath: "example/example", RelPath: "src/lessons/examples/sample.ts"},
			{ImportPath: "example/tool", RelPath: "tools/check.ts"},
		},
		Edges: []*model.Edge{
			{FromImportPath: "example/app", ToImportPath: "example/domain", TargetKind: "internal", Kind: "import"},
			{FromImportPath: "example/app", ToImportPath: "example/example", TargetKind: "internal", Kind: "import"},
			{FromImportPath: "example/domain", ToImportPath: "net/http", TargetKind: "external", Kind: "import"},
			{FromImportPath: "example/tool", ToImportPath: "example/domain", TargetKind: "internal", Kind: "import"},
		},
	}

	filtered, err := graph.FilterGraph(input, []string{"src/**/*.ts"}, []string{"src/**/examples/**"})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Packages) != 2 || !hasPackage(filtered, "src/app/main.ts") || !hasPackage(filtered, "src/domain/order.ts") {
		t.Fatalf("unexpected scoped packages: %#v", filtered.Packages)
	}
	if len(filtered.Edges) != 2 || !hasGraphEdge(filtered, "example/app", "example/domain") || !hasGraphEdge(filtered, "example/domain", "net/http") {
		t.Fatalf("unexpected scoped edges: %#v", filtered.Edges)
	}
}

func TestFilterGraphRejectsEmptyScope(t *testing.T) {
	input := &model.Graph{ModulePath: "example", Packages: []*model.Package{{ImportPath: "example/app", RelPath: "src/app.ts"}}}
	if _, err := graph.FilterGraph(input, []string{"missing/**"}, nil); err == nil || !strings.Contains(err.Error(), "selected no source units") {
		t.Fatalf("scope error = %v, want no source units", err)
	}
}

func TestLoadGoKeepsToolDiagnosticsOutOfJSONStream(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example/service\n\ngo 1.23\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	goStub := filepath.Join(bin, "go")
	stub := "#!/bin/sh\n" +
		"echo module-cache-warning >&2\n" +
		"printf '{\"ImportPath\":\"example/service\",\"Dir\":\"%s\",\"GoFiles\":[\"main.go\"],\"Imports\":[]}\\n' \"$PWD\"\n"
	if err := os.WriteFile(goStub, []byte(stub), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	loaded, err := graph.LoadGo(root)
	if err != nil {
		t.Fatalf("LoadGo rejected valid JSON with stderr diagnostics: %v", err)
	}
	if loaded.ModulePath != "example/service" || len(loaded.Packages) != 1 || loaded.Packages[0].ImportPath != "example/service" {
		t.Fatalf("unexpected graph: %#v", loaded)
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
		Adapter:      &graph.AdapterMetadata{Kind: "builtin", Name: "source-parser", Version: "1.0.0"},
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
		Include:  []string{"src/**"},
		Exclude:  []string{"src/generated/**"},
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
	if received.Adapter == nil || received.Adapter.Kind != "external" || received.Adapter.Name != "" || received.Adapter.Version != "" || received.Adapter.Executable != scriptPath || received.Adapter.ResolvedExecutable != scriptPath || received.Adapter.ExecutableSHA256 == "" || received.Adapter.ArgsSHA256 == "" {
		t.Fatalf("external graph omitted invocation metadata: %#v", received.Adapter)
	}

	document.Adapter = &graph.AdapterMetadata{Kind: "external", Name: "fixture-rust", Version: "2.3.4"}
	data, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(responsePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, received, err = graph.LoadExternal(context.Background(), scriptPath, []string{requestPath, responsePath}, graph.Request{
		Schema:   graph.RequestSchema,
		Language: "rust",
		Unit:     "file",
		Root:     root,
		Include:  []string{"src/**"},
		Exclude:  []string{"src/generated/**"},
		RequiredCapabilities: graph.Capabilities{
			SourceUnits: []string{"file"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if received.Adapter == nil || received.Adapter.Kind != "external" || received.Adapter.Name != "fixture-rust" || received.Adapter.Version != "2.3.4" || received.Adapter.Executable != scriptPath || received.Adapter.ResolvedExecutable != scriptPath || received.Adapter.ExecutableSHA256 == "" || received.Adapter.ArgsSHA256 == "" {
		t.Fatalf("external graph did not preserve adapter identity: %#v", received.Adapter)
	}
	requestData, err := os.ReadFile(requestPath)
	if err != nil {
		t.Fatal(err)
	}
	var receivedRequest graph.Request
	if err := json.Unmarshal(requestData, &receivedRequest); err != nil {
		t.Fatal(err)
	}
	if receivedRequest.Schema != graph.RequestSchema || receivedRequest.Root != root || receivedRequest.RequiredCapabilities.SourceUnits[0] != "file" || receivedRequest.RequiredCapabilities.EdgeKinds == nil || len(receivedRequest.Include) != 1 || receivedRequest.Include[0] != "src/**" || len(receivedRequest.Exclude) != 1 || receivedRequest.Exclude[0] != "src/generated/**" {
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

func TestValidationDocumentClassifiesAdapterFailures(t *testing.T) {
	tests := []struct {
		name string
		err  string
		want string
	}{
		{name: "process", err: `external graph adapter "adapter" exited with status 7`, want: "process-failure"},
		{name: "graph", err: "external graph adapter returned invalid graph: graph document edge kind", want: "invalid-graph"},
		{name: "source unit", err: `adapter does not support required source unit "package"`, want: "source-unit-mismatch"},
		{name: "capability", err: "external graph adapter capability negotiation failed", want: "capability-mismatch"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := graph.ValidationDocumentForError("adapter-validate", "/workspace", "rust", "file", "adapter", errors.New(test.err))
			if document.Schema != graph.ValidationSchema || document.Valid || len(document.Errors) != 1 || document.Errors[0].Code != test.want {
				t.Fatalf("validation document = %#v, want code %q", document, test.want)
			}
		})
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

func hasGraphEdge(input *model.Graph, from, to string) bool {
	for _, edge := range input.Edges {
		if edge.FromImportPath == from && edge.ToImportPath == to {
			return true
		}
	}
	return false
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
