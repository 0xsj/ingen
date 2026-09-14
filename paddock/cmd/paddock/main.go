package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	paddockartifact "ingen/paddock/internal/artifact"
	paddockbaseline "ingen/paddock/internal/baseline"
	"ingen/paddock/internal/checker"
	paddockexplain "ingen/paddock/internal/explain"
	paddockgraph "ingen/paddock/internal/graph"
	"ingen/paddock/internal/model"
	paddockpolicy "ingen/paddock/internal/policy"
	"ingen/paddock/internal/report"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "check":
		if err := check(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "paddock:", err)
			os.Exit(2)
		}
	case "graph":
		if err := graphCommand(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "paddock:", err)
			os.Exit(2)
		}
	case "baseline":
		if err := createBaseline(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "paddock:", err)
			os.Exit(2)
		}
	case "ci":
		exitCode, err := createCIArtifact(os.Args[2:])
		if err != nil {
			fmt.Fprintln(os.Stderr, "paddock:", err)
			os.Exit(2)
		}
		if exitCode != 0 {
			os.Exit(exitCode)
		}
	case "explain":
		if err := explainReport(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "paddock:", err)
			os.Exit(2)
		}
	default:
		usage()
		os.Exit(2)
	}
}

func check(args []string) error {
	root := "."
	policyPath := ""
	baselinePath := ""
	format := "text"
	rootSet := false
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--policy", "-p":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires a path", arg)
			}
			index++
			policyPath = args[index]
		case "--format", "-f":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires text or json", arg)
			}
			index++
			format = args[index]
		case "--baseline":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires a path", arg)
			}
			index++
			baselinePath = args[index]
		default:
			if strings.HasPrefix(arg, "-") {
				return fmt.Errorf("unknown option %q", arg)
			}
			if rootSet {
				return fmt.Errorf("unexpected argument %q", arg)
			}
			root = arg
			rootSet = true
		}
	}
	if policyPath == "" {
		return fmt.Errorf("check requires --policy <path>")
	}
	result, err := checker.Check(root, policyPath)
	if err != nil {
		return err
	}
	if baselinePath != "" {
		if err := applyBaseline(result, baselinePath, policyPath); err != nil {
			return err
		}
	}
	switch format {
	case "text":
		err = report.Text(os.Stdout, result)
	case "json":
		err = report.JSON(os.Stdout, result)
	default:
		return fmt.Errorf("unsupported format %q; use text or json", format)
	}
	if err != nil {
		return err
	}
	if !result.OK() {
		os.Exit(1)
	}
	return nil
}

type graphDocument struct {
	Schema       string                    `json:"schema"`
	Language     string                    `json:"language"`
	Unit         string                    `json:"source_unit,omitempty"`
	Root         string                    `json:"root"`
	Roots        []string                  `json:"roots,omitempty"`
	ModulePath   string                    `json:"module_path"`
	Capabilities paddockgraph.Capabilities `json:"capabilities"`
	PackageCount int                       `json:"package_count"`
	EdgeCount    int                       `json:"edge_count"`
	Packages     []*model.Package          `json:"packages"`
	Edges        []*model.Edge             `json:"edges"`
}

