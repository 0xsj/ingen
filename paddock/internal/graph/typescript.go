package graph

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"ingen/paddock/internal/model"
)

var (
	typeScriptFromImport = regexp.MustCompile(`(?m)^\s*(?:import|export)\b[^\n;]*?\bfrom\s*["']([^"']+)["']`)
	typeScriptSideEffect = regexp.MustCompile(`(?m)^\s*import\s*["']([^"']+)["']`)
	typeScriptRequire    = regexp.MustCompile(`\brequire\s*\(\s*["']([^"']+)["']\s*\)`)
)

type typeScriptFile struct {
	absPath string
	relPath string
}

type typeScriptImport struct {
	path string
	kind string
	line int
}

type packageMetadata struct {
	Name string `json:"name"`
}

type typeScriptCompilerOptions struct {
	BaseURL string              `json:"baseUrl"`
	Paths   map[string][]string `json:"paths"`
}

type typeScriptProjectConfig struct {
	Root    string
	BaseURL string
	Paths   map[string][]string
}

// LoadTypeScript builds a file-level graph for TypeScript and JavaScript
// sources. Relative imports are resolved against source files; package imports
// remain external edges for policy rules to classify. This adapter deliberately
// avoids requiring npm or a project build so Paddock can run in CI with only
// the source tree available.
func LoadTypeScript(root string) (*model.Graph, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve source root: %w", err)
	}
	modulePath := typeScriptModulePath(root)
	projectConfig, err := loadTypeScriptProjectConfig(root)
	if err != nil {
		return nil, err
	}
	files, err := findTypeScriptFiles(root)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("typescript adapter found no source files in %s", root)
	}

	result := &model.Graph{ModulePath: modulePath}
	byRelPath := make(map[string]*model.Package, len(files))
	for _, file := range files {
		importPath := modulePath + "/" + file.relPath
		pkg := &model.Package{ImportPath: importPath, RelPath: file.relPath}
		result.Packages = append(result.Packages, pkg)
		byRelPath[file.relPath] = pkg
	}

	for _, file := range files {
		from := byRelPath[file.relPath]
		imports, err := importsInTypeScriptFile(file.absPath)
		if err != nil {
			return nil, err
		}
		for _, imported := range imports {
			target := resolveTypeScriptImport(file.relPath, imported.path, byRelPath, projectConfig)
			edge := &model.Edge{
				FromImportPath: from.ImportPath,
				FromPath:       from.RelPath,
				ToImportPath:   imported.path,
				Kind:           imported.kind,
				TargetKind:     typeScriptTargetKind(imported.path, target, projectConfig),
				File:           file.relPath,
				Line:           imported.line,
			}
			if target != nil {
				edge.ToImportPath = target.ImportPath
				edge.ToPath = target.RelPath
			}
			result.Edges = append(result.Edges, edge)
		}
	}
	return result, nil
}

func findTypeScriptFiles(root string) ([]typeScriptFile, error) {
	files := []typeScriptFile{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", ".next", ".nuxt", ".svelte-kit", ".turbo":
				return filepath.SkipDir
			case "dist", "build", "coverage", "storybook-static":
				// These names are commonly generated at the project root, but
				// can also be legitimate nested source namespaces (for example
				// lib/services/coverage).
				if filepath.Dir(path) == root {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !isTypeScriptSource(path) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("relative source path %s: %w", path, err)
		}
		files = append(files, typeScriptFile{absPath: path, relPath: filepath.ToSlash(rel)})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk TypeScript source: %w", err)
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].relPath < files[j].relPath
	})
	return files, nil
}

func isTypeScriptSource(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ts", ".tsx", ".js", ".jsx":
		return true
	default:
		return false
	}
}

func typeScriptModulePath(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err == nil {
		var metadata packageMetadata
		if json.Unmarshal(data, &metadata) == nil && metadata.Name != "" {
			return metadata.Name
		}
	}
	return filepath.Base(root)
}

func loadTypeScriptProjectConfig(root string) (typeScriptProjectConfig, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return typeScriptProjectConfig{}, fmt.Errorf("resolve TypeScript project root: %w", err)
	}
	config := typeScriptProjectConfig{Root: root, BaseURL: root, Paths: map[string][]string{}}
	path := filepath.Join(root, "tsconfig.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return config, nil
	}
	return loadTypeScriptConfigFile(path, root, map[string]bool{})
}

type typeScriptConfigFile struct {
	Extends         string                    `json:"extends"`
	CompilerOptions typeScriptCompilerOptions `json:"compilerOptions"`
}

