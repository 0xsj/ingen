package graph

import (
	"fmt"
	"path"
	"strings"

	"ingen/paddock/internal/model"
)

// FilterGraph returns the induced graph for the requested source scope.
// Include and exclude patterns are relative slash-separated paths. A plain
// directory pattern includes its descendants; `*` matches one path segment
// and `**` matches any number of segments. Internal edges whose target is out
// of scope are omitted, while external and unresolved edges remain visible.
func FilterGraph(input *model.Graph, include, exclude []string) (*model.Graph, error) {
	if input == nil {
		return nil, fmt.Errorf("graph scope input is required")
	}
	if err := validateScopePatterns("include", include); err != nil {
		return nil, err
	}
	if err := validateScopePatterns("exclude", exclude); err != nil {
		return nil, err
	}
	if len(include) == 0 && len(exclude) == 0 {
		return input, nil
	}

	selected := make(map[string]bool, len(input.Packages))
	packages := make([]*model.Package, 0, len(input.Packages))
	for _, pkg := range input.Packages {
		if pkg == nil || !scopePathSelected(pkg.RelPath, include, exclude) {
			continue
		}
		selected[pkg.ImportPath] = true
		packages = append(packages, pkg)
	}
	if len(packages) == 0 {
		return nil, fmt.Errorf("scan scope selected no source units")
	}

	result := &model.Graph{
		ModulePath: input.ModulePath,
		Packages:   packages,
		Edges:      make([]*model.Edge, 0, len(input.Edges)),
	}
	for _, edge := range input.Edges {
		if edge == nil || !selected[edge.FromImportPath] {
			continue
		}
		if edge.TargetKind == "internal" && !selected[edge.ToImportPath] {
			continue
		}
		result.Edges = append(result.Edges, edge)
	}
	return result, nil
}

func validateScopePatterns(kind string, patterns []string) error {
	for index, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			return fmt.Errorf("source.%s[%d] must not be empty", kind, index)
		}
		if strings.HasPrefix(pattern, "/") || pattern == ".." || strings.HasPrefix(pattern, "../") || strings.Contains(pattern, "/../") {
			return fmt.Errorf("source.%s[%d] must be a relative path pattern", kind, index)
		}
		for _, segment := range strings.Split(strings.Trim(pattern, "/"), "/") {
			if segment == "" || segment == "**" {
				continue
			}
			if _, err := path.Match(segment, ""); err != nil {
				return fmt.Errorf("source.%s[%d] has invalid pattern %q: %w", kind, index, pattern, err)
			}
		}
	}
	return nil
}

func scopePathSelected(relPath string, include, exclude []string) bool {
	relPath = strings.Trim(strings.ReplaceAll(relPath, "\\", "/"), "/")
	if relPath == "" {
		relPath = "."
	}
	if len(include) > 0 {
		matched := false
		for _, pattern := range include {
			if scopePatternMatches(pattern, relPath) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for _, pattern := range exclude {
		if scopePatternMatches(pattern, relPath) {
			return false
		}
	}
	return true
}

func scopePatternMatches(pattern, value string) bool {
	pattern = strings.Trim(strings.ReplaceAll(pattern, "\\", "/"), "/")
	if pattern == "" {
		pattern = "."
	}
	if pattern == "." {
		return true
	}
	if matchScopeSegments(strings.Split(pattern, "/"), strings.Split(value, "/")) {
		return true
	}
	// A plain directory name is a convenient shorthand for that directory and
	// everything below it, which makes `exclude: [node_modules]` predictable.
	return !strings.ContainsAny(pattern, "*?[") && strings.HasPrefix(value, pattern+"/")
}

func matchScopeSegments(pattern, value []string) bool {
	if len(pattern) == 0 {
		return len(value) == 0
	}
	if pattern[0] == "**" {
		if matchScopeSegments(pattern[1:], value) {
			return true
		}
		for index := range value {
			if matchScopeSegments(pattern[1:], value[index+1:]) {
				return true
			}
		}
		return false
	}
	if len(value) == 0 {
		return false
	}
	matched, err := path.Match(pattern[0], value[0])
	return err == nil && matched && matchScopeSegments(pattern[1:], value[1:])
}