func graphCommand(args []string) error {
	root := "."
	language := ""
	policyPath := ""
	format := "text"
	rootSet := false
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--language", "-l":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires a language", arg)
			}
			index++
			language = args[index]
		case "--policy", "-p":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires a path", arg)
			}
			index++
			policyPath = args[index]
		case "--format", "-f":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires text or json", arg)
			}
			index++
			format = args[index]
		default:
			if strings.HasPrefix(arg, "-") {
				return fmt.Errorf("unknown option %q", arg)
			}
			if rootSet {
				return fmt.Errorf("unexpected argument %q", arg)
			}
			root = arg
			rootSet = true
		}
	}

	request := paddockgraph.LoadRequest{Root: root}
	if policyPath != "" {
		config, err := paddockpolicy.Load(policyPath)
		if err != nil {
			return err
		}
		if language != "" && language != config.Source.Language {
			return fmt.Errorf("graph language %q does not match policy language %q", language, config.Source.Language)
		}
		language = config.Source.Language
		request.Unit = config.Source.Unit
		request.Roots = append([]string(nil), config.Source.Roots...)
	}
	if language == "" {
		return fmt.Errorf("graph requires --language <go|typescript> or --policy <path>")
	}

	registry := paddockgraph.DefaultRegistry()
	adapter, err := registry.Lookup(language)
	if err != nil {
		return err
	}
	loaded, err := registry.Load(language, request)
	if err != nil {
		return err
	}
	loaded = paddockgraph.StableCopy(loaded)
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve source root: %w", err)
	}
	document := graphDocument{
		Schema:       "paddock.graph/v1",
		Language:     language,
		Unit:         request.Unit,
		Root:         absRoot,
		Roots:        request.Roots,
		ModulePath:   loaded.ModulePath,
		Capabilities: adapter.Capabilities(),
		PackageCount: len(loaded.Packages),
		EdgeCount:    len(loaded.Edges),
		Packages:     loaded.Packages,
		Edges:        loaded.Edges,
	}

	switch format {
	case "text":
		return printGraphText(os.Stdout, document)
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(document)
	default:
		return fmt.Errorf("unsupported format %q; use text or json", format)
	}
}

func printGraphText(w io.Writer, document graphDocument) error {
	if _, err := fmt.Fprintf(w, "GRAPH %s %s (%d packages, %d edges)\n", document.Language, document.Root, document.PackageCount, document.EdgeCount); err != nil {
		return err
	}
	if document.ModulePath != "" {
		if _, err := fmt.Fprintf(w, "MODULE %s\n", document.ModulePath); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "PACKAGES"); err != nil {
		return err
	}
	for _, pkg := range document.Packages {
		if _, err := fmt.Fprintf(w, "  %s (%s)\n", pkg.ImportPath, pkg.RelPath); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "EDGES"); err != nil {
		return err
	}
	for _, edge := range document.Edges {
		to := edge.ToPath
		if to == "" {
			to = edge.ToImportPath
		}
		location := edge.File
		if edge.Line > 0 {
			location = fmt.Sprintf("%s:%d", location, edge.Line)
		}
		if _, err := fmt.Fprintf(w, "  %s -> %s [%s] (%s)\n", edge.FromPath, to, edge.TargetKind, location); err != nil {
			return err
		}
	}
	return nil
}

func applyBaseline(result *model.Result, baselinePath, policyPath string) error {
	snapshot, err := paddockbaseline.Load(baselinePath)
	if err != nil {
		return err
	}
	policyRef, err := paddockartifact.File(policyPath)
	if err != nil {
		return err
	}
	return paddockbaseline.Apply(result, snapshot, baselinePath, policyRef.SHA256)
}

func createBaseline(args []string) error {
	root := "."
	policyPath := ""
	outputPath := ""
	rootSet := false
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--policy", "-p":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires a path", arg)
			}
			index++
			policyPath = args[index]
		case "--output", "-o":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires a path", arg)
			}
			index++
			outputPath = args[index]
		default:
			if strings.HasPrefix(arg, "-") {
				return fmt.Errorf("unknown option %q", arg)
			}
			if rootSet {
				return fmt.Errorf("unexpected argument %q", arg)
			}
			root = arg
			rootSet = true
		}
	}
	if policyPath == "" {
		return fmt.Errorf("baseline requires --policy <path>")
	}
	if outputPath == "" {
		return fmt.Errorf("baseline requires --output <path>")
	}
	result, err := checker.Check(root, policyPath)
	if err != nil {
		return err
	}
	policyRef, err := paddockartifact.File(policyPath)
	if err != nil {
		return err
	}
	snapshot := paddockbaseline.Build(result, policyRef.SHA256)
	if err := paddockbaseline.Save(outputPath, snapshot); err != nil {
		return err
	}
	_, err = fmt.Fprintf(os.Stdout, "BASELINE %s (%d findings)\n", outputPath, len(snapshot.Entries))
	return err
}

