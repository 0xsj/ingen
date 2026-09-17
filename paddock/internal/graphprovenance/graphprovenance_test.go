package graphprovenance

import (
	"testing"

	"ingen/paddock/internal/model"
)

func TestSHA256IsStableAcrossGraphOrdering(t *testing.T) {
	first := &model.Graph{
		ModulePath: "example/service",
		Packages: []*model.Package{
			{ImportPath: "example/service/domain", RelPath: "domain"},
			{ImportPath: "example/service/app", RelPath: "app"},
		},
		Edges: []*model.Edge{
			{FromImportPath: "example/service/domain", FromPath: "domain", ToImportPath: "example/service/app", ToPath: "app", Kind: "import", TargetKind: "internal"},
			{FromImportPath: "example/service/app", FromPath: "app", ToImportPath: "example/service/domain", ToPath: "domain", Kind: "import", TargetKind: "internal"},
		},
	}
	second := &model.Graph{
		ModulePath: first.ModulePath,
		Packages:   []*model.Package{first.Packages[1], first.Packages[0]},
		Edges:      []*model.Edge{first.Edges[1], first.Edges[0]},
	}

	left, err := SHA256("go", "package", first)
	if err != nil {
		t.Fatal(err)
	}
	right, err := SHA256("go", "package", second)
	if err != nil {
		t.Fatal(err)
	}
	if left != right || len(left) != 64 {
		t.Fatalf("SHA256() differs for equivalent graph order: %q != %q", left, right)
	}
}

func TestSHA256IsStableAcrossEquivalentEdgePrefixOrdering(t *testing.T) {
	first := &model.Graph{
		Packages: []*model.Package{{ImportPath: "example/service", RelPath: "."}},
		Edges: []*model.Edge{
			{FromImportPath: "example/service", FromPath: "main", ToImportPath: "net/http", Kind: "require", TargetKind: "external"},
			{FromImportPath: "example/service", FromPath: "main", ToImportPath: "net/http", Kind: "import", TargetKind: "external"},
		},
	}
	second := &model.Graph{
		Packages: first.Packages,
		Edges:    []*model.Edge{first.Edges[1], first.Edges[0]},
	}

	left, err := SHA256("go", "package", first)
	if err != nil {
		t.Fatal(err)
	}
	right, err := SHA256("go", "package", second)
	if err != nil {
		t.Fatal(err)
	}
	if left != right {
		t.Fatalf("SHA256() differs for equivalent edge ordering: %q != %q", left, right)
	}
}

func TestSHA256ChangesWithSourceConfigurationOrGraph(t *testing.T) {
	input := &model.Graph{Packages: []*model.Package{{ImportPath: "example/service", RelPath: "."}}}
	base, err := SHA256("go", "package", input)
	if err != nil {
		t.Fatal(err)
	}
	fileUnit, err := SHA256("go", "file", input)
	if err != nil {
		t.Fatal(err)
	}
	otherLanguage, err := SHA256("rust", "package", input)
	if err != nil {
		t.Fatal(err)
	}
	input.Packages[0].RelPath = "src"
	otherGraph, err := SHA256("go", "package", input)
	if err != nil {
		t.Fatal(err)
	}
	for label, got := range map[string]string{
		"unit":     fileUnit,
		"language": otherLanguage,
		"graph":    otherGraph,
	} {
		if got == base {
			t.Fatalf("SHA256() did not change for %s", label)
		}
	}
	if len(base) != 64 {
		t.Fatalf("SHA256() = %q, want a SHA-256 digest", base)
	}
}

func TestSHA256RejectsNilGraph(t *testing.T) {
	if _, err := SHA256("go", "package", nil); err == nil {
		t.Fatal("SHA256 accepted nil graph")
	}
}
