package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"ingen/paddock/internal/model"
)

const DocumentSchema = "paddock.graph/v1"
const RequestSchema = "paddock.graph-request/v1"
const ValidationSchema = "paddock.adapter-validation/v1"

type ValidationDocument struct {
	Schema     string            `json:"schema"`
	Operation  string            `json:"operation"`
	Root       string            `json:"root"`
	Language   string            `json:"language"`
	SourceUnit string            `json:"source_unit,omitempty"`
	Adapter    string            `json:"adapter"`
	Valid      bool              `json:"valid"`
	Errors     []ValidationIssue `json:"errors"`
}

type ValidationIssue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func ValidationDocumentForError(operation, root, language, unit, adapter string, err error) ValidationDocument {
	message := err.Error()
	code := ValidationCode(err)
	return ValidationDocument{
		Schema:     ValidationSchema,
		Operation:  operation,
		Root:       root,
		Language:   language,
		SourceUnit: unit,
		Adapter:    adapter,
		Valid:      false,
		Errors:     []ValidationIssue{{Code: code, Message: message}},
	}
}

func ValidationCode(err error) string {
	message := err.Error()
	code := "adapter-validation"
	switch {
	case strings.Contains(message, "expects language") || strings.Contains(message, "requested language"):
		code = "language-mismatch"
	case strings.Contains(message, "expects source_unit") || strings.Contains(message, "source unit"):
		code = "source-unit-mismatch"
	case strings.Contains(message, "requested root"):
		code = "root-mismatch"
	case strings.Contains(message, "capability"):
		code = "capability-mismatch"
	case strings.Contains(message, "returned invalid graph") || strings.HasPrefix(message, "graph document"):
		code = "invalid-graph"
	case strings.Contains(message, "exited with status") || strings.HasPrefix(message, "run external graph adapter"):
		code = "process-failure"
	case strings.Contains(message, "request"):
		code = "invalid-request"
	}
	return code
}

type Document struct {
	Schema       string           `json:"schema"`
	Language     string           `json:"language"`
	Unit         string           `json:"source_unit,omitempty"`
	Root         string           `json:"root"`
	Roots        []string         `json:"roots,omitempty"`
	ModulePath   string           `json:"module_path"`
	Capabilities Capabilities     `json:"capabilities"`
	PackageCount int              `json:"package_count"`
	EdgeCount    int              `json:"edge_count"`
	Packages     []*model.Package `json:"packages"`
	Edges        []*model.Edge    `json:"edges"`
}

func LoadDocument(path string) (*model.Graph, Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, Document{}, fmt.Errorf("read graph document: %w", err)
	}
	return LoadDocumentData(data, path)
}

func LoadDocumentData(data []byte, source string) (*model.Graph, Document, error) {
	var document Document
	if err := json.Unmarshal(data, &document); err != nil {
		if source == "" {
			return nil, Document{}, fmt.Errorf("parse graph document: %w", err)
		}
		return nil, Document{}, fmt.Errorf("parse graph document %s: %w", source, err)
	}
	if err := document.Validate(); err != nil {
		return nil, Document{}, err
	}
	loaded := &model.Graph{
		ModulePath: document.ModulePath,
		Packages:   append([]*model.Package(nil), document.Packages...),
		Edges:      append([]*model.Edge(nil), document.Edges...),
	}
	loaded = StableCopy(loaded)
	document.Packages = loaded.Packages
	document.Edges = loaded.Edges
	document.PackageCount = len(loaded.Packages)
	document.EdgeCount = len(loaded.Edges)
	return loaded, document, nil
}

