package componentmap_test

import (
	"testing"

	"ingen/paddock/internal/componentmap"
	"ingen/paddock/internal/model"
)

func TestBuildSummarizesInternalAndExternalDependencies(t *testing.T) {
	graph := &model.Graph{
		ModulePath: "example",
		Packages: []*model.Package{
			{ImportPath: "example/app", RelPath: "app", Component: "composition", Labels: map[string]string{"layer": "2"}},
			{ImportPath: "example/domain", RelPath: "domain", Component: "domain", Labels: map[string]string{"layer": "0"}},
			{ImportPath: "example/missing", RelPath: "missing"},
		},
		Edges: []*model.Edge{
			{FromImportPath: "example/app", ToImportPath: "example/domain", TargetKind: "internal"},
			{FromImportPath: "example/app", ToImportPath: "example/domain", TargetKind: "internal"},
			{FromImportPath: "example/domain", ToImportPath: "net/http", TargetKind: "external"},
			{FromImportPath: "example/missing", ToImportPath: "./unknown", TargetKind: "unresolved"},
		},
	}

	document, err := componentmap.Build("/tmp/example", "go", "package", graph, nil)
	if err != nil {
		t.Fatal(err)
	}
	if document.Schema != componentmap.Schema || len(document.Components) != 3 {
		t.Fatalf("document = %#v", document)
	}
	if len(document.Dependencies) != 1 || document.Dependencies[0].Edges != 2 {
		t.Fatalf("dependencies = %#v", document.Dependencies)
	}
	if len(document.ExternalDependencies) != 2 {
		t.Fatalf("external dependencies = %#v", document.ExternalDependencies)
	}
	if document.Components[2].Name != "unclassified" || document.Components[2].Labels["role"] != "unclassified" {
		t.Fatalf("unclassified component = %#v", document.Components[2])
	}
}