func createCIArtifact(args []string) (int, error) {
	root := "."
	policyPath := ""
	baselinePath := ""
	outputPath := ""
	rootSet := false
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--policy", "-p":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a path", arg)
			}
			index++
			policyPath = args[index]
		case "--baseline":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a path", arg)
			}
			index++
			baselinePath = args[index]
		case "--output", "-o":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a path", arg)
			}
			index++
			outputPath = args[index]
		default:
			if strings.HasPrefix(arg, "-") {
				return 0, fmt.Errorf("unknown option %q", arg)
			}
			if rootSet {
				return 0, fmt.Errorf("unexpected argument %q", arg)
			}
			root = arg
			rootSet = true
		}
	}
	if policyPath == "" {
		return 0, fmt.Errorf("ci requires --policy <path>")
	}
	if outputPath == "" {
		return 0, fmt.Errorf("ci requires --output <path>")
	}

	createdAt := time.Now().UTC()
	policyRef := paddockartifact.FileRef{Path: policyPath}
	if ref, err := paddockartifact.File(policyPath); err == nil {
		policyRef = ref
	} else {
		return saveCIError(outputPath, root, policyRef, nil, err, createdAt)
	}
	var baselineRef *paddockartifact.FileRef
	if baselinePath != "" {
		ref, err := paddockartifact.File(baselinePath)
		if err != nil {
			return saveCIError(outputPath, root, policyRef, &ref, err, createdAt)
		}
		baselineRef = &ref
	}

	result, err := checker.Check(root, policyPath)
	if err != nil {
		return saveCIError(outputPath, root, policyRef, baselineRef, err, createdAt)
	}
	if baselinePath != "" {
		if err := applyBaseline(result, baselinePath, policyPath); err != nil {
			return saveCIError(outputPath, root, policyRef, baselineRef, err, createdAt)
		}
	}
	ciArtifact := paddockartifact.New(result, policyRef, baselineRef, createdAt)
	if err := paddockartifact.Save(outputPath, ciArtifact); err != nil {
		return 0, err
	}
	if _, err := fmt.Fprintf(os.Stdout, "CI-RESULT %s (%s)\n", outputPath, ciArtifact.Status); err != nil {
		return 0, err
	}
	return ciArtifact.ExitCode, nil
}

func saveCIError(outputPath, root string, policy paddockartifact.FileRef, baseline *paddockartifact.FileRef, cause error, createdAt time.Time) (int, error) {
	ciArtifact := paddockartifact.NewError(root, policy, baseline, cause, createdAt)
	if err := paddockartifact.Save(outputPath, ciArtifact); err != nil {
		return 0, err
	}
	if _, err := fmt.Fprintf(os.Stdout, "CI-RESULT %s (error)\n", outputPath); err != nil {
		return 0, err
	}
	return ciArtifact.ExitCode, nil
}

func explainReport(args []string) error {
	reportPath := ""
	format := "text"
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--format", "-f":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires text or json", arg)
			}
			index++
			format = args[index]
		default:
			if strings.HasPrefix(arg, "-") {
				return fmt.Errorf("unknown option %q", arg)
			}
			if reportPath != "" {
				return fmt.Errorf("unexpected argument %q", arg)
			}
			reportPath = arg
		}
	}
	if reportPath == "" {
		return fmt.Errorf("explain requires <paddock-report.json>")
	}
	data, err := os.ReadFile(reportPath)
	if err != nil {
		return fmt.Errorf("read report: %w", err)
	}
	var result model.Result
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("parse report: %w", err)
	}
	if result.Schema != "paddock.report/v1" {
		return fmt.Errorf("report schema must be paddock.report/v1, got %q", result.Schema)
	}
	document := paddockexplain.Explain(&result)
	switch format {
	case "text":
		err = paddockexplain.Text(os.Stdout, document)
	case "json":
		err = paddockexplain.JSON(os.Stdout, document)
	default:
		return fmt.Errorf("unsupported format %q; use text or json", format)
	}
	return err
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: paddock check <source-root> --policy <policy.yaml> [--baseline <file>] [--format text|json]")
	fmt.Fprintln(os.Stderr, "       paddock graph <source-root> [--policy <policy.yaml> | --language go|typescript] [--format text|json]")
	fmt.Fprintln(os.Stderr, "       paddock baseline <source-root> --policy <policy.yaml> --output <baseline.json>")
	fmt.Fprintln(os.Stderr, "       paddock ci <source-root> --policy <policy.yaml> [--baseline <file>] --output <ci-result.json>")
	fmt.Fprintln(os.Stderr, "       paddock explain <paddock-report.json> [--format text|json]")
}
