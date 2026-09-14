package scaffold

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"ingen/paddock/internal/model"
)

type Options struct {
	Language string
	Unit     string
	Template string
	Root     string
	Graph    *model.Graph
}

type component struct {
	Name      string
	Match     string
	Recursive bool
	Labels    map[string]string
}

var templates = map[string]map[string]bool{
	"layered":          {"go": true, "typescript": true, "python": true},
	"hexagonal":        {"go": true},
	"modular-monolith": {"go": true},
	"feature-sliced":   {"typescript": true},
	"cyclic":           {"go": true, "typescript": true},
}

func Generate(options Options) ([]byte, error) {
	if options.Graph == nil {
		return nil, fmt.Errorf("scaffold graph is required")
	}
	if !templateSupported(options.Template, options.Language) {
		return nil, fmt.Errorf("template %q is not supported for language %q", options.Template, options.Language)
	}
	unit := options.Unit
	if unit == "" {
		unit = defaultUnit(options.Language)
	}
	components := discoverComponents(options.Graph, options.Language, options.Template, unit)
	if len(components) == 0 {
		return nil, fmt.Errorf("cannot scaffold a policy without source components")
	}
	roots := discoverRoots(options.Graph)
	project := options.Graph.ModulePath
	if project == "" {
		project = filepath.Base(options.Root)
	}
	var output strings.Builder
	output.WriteString("# Paddock starter policy — DRAFT\n")
	output.WriteString("# Review component matches, labels, and severities before using this in CI.\n")
	output.WriteString("# Generated from the current source graph; it does not certify the existing architecture.\n")
	output.WriteString("# Template: " + options.Template + "\n\n")
	fmt.Fprintf(&output, "schema: paddock.architecture/v1\nproject: %s\n\n", strconv.Quote(project))
	fmt.Fprintf(&output, "source:\n  language: %s\n  unit: %s\n  roots: [%s]\n\n", options.Language, unit, strings.Join(roots, ", "))
	output.WriteString("components:\n")
	for _, item := range components {
		match := item.Match + "/**"
		if item.Match == "." {
			if item.Recursive {
				match = "**"
			} else if unit == "package" {
				// The Go module root is represented by an empty relative
				// package path. An empty pattern matches that package only.
				match = ""
			} else {
				// Root-level files have one path segment. Keep them separate
				// from nested components in a mixed source tree.
				match = "*"
			}
		} else if !item.Recursive {
			match = item.Match
		}
		// A bare glob is a YAML alias indicator, not a scalar. Quote root
		// globs while leaving ordinary path patterns easy to read.
		matchValue := match
		if match == "**" || match == "*" {
			matchValue = strconv.Quote(match)
		}
		fmt.Fprintf(&output, "  %s:\n    match: %s\n    labels:\n", item.Name, matchValue)
		keys := make([]string, 0, len(item.Labels))
		for key := range item.Labels {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(&output, "      %s: %s\n", key, strconv.Quote(item.Labels[key]))
		}
	}
	output.WriteString("\nrules:\n")
	writeRules(&output, options.Language, options.Template, unit)
	output.WriteString("\nwaivers: []\n")
	return []byte(output.String()), nil
}

func templateSupported(template, language string) bool {
	if template == "layered" || template == "cyclic" {
		return true
	}
	return templates[template][language]
}

func defaultUnit(language string) string {
	if language == "typescript" || language == "python" {
		return "file"
	}
	return "package"
}