func (d Document) Validate() error {
	if d.Schema != DocumentSchema {
		return fmt.Errorf("graph document schema must be %s, got %q", DocumentSchema, d.Schema)
	}
	if d.Language == "" {
		return fmt.Errorf("graph document language is required")
	}
	if d.Root == "" {
		return fmt.Errorf("graph document root is required")
	}
	if len(d.Packages) == 0 {
		return fmt.Errorf("graph document must contain packages")
	}
	if err := d.Capabilities.Validate(); err != nil {
		return fmt.Errorf("graph document capabilities: %w", err)
	}
	if d.PackageCount != 0 && d.PackageCount != len(d.Packages) {
		return fmt.Errorf("graph document package_count does not match packages")
	}
	if d.EdgeCount != 0 && d.EdgeCount != len(d.Edges) {
		return fmt.Errorf("graph document edge_count does not match edges")
	}
	byImport := make(map[string]*model.Package, len(d.Packages))
	byPath := make(map[string]*model.Package, len(d.Packages))
	for _, pkg := range d.Packages {
		if pkg == nil || pkg.ImportPath == "" {
			return fmt.Errorf("graph document packages need import paths")
		}
		if _, exists := byImport[pkg.ImportPath]; exists {
			return fmt.Errorf("graph document has duplicate package %q", pkg.ImportPath)
		}
		if pkg.RelPath != "" {
			if _, exists := byPath[pkg.RelPath]; exists {
				return fmt.Errorf("graph document has duplicate package path %q", pkg.RelPath)
			}
			byPath[pkg.RelPath] = pkg
		}
		byImport[pkg.ImportPath] = pkg
	}
	for _, edge := range d.Edges {
		if edge == nil || edge.FromImportPath == "" || edge.ToImportPath == "" || edge.Kind == "" || edge.TargetKind == "" {
			return fmt.Errorf("graph document edges need from, to, kind, and target_kind")
		}
		if !contains(d.Capabilities.EdgeKinds, edge.Kind) {
			return fmt.Errorf("graph document edge kind %q is not declared in capabilities", edge.Kind)
		}
		if byImport[edge.FromImportPath] == nil {
			return fmt.Errorf("graph document edge refers to unknown source package %q", edge.FromImportPath)
		}
		if edge.TargetKind == "internal" {
			target := byImport[edge.ToImportPath]
			if target == nil {
				return fmt.Errorf("graph document internal edge refers to unknown target package %q", edge.ToImportPath)
			}
			if edge.ToPath != "" && edge.ToPath != target.RelPath {
				return fmt.Errorf("graph document edge target path does not match package %q", edge.ToImportPath)
			}
		}
	}
	return nil
}

type LoadRequest struct {
	Root  string
	Unit  string
	Roots []string
}

// Request is sent as JSON on stdin to an external graph adapter. The adapter
// must return a Document using DocumentSchema on stdout.
type Request struct {
	Schema               string       `json:"schema"`
	Language             string       `json:"language"`
	Unit                 string       `json:"source_unit,omitempty"`
	Root                 string       `json:"root"`
	Roots                []string     `json:"roots,omitempty"`
	RequiredCapabilities Capabilities `json:"required_capabilities"`
}

func (r Request) Validate() error {
	if r.Schema != RequestSchema {
		return fmt.Errorf("graph adapter request schema must be %s, got %q", RequestSchema, r.Schema)
	}
	if r.Language == "" {
		return fmt.Errorf("graph adapter request language is required")
	}
	if r.Root == "" {
		return fmt.Errorf("graph adapter request root is required")
	}
	if err := r.RequiredCapabilities.validate(false); err != nil {
		return fmt.Errorf("graph adapter request capabilities: %w", err)
	}
	return nil
}

type Capabilities struct {
	SourceUnits []string `json:"source_units"`
	EdgeKinds   []string `json:"edge_kinds"`
}

func (c Capabilities) Validate() error {
	return c.validate(true)
}

func (c Capabilities) validate(requireValues bool) error {
	if requireValues && len(c.SourceUnits) == 0 {
		return fmt.Errorf("source_units are required")
	}
	if requireValues && len(c.EdgeKinds) == 0 {
		return fmt.Errorf("edge_kinds are required")
	}
	if err := validateCapabilityList("source unit", c.SourceUnits); err != nil {
		return err
	}
	return validateCapabilityList("edge kind", c.EdgeKinds)
}

