package graph

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"ingen/paddock/internal/model"
)

var (
	pythonImportStatement = regexp.MustCompile(`(?m)^\s*import\s+([^#\n]+)`)
	pythonFromStatement   = regexp.MustCompile(`(?m)^\s*from\s+([.A-Za-z_][.A-Za-z0-9_]*)\s+import\s+([^#\n]+)`)
)

type pythonFile struct {
	absPath       string
	relPath       string
	module        string
	packageModule bool
}

type pythonImport struct {
	module     string
	candidates []string
	kind       string
	line       int
}

// LoadPython builds a file-level graph for Python source. It resolves local
// absolute and relative modules without importing or executing project code;
// unresolved imports remain visible as unresolved edges when they target a
// project module; imports outside the project are classified as standard
// library or external edges.
func LoadPython(root string) (*model.Graph, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve Python source root: %w", err)
	}
	modulePath := pythonModulePath(root)
	files, err := findPythonFiles(root)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("python adapter found no source files in %s", root)
	}

	result := &model.Graph{ModulePath: modulePath}
	byModule := make(map[string]*model.Package, len(files))
	for _, file := range files {
		importPath := modulePath
		if file.module != "" {
			importPath += "/" + file.module
		}
		pkg := &model.Package{ImportPath: importPath, RelPath: file.relPath}
		result.Packages = append(result.Packages, pkg)
		byModule[file.module] = pkg
	}

	for _, file := range files {
		from := byModule[file.module]
		imports, err := importsInPythonFile(file.absPath, file.module, file.packageModule)
		if err != nil {
			return nil, err
		}
		for _, imported := range imports {
			resolvedModule, target := resolvePythonImport(imported, byModule)
			edge := &model.Edge{
				FromImportPath: from.ImportPath,
				FromPath:       from.RelPath,
				ToImportPath:   imported.module,
				Kind:           imported.kind,
				TargetKind:     pythonTargetKind(imported.module, target, byModule),
				File:           file.relPath,
				Line:           imported.line,
			}
			if target != nil {
				edge.ToImportPath = modulePath + "/" + resolvedModule
				edge.ToPath = target.RelPath
			}
			result.Edges = append(result.Edges, edge)
		}
	}
	return result, nil
}

func findPythonFiles(root string) ([]pythonFile, error) {
	files := []pythonFile{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".venv", "venv", "__pycache__", "dist", "build", "coverage", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".py" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("relative Python path %s: %w", path, err)
		}
		rel = filepath.ToSlash(rel)
		files = append(files, pythonFile{
			absPath:       path,
			relPath:       rel,
			module:        pythonModuleName(rel),
			packageModule: filepath.Base(rel) == "__init__.py",
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk Python source: %w", err)
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].relPath < files[j].relPath
	})
	return files, nil
}

func pythonModulePath(root string) string {
	if data, err := os.ReadFile(filepath.Join(root, "pyproject.toml")); err == nil {
		namePattern := regexp.MustCompile(`(?m)^\s*name\s*=\s*["']([^"']+)["']`)
		if match := namePattern.FindSubmatch(data); len(match) == 2 {
			return string(match[1])
		}
	}
	base := filepath.Base(root)
	if base == "." || base == string(filepath.Separator) || base == "" {
		return "python-project"
	}
	return base
}

func pythonModuleName(relPath string) string {
	withoutExtension := strings.TrimSuffix(filepath.ToSlash(relPath), ".py")
	parts := strings.Split(withoutExtension, "/")
	if parts[len(parts)-1] == "__init__" {
		parts = parts[:len(parts)-1]
	}
	return strings.Join(parts, ".")
}

