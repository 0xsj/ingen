package graph

import (
	"fmt"
	"sort"

	"ingen/paddock/internal/model"
)

type LoadRequest struct {
	Root  string
	Unit  string
	Roots []string
}

type Capabilities struct {
	SourceUnits []string `json:"source_units"`
	EdgeKinds   []string `json:"edge_kinds"`
}

type Adapter interface {
	Language() string
	Capabilities() Capabilities
	Load(LoadRequest) (*model.Graph, error)
}

type Registry struct {
	adapters map[string]Adapter
}

func NewRegistry(adapters ...Adapter) (*Registry, error) {
	registry := &Registry{adapters: map[string]Adapter{}}
	for _, adapter := range adapters {
		if err := registry.Register(adapter); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func DefaultRegistry() *Registry {
	registry, err := NewRegistry(GoAdapter{}, TypeScriptAdapter{}, PythonAdapter{})
	if err != nil {
		panic(err)
	}
	return registry
}

func (r *Registry) Register(adapter Adapter) error {
	if adapter == nil || adapter.Language() == "" {
		return fmt.Errorf("graph adapter language is required")
	}
	if r.adapters == nil {
		r.adapters = map[string]Adapter{}
	}
	if _, exists := r.adapters[adapter.Language()]; exists {
		return fmt.Errorf("graph adapter for language %q is already registered", adapter.Language())
	}
	r.adapters[adapter.Language()] = adapter
	return nil
}

func (r *Registry) Lookup(language string) (Adapter, error) {
	adapter := r.adapters[language]
	if adapter == nil {
		return nil, fmt.Errorf("source.language %q is not supported by a graph adapter", language)
	}
	return adapter, nil
}

func (r *Registry) Load(language string, request LoadRequest) (*model.Graph, error) {
	adapter, err := r.Lookup(language)
	if err != nil {
		return nil, err
	}
	if request.Root == "" {
		return nil, fmt.Errorf("graph load root is required")
	}
	return adapter.Load(request)
}

func Load(root, language string) (*model.Graph, error) {
	return LoadWithRequest(LoadRequest{Root: root}, language)
}

func LoadWithRequest(request LoadRequest, language string) (*model.Graph, error) {
	return DefaultRegistry().Load(language, request)
}

type GoAdapter struct{}

func (GoAdapter) Language() string {
	return "go"
}

func (GoAdapter) Capabilities() Capabilities {
	return Capabilities{SourceUnits: []string{"package"}, EdgeKinds: []string{"import"}}
}

func (GoAdapter) Load(request LoadRequest) (*model.Graph, error) {
	return LoadGo(request.Root)
}

type TypeScriptAdapter struct{}

func (TypeScriptAdapter) Language() string {
	return "typescript"
}

func (TypeScriptAdapter) Capabilities() Capabilities {
	return Capabilities{SourceUnits: []string{"file"}, EdgeKinds: []string{"import", "require"}}
}

func (TypeScriptAdapter) Load(request LoadRequest) (*model.Graph, error) {
	return LoadTypeScript(request.Root)
}

type PythonAdapter struct{}

func (PythonAdapter) Language() string {
	return "python"
}

func (PythonAdapter) Capabilities() Capabilities {
	return Capabilities{SourceUnits: []string{"file"}, EdgeKinds: []string{"import"}}
}

func (PythonAdapter) Load(request LoadRequest) (*model.Graph, error) {
	return LoadPython(request.Root)
}

func StableCopy(input *model.Graph) *model.Graph {
	if input == nil {
		return nil
	}
	output := &model.Graph{
		ModulePath: input.ModulePath,
		Packages:   append([]*model.Package(nil), input.Packages...),
		Edges:      append([]*model.Edge(nil), input.Edges...),
	}
	sort.SliceStable(output.Packages, func(i, j int) bool {
		return output.Packages[i].ImportPath < output.Packages[j].ImportPath
	})
	sort.SliceStable(output.Edges, func(i, j int) bool {
		left, right := output.Edges[i], output.Edges[j]
		if left.FromPath != right.FromPath {
			return left.FromPath < right.FromPath
		}
		if left.File != right.File {
			return left.File < right.File
		}
		if left.Line != right.Line {
			return left.Line < right.Line
		}
		return left.ToImportPath < right.ToImportPath
	})
	return output
}