func (c Capabilities) Supports(required Capabilities) error {
	if err := required.validate(false); err != nil {
		return err
	}
	for _, sourceUnit := range required.SourceUnits {
		if !contains(c.SourceUnits, sourceUnit) {
			return fmt.Errorf("adapter does not support required source unit %q", sourceUnit)
		}
	}
	for _, edgeKind := range required.EdgeKinds {
		if !contains(c.EdgeKinds, edgeKind) {
			return fmt.Errorf("adapter does not support required edge kind %q", edgeKind)
		}
	}
	return nil
}

func validateCapabilityList(kind string, values []string) error {
	seen := map[string]struct{}{}
	for _, value := range values {
		if value == "" {
			return fmt.Errorf("%ss cannot contain empty values", kind)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("duplicate %s %q", kind, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func normalizeCapabilities(capabilities Capabilities) Capabilities {
	capabilities.SourceUnits = append(make([]string, 0, len(capabilities.SourceUnits)), capabilities.SourceUnits...)
	capabilities.EdgeKinds = append(make([]string, 0, len(capabilities.EdgeKinds)), capabilities.EdgeKinds...)
	return capabilities
}

// LoadExternal invokes an explicitly selected adapter executable. Request JSON
// is sent on stdin; stdout is reserved for one graph document and stderr is
// treated as diagnostic output. The caller owns the executable's lifecycle and
// can cancel it through ctx.
func LoadExternal(ctx context.Context, executable string, args []string, request Request) (*model.Graph, Document, error) {
	if executable == "" {
		return nil, Document{}, fmt.Errorf("external graph adapter executable is required")
	}
	if err := request.Validate(); err != nil {
		return nil, Document{}, err
	}
	request.RequiredCapabilities = normalizeCapabilities(request.RequiredCapabilities)
	requestData, err := json.Marshal(request)
	if err != nil {
		return nil, Document{}, fmt.Errorf("encode graph adapter request: %w", err)
	}
	requestData = append(requestData, '\n')

	command := exec.CommandContext(ctx, executable, args...)
	command.Stdin = bytes.NewReader(requestData)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		diagnostic := strings.TrimSpace(stderr.String())
		if exitError, ok := err.(*exec.ExitError); ok {
			if diagnostic != "" {
				return nil, Document{}, fmt.Errorf("external graph adapter %q exited with status %d: %s", executable, exitError.ExitCode(), diagnostic)
			}
			return nil, Document{}, fmt.Errorf("external graph adapter %q exited with status %d", executable, exitError.ExitCode())
		}
		return nil, Document{}, fmt.Errorf("run external graph adapter %q: %w", executable, err)
	}

	loaded, document, err := LoadDocumentData(stdout.Bytes(), "adapter stdout")
	if err != nil {
		if diagnostic := strings.TrimSpace(stderr.String()); diagnostic != "" {
			return nil, Document{}, fmt.Errorf("external graph adapter returned invalid graph: %w (stderr: %s)", err, diagnostic)
		}
		return nil, Document{}, fmt.Errorf("external graph adapter returned invalid graph: %w", err)
	}
	if document.Language != request.Language {
		return nil, Document{}, fmt.Errorf("external graph adapter language %q does not match requested language %q", document.Language, request.Language)
	}
	if document.Root != request.Root {
		return nil, Document{}, fmt.Errorf("external graph adapter root %q does not match requested root %q", document.Root, request.Root)
	}
	if request.Unit != "" && document.Unit != request.Unit {
		return nil, Document{}, fmt.Errorf("external graph adapter source unit %q does not match requested source unit %q", document.Unit, request.Unit)
	}
	if err := document.Capabilities.Supports(request.RequiredCapabilities); err != nil {
		return nil, Document{}, fmt.Errorf("external graph adapter capability negotiation failed: %w", err)
	}
	return loaded, document, nil
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