func importsInPythonFile(path, fromModule string, packageModule bool) ([]pythonImport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	contents := string(data)
	imports := []pythonImport{}
	for _, match := range pythonImportStatement.FindAllStringSubmatchIndex(contents, -1) {
		line := 1 + strings.Count(contents[:match[0]], "\n")
		for _, item := range strings.Split(contents[match[2]:match[3]], ",") {
			module := strings.TrimSpace(strings.SplitN(item, " as ", 2)[0])
			if module == "" || strings.HasPrefix(module, ".") {
				continue
			}
			imports = append(imports, pythonImport{module: module, candidates: []string{module}, kind: "import", line: line})
		}
	}
	for _, match := range pythonFromStatement.FindAllStringSubmatchIndex(contents, -1) {
		line := 1 + strings.Count(contents[:match[0]], "\n")
		base := contents[match[2]:match[3]]
		members := strings.Split(contents[match[4]:match[5]], ",")
		for _, member := range members {
			name := strings.TrimSpace(strings.SplitN(member, " as ", 2)[0])
			if name == "" || name == "*" {
				continue
			}
			module := base
			candidates := []string{base + "." + name}
			if strings.HasPrefix(base, ".") {
				module = resolvePythonFromModule(base, name, fromModule, packageModule)
				candidates = []string{module}
				if parent := pythonModuleParent(module); parent != "" {
					candidates = append(candidates, parent)
				}
			} else {
				candidates = append(candidates, base)
			}
			imports = append(imports, pythonImport{module: module, candidates: candidates, kind: "import", line: line})
		}
	}
	return imports, nil
}

func resolvePythonFromModule(base, member, fromModule string, packageModule bool) string {
	if strings.HasPrefix(base, ".") {
		dots := 0
		for dots < len(base) && base[dots] == '.' {
			dots++
		}
		packageParts := strings.Split(fromModule, ".")
		if len(packageParts) > 0 && !packageModule {
			packageParts = packageParts[:len(packageParts)-1]
		}
		if remove := dots - 1; remove > 0 && remove <= len(packageParts) {
			packageParts = packageParts[:len(packageParts)-remove]
		}
		baseParts := strings.TrimPrefix(base, strings.Repeat(".", dots))
		parts := append([]string{}, packageParts...)
		if baseParts != "" {
			parts = append(parts, strings.Split(baseParts, ".")...)
		}
		parts = append(parts, member)
		return strings.Join(parts, ".")
	}
	if base == "" {
		return member
	}
	return base + "." + member
}

func resolvePythonImport(imported pythonImport, byModule map[string]*model.Package) (string, *model.Package) {
	candidates := imported.candidates
	if len(candidates) == 0 {
		candidates = []string{imported.module}
	}
	for _, candidate := range candidates {
		if target := byModule[candidate]; target != nil {
			return candidate, target
		}
	}
	return "", nil
}

func pythonModuleParent(module string) string {
	index := strings.LastIndexByte(module, '.')
	if index <= 0 {
		return ""
	}
	return module[:index]
}

func pythonTargetKind(imported string, target *model.Package, byModule map[string]*model.Package) string {
	if target != nil {
		return "internal"
	}
	if strings.HasPrefix(imported, ".") || pythonHasModuleRoot(imported, byModule) {
		return "unresolved"
	}
	root := strings.Split(imported, ".")[0]
	if pythonStandardLibrary[root] {
		return "standard-library"
	}
	return "external"
}

func pythonHasModuleRoot(imported string, byModule map[string]*model.Package) bool {
	root := strings.Split(imported, ".")[0]
	for module := range byModule {
		if module == root || strings.HasPrefix(module, root+".") {
			return true
		}
	}
	return false
}

var pythonStandardLibrary = map[string]bool{
	"abc": true, "asyncio": true, "collections": true, "contextlib": true,
	"csv": true, "dataclasses": true, "datetime": true, "enum": true,
	"functools": true, "hashlib": true, "http": true, "itertools": true,
	"json": true, "logging": true, "math": true, "os": true,
	"pathlib": true, "re": true, "secrets": true, "sqlite3": true,
	"subprocess": true, "sys": true, "threading": true, "time": true,
	"typing": true, "unittest": true, "uuid": true,
}
