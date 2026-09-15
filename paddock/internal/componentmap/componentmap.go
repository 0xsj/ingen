package componentmap

import (
	"fmt"
	"sort"
	"strings"

	"ingen/paddock/internal/model"
)

const Schema = "paddock.component-map/v1"

type Document struct {
	Schema                 string               `json:"schema"`
	Language               string               `json:"language"`
	Unit                   string               `json:"source_unit"`
	Root                   string               `json:"root"`
	ModulePath             string               `json:"module_path,omitempty"`
	PackageCount           int                  `json:"package_count"`
	EdgeCount              int                  `json:"edge_count"`
	Components             []Component          `json:"components"`
	Dependencies           []Dependency         `json:"dependencies"`
	ExternalDependencies   []ExternalDependency `json:"external_dependencies,omitempty"`
	ClassificationFindings []*model.Finding     `json:"classification_findings,omitempty"`
}

type Component struct {
	Name         string            `json:"name"`
	Labels       map[string]string `json:"labels,omitempty"`
	PackageCount int               `json:"package_count"`
	Packages     []string          `json:"packages"`
}

type Dependency struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Edges int    `json:"edges"`
}

type ExternalDependency struct {
	From       string `json:"from"`
	Target     string `json:"target"`
	TargetKind string `json:"target_kind"`
	Edges      int    `json:"edges"`
}

func Build(root, language, unit string, dependencyGraph *model.Graph, classificationFindings []*model.Finding) (Document, error) {
	if dependencyGraph == nil {
		return Document{}, fmt.Errorf("component map graph is required")
	}
	if root == "" {
		return Document{}, fmt.Errorf("component map root is required")
	}

	byImport := make(map[string]*model.Package, len(dependencyGraph.Packages))
	components := make(map[string]*Component)
	for _, pkg := range dependencyGraph.Packages {
		if pkg == nil || pkg.ImportPath == "" {
			continue
		}
		byImport[pkg.ImportPath] = pkg
		name := componentName(pkg)
		component := components[name]
		if component == nil {
			labels := copyLabels(pkg.Labels)
			if name == "unclassified" && labels == nil {
				labels = map[string]string{"role": "unclassified"}
			}
			component = &Component{Name: name, Labels: labels, Packages: []string{}}
			components[name] = component
		}
		component.Packages = append(component.Packages, pkg.RelPath)
		component.PackageCount++
	}

	dependencyCounts := make(map[[2]string]int)
	externalCounts := make(map[[3]string]int)
	for _, edge := range dependencyGraph.Edges {
		from := byImport[edge.FromImportPath]
		if from == nil {
			continue
		}
		fromComponent := componentName(from)
		if edge.TargetKind == "internal" {
			to := byImport[edge.ToImportPath]
			if to == nil {
				continue
			}
			toComponent := componentName(to)
			if fromComponent != toComponent {
				dependencyCounts[[2]string{fromComponent, toComponent}]++
			}
			continue
		}
		target := edge.ToPath
		if target == "" {
			target = edge.ToImportPath
		}
		externalCounts[[3]string{fromComponent, target, edge.TargetKind}]++
	}

	result := Document{
		Schema:                 Schema,
		Language:               language,
		Unit:                   unit,
		Root:                   root,
		ModulePath:             dependencyGraph.ModulePath,
		PackageCount:           len(dependencyGraph.Packages),
		EdgeCount:              len(dependencyGraph.Edges),
		Components:             make([]Component, 0, len(components)),
		Dependencies:           make([]Dependency, 0, len(dependencyCounts)),
		ExternalDependencies:   make([]ExternalDependency, 0, len(externalCounts)),
		ClassificationFindings: append([]*model.Finding(nil), classificationFindings...),
	}
	for _, component := range components {
		sort.Strings(component.Packages)
		component.PackageCount = len(component.Packages)
		result.Components = append(result.Components, *component)
	}
	sort.Slice(result.Components, func(i, j int) bool {
		return result.Components[i].Name < result.Components[j].Name
	})
	for key, edges := range dependencyCounts {
		result.Dependencies = append(result.Dependencies, Dependency{From: key[0], To: key[1], Edges: edges})
	}
	sort.Slice(result.Dependencies, func(i, j int) bool {
		if result.Dependencies[i].From != result.Dependencies[j].From {
			return result.Dependencies[i].From < result.Dependencies[j].From
		}
		return result.Dependencies[i].To < result.Dependencies[j].To
	})
	for key, edges := range externalCounts {
		result.ExternalDependencies = append(result.ExternalDependencies, ExternalDependency{From: key[0], Target: key[1], TargetKind: key[2], Edges: edges})
	}
	sort.Slice(result.ExternalDependencies, func(i, j int) bool {
		a, b := result.ExternalDependencies[i], result.ExternalDependencies[j]
		if a.From != b.From {
			return a.From < b.From
		}
		if a.TargetKind != b.TargetKind {
			return a.TargetKind < b.TargetKind
		}
		return a.Target < b.Target
	})
	return result, nil
}

func componentName(pkg *model.Package) string {
	if pkg.Component == "" {
		return "unclassified"
	}
	return pkg.Component
}

func copyLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	copy := make(map[string]string, len(labels))
	for key, value := range labels {
		copy[key] = value
	}
	return copy
}

func Text(document Document) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "COMPONENT-MAP %s %s (%d packages, %d edges)\n", document.Language, document.Root, document.PackageCount, document.EdgeCount)
	if document.ModulePath != "" {
		fmt.Fprintf(&builder, "MODULE %s\n", document.ModulePath)
	}
	fmt.Fprintln(&builder, "COMPONENTS")
	for _, component := range document.Components {
		fmt.Fprintf(&builder, "  %s (%d packages)\n", component.Name, component.PackageCount)
	}
	fmt.Fprintln(&builder, "DEPENDENCIES")
	for _, dependency := range document.Dependencies {
		fmt.Fprintf(&builder, "  %s -> %s (%d edges)\n", dependency.From, dependency.To, dependency.Edges)
	}
	if len(document.ExternalDependencies) > 0 {
		fmt.Fprintln(&builder, "EXTERNAL DEPENDENCIES")
		for _, dependency := range document.ExternalDependencies {
			fmt.Fprintf(&builder, "  %s -> %s [%s] (%d edges)\n", dependency.From, dependency.Target, dependency.TargetKind, dependency.Edges)
		}
	}
	if len(document.ClassificationFindings) > 0 {
		fmt.Fprintln(&builder, "CLASSIFICATION FINDINGS")
		for _, finding := range document.ClassificationFindings {
			fmt.Fprintf(&builder, "  %s: %s\n", finding.From, finding.Message)
		}
	}
	return builder.String()
}
