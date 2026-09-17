package componentmap_test

import (
	"strings"
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

func TestBuildPreservesLabelVariants(t *testing.T) {
	graph := &model.Graph{
		ModulePath: "example",
		Packages: []*model.Package{
			{ImportPath: "example/app/orders", RelPath: "app/orders", Component: "application", Labels: map[string]string{"context": "orders", "layer": "1", "role": "application"}},
			{ImportPath: "example/app/billing", RelPath: "app/billing", Component: "application", Labels: map[string]string{"context": "billing", "layer": "1", "role": "application"}},
			{ImportPath: "example/domain/orders", RelPath: "domain/orders", Component: "domain", Labels: map[string]string{"context": "orders", "layer": "0", "role": "domain"}},
		},
		Edges: []*model.Edge{
			{FromImportPath: "example/app/orders", ToImportPath: "example/domain/orders", TargetKind: "internal"},
			{FromImportPath: "example/app/orders", ToImportPath: "example/app/billing", TargetKind: "internal"},
			{FromImportPath: "example/app/billing", ToImportPath: "net/http", TargetKind: "external"},
		},
	}

	document, err := componentmap.Build("/tmp/example", "go", "package", graph, nil)
	if err != nil {
		t.Fatal(err)
	}
	contexts := map[string]bool{}
	for _, component := range document.Components {
		if component.Name == "application" {
			contexts[component.Labels["context"]] = true
			if component.Identity == "" || !strings.Contains(component.Identity, "context="+component.Labels["context"]) {
				t.Fatalf("application variant lost identity: %#v", component)
			}
		}
	}
	if len(contexts) != 2 {
		t.Fatalf("application label variants were collapsed: %#v", document.Components)
	}

	foundCrossVariant := false
	for _, dependency := range document.Dependencies {
		if dependency.From == "application" && dependency.To == "application" {
			foundCrossVariant = dependency.FromIdentity != "" && dependency.ToIdentity != ""
		}
	}
	if !foundCrossVariant {
		t.Fatalf("cross-variant dependency identities missing: %#v", document.Dependencies)
	}
	foundExternalVariant := false
	for _, dependency := range document.ExternalDependencies {
		if dependency.From == "application" && dependency.Target == "net/http" {
			foundExternalVariant = dependency.FromIdentity != ""
		}
	}
	if !foundExternalVariant {
		t.Fatalf("external dependency variant identity missing: %#v", document.ExternalDependencies)
	}
	text := componentmap.Text(document)
	if !strings.Contains(text, "application[context=orders,layer=1,role=application]") || !strings.Contains(text, "application[context=billing,layer=1,role=application]") {
		t.Fatalf("text component map omitted label variants:\n%s", text)
	}
}