func loadTypeScriptConfigFile(path, root string, loading map[string]bool) (typeScriptProjectConfig, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return typeScriptProjectConfig{}, fmt.Errorf("resolve %s: %w", path, err)
	}
	if loading[path] {
		return typeScriptProjectConfig{}, fmt.Errorf("TypeScript config extends cycle includes %s", path)
	}
	loading[path] = true
	defer delete(loading, path)

	data, err := os.ReadFile(path)
	if err != nil {
		return typeScriptProjectConfig{}, fmt.Errorf("read %s: %w", path, err)
	}
	var raw typeScriptConfigFile
	if err := json.Unmarshal(stripJSONC(data), &raw); err != nil {
		return typeScriptProjectConfig{}, fmt.Errorf("parse %s: %w", path, err)
	}

	config := typeScriptProjectConfig{Root: root, BaseURL: root, Paths: map[string][]string{}}
	if raw.Extends != "" {
		parentPath, err := resolveTypeScriptConfigExtends(filepath.Dir(path), raw.Extends)
		if err != nil {
			return typeScriptProjectConfig{}, fmt.Errorf("resolve extends in %s: %w", path, err)
		}
		config, err = loadTypeScriptConfigFile(parentPath, root, loading)
		if err != nil {
			return typeScriptProjectConfig{}, err
		}
	}

	if raw.CompilerOptions.BaseURL != "" {
		config.BaseURL = filepath.Clean(filepath.Join(filepath.Dir(path), raw.CompilerOptions.BaseURL))
	}
	if raw.CompilerOptions.Paths != nil {
		paths := make(map[string][]string, len(raw.CompilerOptions.Paths))
		for pattern, targets := range raw.CompilerOptions.Paths {
			absoluteTargets := make([]string, 0, len(targets))
			for _, target := range targets {
				absoluteTargets = append(absoluteTargets, filepath.Clean(filepath.Join(config.BaseURL, target)))
			}
			paths[pattern] = absoluteTargets
		}
		config.Paths = paths
	}
	return config, nil
}

func resolveTypeScriptConfigExtends(configDir, extends string) (string, error) {
	if strings.HasPrefix(extends, ".") || filepath.IsAbs(extends) {
		return firstTypeScriptConfigCandidate(filepath.Join(configDir, extends))
	}
	for current := configDir; ; current = filepath.Dir(current) {
		candidate, err := firstTypeScriptConfigCandidate(filepath.Join(current, "node_modules", extends))
		if err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return "", fmt.Errorf("cannot find extended config %q", extends)
}

func firstTypeScriptConfigCandidate(base string) (string, error) {
	candidates := []string{base}
	if filepath.Ext(base) == "" {
		candidates = append(candidates, base+".json")
	}
	candidates = append(candidates, filepath.Join(base, "tsconfig.json"))
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("config %q not found", base)
}

func stripJSONC(data []byte) []byte {
	var cleaned strings.Builder
	cleaned.Grow(len(data))
	inString := false
	escaped := false
	lineComment := false
	blockComment := false
	for index := 0; index < len(data); index++ {
		current := data[index]
		if lineComment {
			if current == '\n' {
				lineComment = false
				cleaned.WriteByte(current)
			}
			continue
		}
		if blockComment {
			if current == '*' && index+1 < len(data) && data[index+1] == '/' {
				blockComment = false
				index++
			}
			continue
		}
		if inString {
			cleaned.WriteByte(current)
			if escaped {
				escaped = false
			} else if current == '\\' {
				escaped = true
			} else if current == '"' {
				inString = false
			}
			continue
		}
		switch current {
		case '"':
			inString = true
			cleaned.WriteByte(current)
		case '/':
			if index+1 < len(data) && data[index+1] == '/' {
				lineComment = true
				index++
			} else if index+1 < len(data) && data[index+1] == '*' {
				blockComment = true
				index++
			} else {
				cleaned.WriteByte(current)
			}
		default:
			cleaned.WriteByte(current)
		}
	}

	dataWithoutTrailingCommas := cleaned.String()
	var result strings.Builder
	result.Grow(len(dataWithoutTrailingCommas))
	inString = false
	escaped = false
	for index := 0; index < len(dataWithoutTrailingCommas); index++ {
		current := dataWithoutTrailingCommas[index]
		if inString {
			result.WriteByte(current)
			if escaped {
				escaped = false
			} else if current == '\\' {
				escaped = true
			} else if current == '"' {
				inString = false
			}
			continue
		}
		if current == '"' {
			inString = true
			result.WriteByte(current)
			continue
		}
		if current == ',' {
			lookahead := index + 1
			for lookahead < len(dataWithoutTrailingCommas) && strings.ContainsRune(" \t\r\n", rune(dataWithoutTrailingCommas[lookahead])) {
				lookahead++
			}
			if lookahead < len(dataWithoutTrailingCommas) && (dataWithoutTrailingCommas[lookahead] == '}' || dataWithoutTrailingCommas[lookahead] == ']') {
				continue
			}
		}
		result.WriteByte(current)
	}
	return []byte(result.String())
}

func importsInTypeScriptFile(file string) ([]typeScriptImport, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", file, err)
	}
	contents := string(data)
	imports := []typeScriptImport{}
	seen := map[string]bool{}
	for _, expression := range []struct {
		pattern *regexp.Regexp
		kind    string
	}{
		{pattern: typeScriptFromImport, kind: "import"},
		{pattern: typeScriptSideEffect, kind: "import"},
		{pattern: typeScriptRequire, kind: "require"},
	} {
		for _, match := range expression.pattern.FindAllStringSubmatchIndex(contents, -1) {
			path := contents[match[2]:match[3]]
			line := 1 + strings.Count(contents[:match[0]], "\n")
			key := fmt.Sprintf("%s:%d", path, line)
			if seen[key] {
				continue
			}
			seen[key] = true
			imports = append(imports, typeScriptImport{path: path, kind: expression.kind, line: line})
		}
	}
	sort.Slice(imports, func(i, j int) bool {
		if imports[i].line != imports[j].line {
			return imports[i].line < imports[j].line
		}
		return imports[i].path < imports[j].path
	})
	return imports, nil
}

