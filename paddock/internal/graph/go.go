package graph

import (
	"bufio"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"ingen/paddock/internal/model"
)

type listPackage struct {
	ImportPath string
	Dir        string
	GoFiles    []string
	CgoFiles   []string
	Imports    []string
	Error      *struct {
		Err string
	}
}

// LoadGo uses the Go toolchain to resolve packages and the standard parser to
// attach source locations to each import. This keeps the first adapter
// dependency-free while still respecting nested modules, build selection, and
// Go's package identity rules.
func LoadGo(root string) (*model.Graph, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve source root: %w", err)
	}
	modulePath, err := modulePath(root)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command("go", "list", "-e", "-json", "./...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("go list: %w\n%s", err, strings.TrimSpace(string(output)))
	}

	var listed []listPackage
	decoder := json.NewDecoder(bufio.NewReader(strings.NewReader(string(output))))
	for {
		var item listPackage
		if err := decoder.Decode(&item); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("decode go list output: %w", err)
		}
		// `go list -e` still gives us useful package metadata for import
		// cycles. Keep those packages so Paddock can report the architectural
		// cycle instead of stopping at the compiler's error. Pattern-level
		// failures have no directory and remain hard errors.
		if item.Error != nil && item.Dir == "" {
			return nil, fmt.Errorf("package %s: %s", item.ImportPath, item.Error.Err)
		}
		if isModulePackage(item.ImportPath, modulePath) {
			listed = append(listed, item)
		}
	}
	if len(listed) == 0 {
		return nil, fmt.Errorf("go list found no packages in %s", root)
	}

	result := &model.Graph{ModulePath: modulePath}
	byImport := make(map[string]*model.Package, len(listed))
	for _, item := range listed {
		rel, err := filepath.Rel(root, item.Dir)
		if err != nil {
			return nil, fmt.Errorf("relative package path %s: %w", item.Dir, err)
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			rel = ""
		}
		pkg := &model.Package{ImportPath: item.ImportPath, RelPath: rel}
		result.Packages = append(result.Packages, pkg)
		byImport[item.ImportPath] = pkg
	}

	for _, item := range listed {
		pkg := byImport[item.ImportPath]
		files := append([]string{}, item.GoFiles...)
		files = append(files, item.CgoFiles...)
		for _, name := range files {
			file := filepath.Join(item.Dir, name)
			imports, err := importsInFile(file)
			if err != nil {
				return nil, err
			}
			for _, imported := range imports {
				target := byImport[imported.Path]
				edge := &model.Edge{
					FromImportPath: item.ImportPath,
					FromPath:       pkg.RelPath,
					ToImportPath:   imported.Path,
					TargetKind:     targetKind(imported.Path, target),
					File:           filepath.ToSlash(filepath.Join(pkg.RelPath, name)),
					Line:           imported.Line,
				}
				if target != nil {
					edge.ToPath = target.RelPath
				}
				result.Edges = append(result.Edges, edge)
			}
		}
	}

	return result, nil
}

type importedPackage struct {
	Path string
	Line int
}

func importsInFile(file string) ([]importedPackage, error) {
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, nil, parser.ImportsOnly)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", file, err)
	}
	result := make([]importedPackage, 0, len(parsed.Imports))
	for _, spec := range parsed.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, fmt.Errorf("decode import in %s: %w", file, err)
		}
		result = append(result, importedPackage{
			Path: path,
			Line: fset.Position(spec.Pos()).Line,
		})
	}
	return result, nil
}

func targetKind(importPath string, internal *model.Package) string {
	if internal != nil {
		return "internal"
	}
	first := importPath
	if index := strings.IndexByte(first, '/'); index >= 0 {
		first = first[:index]
	}
	if strings.Contains(first, ".") {
		return "external"
	}
	return "standard-library"
}

func isModulePackage(importPath, modulePath string) bool {
	return importPath == modulePath || strings.HasPrefix(importPath, modulePath+"/")
}

func modulePath(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}
	match := regexp.MustCompile(`(?m)^\s*module\s+([^\s]+)\s*$`).FindSubmatch(data)
	if len(match) != 2 {
		return "", fmt.Errorf("go.mod has no module directive")
	}
	return string(match[1]), nil
}