func discoverRoots(dependencyGraph *model.Graph) []string {
	seen := map[string]bool{}
	for _, pkg := range dependencyGraph.Packages {
		path := strings.Trim(filepath.ToSlash(pkg.RelPath), "/")
		if path == "" {
			continue
		}
		root := strings.Split(path, "/")[0]
		seen[root] = true
	}
	roots := make([]string, 0, len(seen))
	for root := range seen {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	if len(roots) == 0 {
		return []string{"."}
	}
	return roots
}

func discoverComponents(dependencyGraph *model.Graph, language, template, unit string) []component {
	groups := map[string]bool{}
	recursive := map[string]bool{}
	pythonDirectoriesWithSource := map[string]bool{}
	if language == "python" {
		for _, pkg := range dependencyGraph.Packages {
			path := filepath.ToSlash(pkg.RelPath)
			if filepath.Base(path) != "__init__.py" {
				pythonDirectoriesWithSource[filepath.ToSlash(filepath.Dir(path))] = true
			}
		}
	}
	for _, pkg := range dependencyGraph.Packages {
		path := strings.Trim(filepath.ToSlash(pkg.RelPath), "/")
		isRecursive := true
		if language == "python" && filepath.Base(path) == "__init__.py" {
			if pythonDirectoriesWithSource[filepath.ToSlash(filepath.Dir(path))] {
				continue
			}
			isRecursive = false
		} else if unit == "file" {
			path = filepath.ToSlash(filepath.Dir(path))
		}
		parts := splitPath(path)
		if len(parts) == 0 {
			groups["."] = true
			continue
		}
		groupSize := 2
		if parts[0] == "internal" && len(parts) >= 3 {
			groupSize = 3
		}
		if len(parts) < groupSize {
			groupSize = len(parts)
		}
		componentPath := strings.Join(parts[:groupSize], "/")
		groups[componentPath] = true
		recursive[componentPath] = isRecursive
	}
	// A broad context component and a nested layer component overlap for every
	// package below that context. Keep the parent component for the context's
	// own package, but make its match exact so generated drafts satisfy the
	// coverage invariant. Descendants still carry the inherited context label
	// on their more-specific component.
	for path := range groups {
		if path == "." && len(groups) > 1 {
			recursive[path] = false
			continue
		}
		for descendant := range groups {
			if path != descendant && strings.HasPrefix(descendant, path+"/") {
				recursive[path] = false
				break
			}
		}
	}

	paths := make([]string, 0, len(groups))
	for path := range groups {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	components := make([]component, 0, len(paths))
	usedNames := map[string]int{}
	for _, path := range paths {
		name := strings.ReplaceAll(path, "/", "-")
		if name == "." {
			name = "root"
		}
		usedNames[name]++
		if usedNames[name] > 1 {
			name += fmt.Sprintf("-%d", usedNames[name])
		}
		labels := map[string]string{"role": inferRole(path, template)}
		parts := splitPath(path)
		if len(parts) >= 2 && parts[0] == "internal" {
			labels["context"] = parts[1]
		}
		if len(parts) >= 2 && parts[0] == "src" {
			labels["slice"] = parts[1]
		}
		if layer, ok := inferLayer(path, template); ok {
			labels["layer"] = strconv.Itoa(layer)
		}
		components = append(components, component{Name: name, Match: path, Recursive: recursive[path], Labels: labels})
	}
	return components
}

func splitPath(path string) []string {
	if path == "" || path == "." {
		return nil
	}
	return strings.Split(path, "/")
}

func inferRole(path, template string) string {
	parts := splitPath(path)
	for index := len(parts) - 1; index >= 0; index-- {
		name := strings.ToLower(parts[index])
		switch {
		case template == "feature-sliced" && name == "shared":
			return "shared"
		case template == "feature-sliced" && name == "entities":
			return "entities"
		case template == "feature-sliced" && name == "features":
			return "feature"
		case template == "feature-sliced" && name == "widgets":
			return "widget"
		case template == "feature-sliced" && name == "pages":
			return "page"
		case template == "feature-sliced" && name == "app":
			return "app"
		case name == "domain":
			return "domain"
		case name == "application", name == "service", name == "services", name == "usecase", name == "usecases":
			return "application"
		case name == "port", name == "ports":
			return "port"
		case name == "adapter", name == "adapters", name == "transport":
			return "adapter"
		case name == "repository", name == "repositories", name == "infrastructure", name == "infra":
			return "infrastructure"
		case name == "cmd", name == "api", name == "app":
			return "composition"
		}
	}
	return "unclassified"
}

func inferLayer(path, template string) (int, bool) {
	role := inferRole(path, template)
	if template == "feature-sliced" {
		layers := map[string]int{"shared": 0, "entities": 1, "feature": 2, "widget": 3, "page": 4, "app": 5}
		layer, ok := layers[role]
		return layer, ok
	}
	if template == "layered" {
		layers := map[string]int{"domain": 0, "application": 1, "infrastructure": 2, "adapter": 3, "composition": 4}
		layer, ok := layers[role]
		return layer, ok
	}
	return 0, false
}

func writeRules(output *strings.Builder, language, template, unit string) {
	writeRule(output, "complete-classification", "coverage", "Every source unit should have one reviewed component.")
	switch template {
	case "layered":
		writeRuleWithDetails(output, "layers-point-inward", "layer-direction", "Dependencies should point toward lower layers.", "    direction: toward-lower-layer\n")
	case "hexagonal":
		writeRuleWithDetails(output, "domain-is-pure", "allow-dependencies", "Domain code should depend on stable abstractions, not infrastructure.", "    from: {role: domain}\n    allow:\n      - standard-library:errors\n      - standard-library:fmt\n      - {role: domain, context: same}\n      - {role: port, context: same}\n")
		writeRuleWithDetails(output, "adapters-point-inward", "allow-dependencies", "Adapters should point toward application, domain, and ports.", "    from: {role: adapter}\n    allow:\n      - {role: application}\n      - {role: domain}\n      - {role: port}\n      - external: approved\n")
	case "modular-monolith":
		writeRuleWithDetails(output, "contexts-are-islands", "no-cross-context", "Contexts should not import one another directly.", "")
	case "feature-sliced":
		writeRuleWithDetails(output, "layers-point-downward", "layer-direction", "Higher frontend layers should depend on lower layers.", "    direction: toward-lower-layer\n")
	case "cyclic":
		writeRuleWithDetails(output, "no-cycles", "no-cycles", "The selected dependency graph should remain acyclic.", "")
	}
	if unit == "file" {
		writeRule(output, "no-unresolved-imports", "unresolved", "Relative and configured alias imports should resolve.")
	}
}

func writeRule(output *strings.Builder, id, kind, message string) {
	writeRuleWithDetails(output, id, kind, message, "")
}

func writeRuleWithDetails(output *strings.Builder, id, kind, message, details string) {
	fmt.Fprintf(output, "  - id: %s\n    kind: %s\n    severity: warning\n", id, kind)
	output.WriteString(details)
	fmt.Fprintf(output, "    message: %s\n", strconv.Quote(message))
}