func resolveTypeScriptImport(fromPath, importPath string, packages map[string]*model.Package, config typeScriptProjectConfig) *model.Package {
	bases := []string{}
	if strings.HasPrefix(importPath, ".") {
		bases = append(bases, filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(fromPath), importPath))))
	} else {
		for pattern, targets := range config.Paths {
			capture, ok := pathAliasCapture(pattern, importPath)
			if !ok {
				continue
			}
			for _, target := range targets {
				target = strings.ReplaceAll(target, "*", capture)
				relative, err := filepath.Rel(config.Root, target)
				if err != nil || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || relative == ".." {
					continue
				}
				bases = append(bases, filepath.ToSlash(filepath.Clean(relative)))
			}
		}
	}
	for _, base := range bases {
		for _, candidate := range typeScriptCandidates(base) {
			if target := packages[candidate]; target != nil {
				return target
			}
		}
	}
	return nil
}

func typeScriptCandidates(base string) []string {
	candidates := []string{base}
	extension := strings.ToLower(filepath.Ext(base))
	if extension == "" || (!isTypeScriptSourceExtension(extension) && !isTypeScriptAssetExtension(extension)) {
		for _, candidateExtension := range []string{".ts", ".tsx", ".js", ".jsx"} {
			candidates = append(candidates, base+candidateExtension)
		}
	} else if extension == ".js" || extension == ".jsx" {
		withoutExtension := strings.TrimSuffix(base, filepath.Ext(base))
		for _, candidateExtension := range []string{".ts", ".tsx", ".js", ".jsx"} {
			candidates = append(candidates, withoutExtension+candidateExtension)
		}
	}
	if extension == "" {
		for _, candidateExtension := range []string{".ts", ".tsx", ".js", ".jsx"} {
			candidates = append(candidates, filepath.ToSlash(filepath.Join(base, "index"+candidateExtension)))
		}
	}
	return candidates
}

func isTypeScriptSourceExtension(extension string) bool {
	switch extension {
	case ".ts", ".tsx", ".js", ".jsx":
		return true
	default:
		return false
	}
}

func isTypeScriptAssetExtension(extension string) bool {
	switch extension {
	case ".css", ".scss", ".sass", ".less", ".styl", ".stylus", ".pcss", ".postcss",
		".json", ".svg", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".ico",
		".woff", ".woff2", ".ttf", ".eot", ".mp3", ".mp4", ".webm", ".wav":
		return true
	default:
		return false
	}
}

func pathAliasCapture(pattern, importPath string) (string, bool) {
	if !strings.Contains(pattern, "*") {
		return "", pattern == importPath
	}
	parts := strings.SplitN(pattern, "*", 2)
	if !strings.HasPrefix(importPath, parts[0]) || !strings.HasSuffix(importPath, parts[1]) {
		return "", false
	}
	return strings.TrimSuffix(strings.TrimPrefix(importPath, parts[0]), parts[1]), true
}

func configuredPathAlias(importPath string, config typeScriptProjectConfig) bool {
	for pattern := range config.Paths {
		if _, ok := pathAliasCapture(pattern, importPath); ok {
			return true
		}
	}
	return false
}

func typeScriptTargetKind(importPath string, internal *model.Package, config typeScriptProjectConfig) string {
	if internal != nil {
		return "internal"
	}
	if strings.HasPrefix(importPath, "node:") {
		return "standard-library"
	}
	if isTypeScriptGeneratedImport(importPath) {
		return "generated"
	}
	if isTypeScriptAssetExtension(strings.ToLower(filepath.Ext(importPath))) {
		return "asset"
	}
	if strings.HasPrefix(importPath, ".") || configuredPathAlias(importPath, config) {
		return "unresolved"
	}
	return "external"
}

func isTypeScriptGeneratedImport(importPath string) bool {
	for _, prefix := range []string{"./.next/", "../.next/", "./.nuxt/", "../.nuxt/", "./.svelte-kit/", "../.svelte-kit/"} {
		if strings.HasPrefix(importPath, prefix) {
			return true
		}
	}
	return false
}
