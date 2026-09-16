package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ingen/core/ciresult"
	paddockadaptertest "ingen/paddock/internal/adaptertest"
	paddockartifact "ingen/paddock/internal/artifact"
	paddockbaseline "ingen/paddock/internal/baseline"
	"ingen/paddock/internal/checker"
	paddockcomponentmap "ingen/paddock/internal/componentmap"
	paddockexplain "ingen/paddock/internal/explain"
	paddockgraph "ingen/paddock/internal/graph"
	"ingen/paddock/internal/model"
	paddockpolicy "ingen/paddock/internal/policy"
	paddockpolicydiff "ingen/paddock/internal/policydiff"
	paddockpolicylock "ingen/paddock/internal/policylock"
	paddockpolicyreview "ingen/paddock/internal/policyreview"
	paddockpolicytest "ingen/paddock/internal/policytest"
	paddockrelease "ingen/paddock/internal/release"
	"ingen/paddock/internal/report"
	paddockscaffold "ingen/paddock/internal/scaffold"
	paddockversion "ingen/paddock/internal/version"
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
			var exitFailure *graphCommandExitError
			if errors.As(err, &exitFailure) {
				os.Exit(exitFailure.code)
			}
			fmt.Fprintln(os.Stderr, "paddock:", err)
			os.Exit(2)
		}
	case "map":
		if err := componentMapCommand(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "paddock:", err)
			os.Exit(2)
		}
	case "init":
		if err := initCommand(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "paddock:", err)
			os.Exit(2)
		}
	case "policy":
		exitCode, err := policyCommand(os.Args[2:])
		if err != nil {
			fmt.Fprintln(os.Stderr, "paddock:", err)
			os.Exit(2)
		}
		if exitCode != 0 {
			os.Exit(exitCode)
		}
	case "release":
		exitCode, err := releaseCommand(os.Args[2:])
		if err != nil {
			fmt.Fprintln(os.Stderr, "paddock:", err)
			os.Exit(2)
		}
		if exitCode != 0 {
			os.Exit(exitCode)
		}
	case "baseline":
		if err := createBaseline(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "paddock:", err)
			os.Exit(2)
		}
	case "ci":
		if len(os.Args) > 2 && os.Args[2] == "validate" {
			if err := validateCIArtifact(os.Args[3:]); err != nil {
				fmt.Fprintln(os.Stderr, "paddock:", err)
				os.Exit(2)
			}
			break
		}
		exitCode, err := createCIArtifact(os.Args[2:])
		if err != nil {
			fmt.Fprintln(os.Stderr, "paddock:", err)
			os.Exit(2)
		}
		if exitCode != 0 {
			os.Exit(exitCode)
		}
	case "adapter":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "paddock: adapter requires validate or test subcommand")
			os.Exit(2)
		}
		switch os.Args[2] {
		case "validate":
			exitCode, err := validateAdapter(os.Args[3:])
			if err != nil {
				fmt.Fprintln(os.Stderr, "paddock:", err)
				os.Exit(2)
			}
			if exitCode != 0 {
				os.Exit(exitCode)
			}
		case "test":
			exitCode, err := adapterTestCommand(os.Args[3:])
			if err != nil {
				fmt.Fprintln(os.Stderr, "paddock:", err)
				os.Exit(2)
			}
			if exitCode != 0 {
				os.Exit(exitCode)
			}
		default:
			fmt.Fprintf(os.Stderr, "paddock: unsupported adapter subcommand %q\n", os.Args[2])
			os.Exit(2)
		}
	case "explain":
		if err := explainReport(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "paddock:", err)
			os.Exit(2)
		}
	case "version":
		if err := versionCommand(os.Args[2:]); err != nil {
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
	policyLockPath := ""
	graphInputPath := ""
	adapterExecutable := ""
	adapterArgs := []string(nil)
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
		case "--policy-lock":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires a path", arg)
			}
			index++
			policyLockPath = args[index]
		case "--graph":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires a graph document path", arg)
			}
			index++
			graphInputPath = args[index]
		case "--adapter":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires an executable path", arg)
			}
			index++
			adapterExecutable = args[index]
		case "--adapter-arg":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires an argument", arg)
			}
			index++
			adapterArgs = append(adapterArgs, args[index])
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
	if policyPath == "" && policyLockPath == "" {
		return fmt.Errorf("check requires --policy <path> or --policy-lock <path>")
	}
	if graphInputPath != "" && adapterExecutable != "" {
		return fmt.Errorf("check accepts either --graph or --adapter, not both")
	}
	evaluation, err := loadEvaluationPolicy(policyPath, policyLockPath)
	if err != nil {
		return err
	}
	var result *model.Result
	if graphInputPath != "" {
		loaded, err := loadGraphInput(graphInputPath, evaluation.Config)
		if err != nil {
			return err
		}
		result, err = checker.CheckGraph(root, evaluation.Path, evaluation.Config, loaded)
	} else if adapterExecutable != "" {
		loaded, err := loadExternalGraph(root, evaluation.Config, adapterExecutable, adapterArgs)
		if err != nil {
			return err
		}
		result, err = checker.CheckGraph(root, evaluation.Path, evaluation.Config, loaded)
	} else {
		result, err = checker.CheckPolicy(root, evaluation.Path, evaluation.Config)
	}
	if err != nil {
		return err
	}
	if baselinePath != "" {
		if err := applyBaselineHash(result, baselinePath, evaluation.CanonicalSHA256); err != nil {
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

type graphDocument = paddockgraph.Document

func graphCommand(args []string) error {
	root := "."
	language := ""
	policyPath := ""
	graphInputPath := ""
	adapterExecutable := ""
	adapterArgs := []string(nil)
	format := "text"
	rootSet := false
	var scopeInclude, scopeExclude []string
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
		case "--input", "--graph":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires a graph document path", arg)
			}
			index++
			graphInputPath = args[index]
		case "--adapter":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires an executable path", arg)
			}
			index++
			adapterExecutable = args[index]
		case "--adapter-arg":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires an argument", arg)
			}
			index++
			adapterArgs = append(adapterArgs, args[index])
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
	if graphInputPath != "" && adapterExecutable != "" {
		return fmt.Errorf("graph accepts either --input/--graph or --adapter, not both")
	}
	if format != "text" && format != "json" {
		return fmt.Errorf("unsupported format %q; use text or json", format)
	}

	var loaded *model.Graph
	unit := ""
	roots := []string(nil)
	modulePath := ""
	capabilities := paddockgraph.Capabilities{}
	var adapterMetadata *paddockgraph.AdapterMetadata
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve source root: %w", err)
	}
	if graphInputPath != "" {
		var document paddockgraph.Document
		var err error
		loaded, document, err = paddockgraph.LoadDocument(graphInputPath)
		if err != nil {
			return err
		}
		if language != "" && language != document.Language {
			return fmt.Errorf("graph language %q does not match input language %q", language, document.Language)
		}
		language = document.Language
		unit = document.Unit
		roots = append([]string(nil), document.Roots...)
		modulePath = document.ModulePath
		adapterMetadata = document.Adapter
		capabilities = document.Capabilities
		if policyPath != "" {
			config, err := paddockpolicy.Load(policyPath)
			if err != nil {
				return err
			}
			if config.Source.Language != document.Language {
				return fmt.Errorf("graph input language %q does not match policy language %q", document.Language, config.Source.Language)
			}
			if document.Unit != "" && document.Unit != config.Source.Unit {
				return fmt.Errorf("graph input source unit %q does not match policy source unit %q", document.Unit, config.Source.Unit)
			}
			scopeInclude = append([]string(nil), config.Source.Include...)
			scopeExclude = append([]string(nil), config.Source.Exclude...)
		}
	} else {
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
			request.Include = append([]string(nil), config.Source.Include...)
			request.Exclude = append([]string(nil), config.Source.Exclude...)
		}
		if language == "" {
			return fmt.Errorf("graph requires --language <language>, --policy <path>, or --input <graph.json>")
		}
		if adapterExecutable != "" {
			requiredCapabilities := paddockgraph.Capabilities{}
			if request.Unit != "" {
				requiredCapabilities.SourceUnits = []string{request.Unit}
			}
			adapterRequest := paddockgraph.Request{
				Schema:               paddockgraph.RequestSchema,
				Language:             language,
				Unit:                 request.Unit,
				Root:                 absRoot,
				Roots:                append([]string(nil), request.Roots...),
				Include:              append([]string(nil), request.Include...),
				Exclude:              append([]string(nil), request.Exclude...),
				RequiredCapabilities: requiredCapabilities,
			}
			var document paddockgraph.Document
			loaded, document, err = paddockgraph.LoadExternal(context.Background(), adapterExecutable, adapterArgs, adapterRequest)
			if err != nil {
				return graphCommandFailure(format, absRoot, language, request.Unit, adapterExecutable, err)
			}
			unit = document.Unit
			roots = append([]string(nil), document.Roots...)
			modulePath = document.ModulePath
			adapterMetadata = document.Adapter
			capabilities = document.Capabilities
		} else {
			registry := paddockgraph.DefaultRegistry()
			adapter, err := registry.Lookup(language)
			if err != nil {
				return err
			}
			loaded, err = registry.Load(language, request)
			if err != nil {
				return err
			}
			unit = request.Unit
			roots = request.Roots
			modulePath = loaded.ModulePath
			adapterMetadata = &paddockgraph.AdapterMetadata{Kind: "builtin", Name: language}
			capabilities = adapter.Capabilities()
		}
	}
	if len(scopeInclude) > 0 || len(scopeExclude) > 0 {
		loaded, err = paddockgraph.FilterGraph(loaded, scopeInclude, scopeExclude)
		if err != nil {
			return err
		}
	}
	loaded = paddockgraph.StableCopy(loaded)
	document := graphDocument{
		Schema:       "paddock.graph/v1",
		Language:     language,
		Unit:         unit,
		Root:         absRoot,
		Roots:        roots,
		ModulePath:   modulePath,
		Adapter:      adapterMetadata,
		Capabilities: capabilities,
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

type graphCommandExitError struct {
	code int
}

func (e *graphCommandExitError) Error() string {
	return "graph command exited with a structured diagnostic"
}

func graphCommandFailure(format, root, language, unit, adapter string, err error) error {
	if format != "json" {
		return err
	}
	diagnostics := paddockgraph.ValidationDocumentForError("graph", root, language, unit, adapter, err)
	if encodeErr := json.NewEncoder(os.Stdout).Encode(diagnostics); encodeErr != nil {
		return encodeErr
	}
	return &graphCommandExitError{code: 2}
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

func componentMapCommand(args []string) error {
	root := "."
	policyPath := ""
	policyLockPath := ""
	graphInputPath := ""
	adapterExecutable := ""
	adapterArgs := []string(nil)
	format := "text"
	rootSet := false
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--policy", "-p":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires a path", args[index])
			}
			index++
			policyPath = args[index]
		case "--policy-lock":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires a path", args[index])
			}
			index++
			policyLockPath = args[index]
		case "--graph", "--input":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires a graph document path", args[index])
			}
			index++
			graphInputPath = args[index]
		case "--adapter":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires an executable path", args[index])
			}
			index++
			adapterExecutable = args[index]
		case "--adapter-arg":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires an argument", args[index])
			}
			index++
			adapterArgs = append(adapterArgs, args[index])
		case "--format", "-f":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires text or json", args[index])
			}
			index++
			format = args[index]
		default:
			if strings.HasPrefix(args[index], "-") {
				return fmt.Errorf("unknown option %q", args[index])
			}
			if rootSet {
				return fmt.Errorf("unexpected argument %q", args[index])
			}
			root = args[index]
			rootSet = true
		}
	}
	if policyPath == "" && policyLockPath == "" {
		return fmt.Errorf("map requires --policy <policy.yaml> or --policy-lock <lock.json>")
	}
	if graphInputPath != "" && adapterExecutable != "" {
		return fmt.Errorf("map accepts either --graph or --adapter, not both")
	}
	evaluation, err := loadEvaluationPolicy(policyPath, policyLockPath)
	if err != nil {
		return err
	}
	var loaded *model.Graph
	if graphInputPath != "" {
		loaded, err = loadGraphInput(graphInputPath, evaluation.Config)
	} else if adapterExecutable != "" {
		loaded, err = loadExternalGraph(root, evaluation.Config, adapterExecutable, adapterArgs)
	} else {
		loaded, err = paddockgraph.LoadWithRequest(paddockgraph.LoadRequest{
			Root:    root,
			Unit:    evaluation.Config.Source.Unit,
			Roots:   append([]string(nil), evaluation.Config.Source.Roots...),
			Include: append([]string(nil), evaluation.Config.Source.Include...),
			Exclude: append([]string(nil), evaluation.Config.Source.Exclude...),
		}, evaluation.Config.Source.Language)
	}
	if err != nil {
		return err
	}
	classificationFindings := checker.Classify(loaded.Packages, evaluation.Config)
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve source root: %w", err)
	}
	document, err := paddockcomponentmap.Build(absRoot, evaluation.Config.Source.Language, evaluation.Config.Source.Unit, loaded, classificationFindings)
	if err != nil {
		return err
	}
	switch format {
	case "text":
		_, err = fmt.Fprint(os.Stdout, paddockcomponentmap.Text(document))
		return err
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(document)
	default:
		return fmt.Errorf("unsupported format %q; use text or json", format)
	}
}

func initCommand(args []string) error {
	root := "."
	language := ""
	unit := ""
	graphInputPath := ""
	adapterExecutable := ""
	adapterArgs := []string(nil)
	template := ""
	outputPath := "paddock.yaml"
	format := "text"
	force := false
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
		case "--unit", "--source-unit":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires a source unit", arg)
			}
			index++
			unit = args[index]
		case "--graph", "--input":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires a graph document path", arg)
			}
			index++
			graphInputPath = args[index]
		case "--adapter":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires an executable path", arg)
			}
			index++
			adapterExecutable = args[index]
		case "--adapter-arg":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires an argument", arg)
			}
			index++
			adapterArgs = append(adapterArgs, args[index])
		case "--template", "-t":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires a template", arg)
			}
			index++
			template = args[index]
		case "--output", "-o":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires a path", arg)
			}
			index++
			outputPath = args[index]
		case "--format", "-f":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires text or json", arg)
			}
			index++
			format = args[index]
		case "--force":
			force = true
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
	if graphInputPath != "" && adapterExecutable != "" {
		return fmt.Errorf("init accepts either --graph/--input or --adapter, not both")
	}
	if format != "text" && format != "json" {
		return fmt.Errorf("unsupported format %q; use text or json", format)
	}

	var loaded *model.Graph
	if graphInputPath != "" {
		var document paddockgraph.Document
		var err error
		loaded, document, err = paddockgraph.LoadDocument(graphInputPath)
		if err != nil {
			return err
		}
		if language != "" && language != document.Language {
			return fmt.Errorf("graph language %q does not match input language %q", language, document.Language)
		}
		language = document.Language
		if unit != "" && document.Unit != "" && unit != document.Unit {
			return fmt.Errorf("graph source unit %q does not match input source unit %q", unit, document.Unit)
		}
		if unit == "" {
			unit = document.Unit
		}
	} else if adapterExecutable != "" {
		if language == "" {
			return fmt.Errorf("init with --adapter requires --language <language>")
		}
		if unit == "" {
			unit = sourceUnit(language)
		}
		var document paddockgraph.Document
		var err error
		loaded, document, err = loadExternalGraphRequest(root, language, unit, nil, nil, nil, adapterExecutable, adapterArgs)
		if err != nil {
			return err
		}
		unit = document.Unit
	} else {
		if language == "" {
			language = detectLanguage(root)
		}
		if language == "" {
			return fmt.Errorf("cannot detect source language; use --language go|typescript|python")
		}
		if unit == "" {
			unit = sourceUnit(language)
		}
		request := paddockgraph.LoadRequest{Root: root, Unit: unit}
		registry := paddockgraph.DefaultRegistry()
		var err error
		loaded, err = registry.Load(language, request)
		if err != nil {
			return err
		}
	}

	if language == "" {
		return fmt.Errorf("cannot determine source language from graph")
	}
	if unit == "" {
		unit = sourceUnit(language)
	}
	if template == "" {
		template = defaultTemplate(language)
	}
	contents, err := paddockscaffold.Generate(paddockscaffold.Options{
		Language: language,
		Unit:     unit,
		Template: template,
		Root:     root,
		Graph:    paddockgraph.StableCopy(loaded),
	})
	if err != nil {
		return err
	}
	if !force {
		if _, err := os.Stat(outputPath); err == nil {
			return fmt.Errorf("%s already exists; use --force to replace it", outputPath)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("check output %s: %w", outputPath, err)
		}
	}
	if err := os.WriteFile(outputPath, contents, 0o644); err != nil {
		return fmt.Errorf("write starter policy: %w", err)
	}
	if format == "json" {
		generated, err := paddockpolicy.Load(outputPath)
		if err != nil {
			return fmt.Errorf("validate generated policy: %w", err)
		}
		document := initDocumentForPolicy(root, outputPath, language, unit, template, loaded, generated)
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(document)
	}
	_, err = fmt.Fprintf(os.Stdout, "INIT %s (draft; review before CI)\n", outputPath)
	return err
}

const initSchema = "paddock.init/v1"

type initDocument struct {
	Schema                 string   `json:"schema"`
	Root                   string   `json:"root"`
	Output                 string   `json:"output"`
	Language               string   `json:"language"`
	SourceUnit             string   `json:"source_unit"`
	Template               string   `json:"template"`
	SourceUnitCount        int      `json:"source_unit_count"`
	EdgeCount              int      `json:"edge_count"`
	ComponentCount         int      `json:"component_count"`
	UnclassifiedComponents []string `json:"unclassified_components"`
	WarningRules           []string `json:"warning_rules"`
	ReviewRequired         bool     `json:"review_required"`
}

func initDocumentForPolicy(root, outputPath, language, unit, template string, graph *model.Graph, generated paddockpolicy.Policy) initDocument {
	unclassified := make([]string, 0)
	for name, component := range generated.Components {
		role, _ := component.Labels["role"].(string)
		if role == "" || role == "unclassified" {
			unclassified = append(unclassified, name)
		}
	}
	sort.Strings(unclassified)
	warningRules := make([]string, 0)
	for _, rule := range generated.Rules {
		if rule.Severity == "warning" {
			warningRules = append(warningRules, rule.ID)
		}
	}
	sort.Strings(warningRules)
	return initDocument{
		Schema:                 initSchema,
		Root:                   root,
		Output:                 outputPath,
		Language:               language,
		SourceUnit:             unit,
		Template:               template,
		SourceUnitCount:        len(graph.Packages),
		EdgeCount:              len(graph.Edges),
		ComponentCount:         len(generated.Components),
		UnclassifiedComponents: unclassified,
		WarningRules:           warningRules,
		ReviewRequired:         true,
	}
}

func sourceUnit(language string) string {
	if language == "typescript" || language == "python" {
		return "file"
	}
	return "package"
}

func defaultTemplate(language string) string {
	if language == "typescript" {
		return "feature-sliced"
	}
	return "layered"
}

func detectLanguage(root string) string {
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
		return "go"
	}
	if _, err := os.Stat(filepath.Join(root, "tsconfig.json")); err == nil {
		return "typescript"
	}
	if _, err := os.Stat(filepath.Join(root, "pyproject.toml")); err == nil {
		return "python"
	}
	var language string
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || language != "" {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "dist", "build", "coverage":
				return filepath.SkipDir
			}
			return nil
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".ts", ".tsx", ".js", ".jsx":
			language = "typescript"
		case ".py":
			language = "python"
		}
		return nil
	})
	return language
}

func policyCommand(args []string) (int, error) {
	if len(args) == 0 {
		return 0, fmt.Errorf("policy requires validate, diff, seal, verify, test, or review subcommand")
	}
	switch args[0] {
	case "validate":
		return validatePolicy(args[1:])
	case "diff":
		return diffPolicies(args[1:])
	case "seal":
		return 0, sealPolicy(args[1:])
	case "verify":
		return 0, verifyPolicy(args[1:])
	case "test":
		if len(args) > 1 && args[1] == "validate" {
			return 0, validatePolicyTests(args[2:])
		}
		return testPolicy(args[1:])
	case "review":
		if len(args) > 1 && args[1] == "verify" {
			return verifyPolicyReview(args[2:])
		}
		return reviewPolicy(args[1:])
	default:
		return 0, fmt.Errorf("unsupported policy subcommand %q", args[0])
	}
}

func validatePolicyTests(args []string) error {
	casesPath := ""
	format := "text"
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--cases", "-c":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires a test manifest path", args[index])
			}
			index++
			casesPath = args[index]
		case "--format", "-f":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires text or json", args[index])
			}
			index++
			format = args[index]
		default:
			return fmt.Errorf("unknown option %q", args[index])
		}
	}
	if casesPath == "" {
		return fmt.Errorf("policy test validate requires --cases <manifest.yaml>")
	}
	manifest, err := paddockpolicytest.Load(casesPath)
	if err != nil {
		return err
	}
	switch format {
	case "text":
		fmt.Fprintln(os.Stdout, "POLICY-TEST-MANIFEST VALID")
		fmt.Fprintf(os.Stdout, "manifest: %s\n", casesPath)
		fmt.Fprintf(os.Stdout, "cases: %d\n", len(manifest.Cases))
		for _, testCase := range manifest.Cases {
			detail := fmt.Sprintf("expected %s, root %s", testCase.Expect, testCase.Root)
			if len(testCase.RequireRules) > 0 {
				detail += ", requires " + strings.Join(testCase.RequireRules, ", ")
			}
			fmt.Fprintf(os.Stdout, "  %s (%s)\n", testCase.Name, detail)
		}
		return nil
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(manifest)
	default:
		return fmt.Errorf("unsupported format %q; use text or json", format)
	}
}

func validatePolicy(args []string) (int, error) {
	policyPath := ""
	format := "text"
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--policy", "-p":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a policy path", args[index])
			}
			index++
			policyPath = args[index]
		case "--format", "-f":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires text or json", args[index])
			}
			index++
			format = args[index]
		default:
			return 0, fmt.Errorf("unknown option %q", args[index])
		}
	}
	if policyPath == "" {
		return 0, fmt.Errorf("policy validate requires --policy <policy.yaml>")
	}
	if format != "text" && format != "json" {
		return 0, fmt.Errorf("unsupported format %q; use text or json", format)
	}
	config, err := paddockpolicy.Load(policyPath)
	if err != nil {
		if format != "json" {
			return 0, err
		}
		document := paddockpolicy.ValidationDocumentForError(policyPath, err)
		if encodeErr := json.NewEncoder(os.Stdout).Encode(document); encodeErr != nil {
			return 0, encodeErr
		}
		return 2, nil
	}

	switch format {
	case "text":
		fmt.Fprintln(os.Stdout, "POLICY VALID")
		fmt.Fprintf(os.Stdout, "policy: %s\n", policyPath)
		if config.Project != "" {
			fmt.Fprintf(os.Stdout, "project: %s\n", config.Project)
		}
		fmt.Fprintf(os.Stdout, "source: %s (%s)\n", config.Source.Language, config.Source.Unit)
		fmt.Fprintf(os.Stdout, "components: %d\n", len(config.Components))
		fmt.Fprintf(os.Stdout, "rules: %d\n", len(config.Rules))
		fmt.Fprintf(os.Stdout, "waivers: %d\n", len(config.Waivers))
		return 0, nil
	case "json":
		data, err := paddockpolicy.CanonicalJSON(config)
		if err != nil {
			return 0, err
		}
		_, err = os.Stdout.Write(append(data, '\n'))
		return 0, err
	}
	return 0, nil
}

func testPolicy(args []string) (int, error) {
	policyPath := ""
	casesPath := ""
	adapterExecutable := ""
	adapterArgs := []string(nil)
	format := "text"
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--policy", "-p":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a policy path", arg)
			}
			index++
			policyPath = args[index]
		case "--cases", "-c":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a test manifest path", arg)
			}
			index++
			casesPath = args[index]
		case "--adapter":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires an executable path", arg)
			}
			index++
			adapterExecutable = args[index]
		case "--adapter-arg":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires an argument", arg)
			}
			index++
			adapterArgs = append(adapterArgs, args[index])
		case "--format", "-f":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires text or json", arg)
			}
			index++
			format = args[index]
		default:
			return 0, fmt.Errorf("unknown option %q", arg)
		}
	}
	if policyPath == "" || casesPath == "" {
		return 0, fmt.Errorf("policy test requires --policy <path> and --cases <manifest.yaml>")
	}
	config, err := paddockpolicy.Load(policyPath)
	if err != nil {
		return 0, err
	}
	document, err := paddockpolicytest.RunWithOptions(casesPath, policyPath, config, paddockpolicytest.Options{
		AdapterExecutable: adapterExecutable,
		AdapterArgs:       adapterArgs,
	})
	if err != nil {
		return 0, err
	}
	switch format {
	case "text":
		err = paddockpolicytest.Text(os.Stdout, document)
	case "json":
		err = paddockpolicytest.JSON(os.Stdout, document)
	default:
		return 0, fmt.Errorf("unsupported format %q; use text or json", format)
	}
	if err != nil {
		return 0, err
	}
	if document.Status == "FAIL" {
		return 1, nil
	}
	return 0, nil
}

func diffPolicies(args []string) (int, error) {
	beforePath := ""
	afterPath := ""
	casesPath := ""
	adapterExecutable := ""
	adapterArgs := []string(nil)
	format := "text"
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--before", "-b":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a policy path", arg)
			}
			index++
			beforePath = args[index]
		case "--after", "-a":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a policy path", arg)
			}
			index++
			afterPath = args[index]
		case "--cases", "-c":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a test manifest path", arg)
			}
			index++
			casesPath = args[index]
		case "--adapter":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires an executable path", arg)
			}
			index++
			adapterExecutable = args[index]
		case "--adapter-arg":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires an argument", arg)
			}
			index++
			adapterArgs = append(adapterArgs, args[index])
		case "--format", "-f":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires text or json", arg)
			}
			index++
			format = args[index]
		default:
			return 0, fmt.Errorf("unknown option %q", arg)
		}
	}
	if beforePath == "" || afterPath == "" {
		return 0, fmt.Errorf("policy diff requires --before <policy.yaml> and --after <policy.yaml>")
	}
	if format != "text" && format != "json" {
		return 0, fmt.Errorf("unsupported format %q; use text or json", format)
	}
	if (adapterExecutable != "" || len(adapterArgs) > 0) && casesPath == "" {
		return 0, fmt.Errorf("policy diff adapter options require --cases <manifest.yaml>")
	}
	before, err := paddockpolicy.Load(beforePath)
	if err != nil {
		return policyComparisonLoadError(format, "diff", beforePath, beforePath, afterPath, err)
	}
	after, err := paddockpolicy.Load(afterPath)
	if err != nil {
		return policyComparisonLoadError(format, "diff", afterPath, beforePath, afterPath, err)
	}
	beforeRef, err := paddockartifact.File(beforePath)
	if err != nil {
		return 0, err
	}
	afterRef, err := paddockartifact.File(afterPath)
	if err != nil {
		return 0, err
	}
	beforeCanonicalHash, err := paddockpolicy.CanonicalSHA256(before)
	if err != nil {
		return 0, err
	}
	afterCanonicalHash, err := paddockpolicy.CanonicalSHA256(after)
	if err != nil {
		return 0, err
	}
	document := paddockpolicydiff.Compare(before, after)
	document.Before = paddockpolicydiff.Input{Path: beforeRef.Path, SHA256: beforeRef.SHA256, CanonicalSHA256: beforeCanonicalHash}
	document.After = paddockpolicydiff.Input{Path: afterRef.Path, SHA256: afterRef.SHA256, CanonicalSHA256: afterCanonicalHash}
	if casesPath != "" {
		tests, err := paddockpolicytest.RunWithOptions(casesPath, afterPath, after, paddockpolicytest.Options{
			AdapterExecutable: adapterExecutable,
			AdapterArgs:       adapterArgs,
		})
		if err != nil {
			return 0, err
		}
		document.Tests = &tests
	}
	switch format {
	case "text":
		err = paddockpolicydiff.Text(os.Stdout, document)
	case "json":
		if err := document.Validate(); err != nil {
			return 0, err
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		err = encoder.Encode(document)
	default:
		return 0, fmt.Errorf("unsupported format %q; use text or json", format)
	}
	if err != nil {
		return 0, err
	}
	if document.Tests != nil && document.Tests.Status == "FAIL" {
		return 1, nil
	}
	return 0, nil
}

func reviewPolicy(args []string) (int, error) {
	beforePath := ""
	afterPath := ""
	casesPath := ""
	adapterExecutable := ""
	adapterArgs := []string(nil)
	outputPath := ""
	format := "text"
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--before", "-b":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a policy path", arg)
			}
			index++
			beforePath = args[index]
		case "--after", "-a":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a policy path", arg)
			}
			index++
			afterPath = args[index]
		case "--cases", "-c":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a test manifest path", arg)
			}
			index++
			casesPath = args[index]
		case "--adapter":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires an executable path", arg)
			}
			index++
			adapterExecutable = args[index]
		case "--adapter-arg":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires an argument", arg)
			}
			index++
			adapterArgs = append(adapterArgs, args[index])
		case "--output", "-o":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires an output path", arg)
			}
			index++
			outputPath = args[index]
		case "--format", "-f":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires text or json", arg)
			}
			index++
			format = args[index]
		default:
			return 0, fmt.Errorf("unknown option %q", arg)
		}
	}
	if beforePath == "" || afterPath == "" || casesPath == "" || outputPath == "" {
		return 0, fmt.Errorf("policy review requires --before <policy.yaml>, --after <policy.yaml>, --cases <manifest.yaml>, and --output <review.json>")
	}
	if format != "text" && format != "json" {
		return 0, fmt.Errorf("unsupported format %q; use text or json", format)
	}
	before, err := paddockpolicy.Load(beforePath)
	if err != nil {
		return policyComparisonLoadError(format, "review", beforePath, beforePath, afterPath, err)
	}
	after, err := paddockpolicy.Load(afterPath)
	if err != nil {
		return policyComparisonLoadError(format, "review", afterPath, beforePath, afterPath, err)
	}
	beforeRef, err := paddockartifact.File(beforePath)
	if err != nil {
		return 0, err
	}
	afterRef, err := paddockartifact.File(afterPath)
	if err != nil {
		return 0, err
	}
	beforeCanonicalHash, err := paddockpolicy.CanonicalSHA256(before)
	if err != nil {
		return 0, err
	}
	afterCanonicalHash, err := paddockpolicy.CanonicalSHA256(after)
	if err != nil {
		return 0, err
	}
	diff := paddockpolicydiff.Compare(before, after)
	diff.Before = paddockpolicydiff.Input{Path: beforeRef.Path, SHA256: beforeRef.SHA256, CanonicalSHA256: beforeCanonicalHash}
	diff.After = paddockpolicydiff.Input{Path: afterRef.Path, SHA256: afterRef.SHA256, CanonicalSHA256: afterCanonicalHash}
	tests, err := paddockpolicytest.RunWithOptions(casesPath, afterPath, after, paddockpolicytest.Options{
		AdapterExecutable: adapterExecutable,
		AdapterArgs:       adapterArgs,
	})
	if err != nil {
		return 0, err
	}
	diff.Tests = &tests
	document := paddockpolicyreview.New(diff)
	if err := paddockpolicyreview.Save(outputPath, document); err != nil {
		return 0, err
	}
	switch format {
	case "text":
		err = paddockpolicyreview.Text(os.Stdout, document)
	case "json":
		err = paddockpolicyreview.JSON(os.Stdout, document)
	}
	if err != nil {
		return 0, err
	}
	if format == "text" {
		if _, err := fmt.Fprintf(os.Stdout, "  output: %s\n", outputPath); err != nil {
			return 0, err
		}
	}
	if document.Status == "FAIL" {
		return 1, nil
	}
	return 0, nil
}

func policyComparisonLoadError(format, operation, policyPath, beforePath, afterPath string, err error) (int, error) {
	if format != "json" {
		return 0, err
	}
	document := paddockpolicy.ValidationDocumentForComparison(operation, policyPath, beforePath, afterPath, err)
	if encodeErr := json.NewEncoder(os.Stdout).Encode(document); encodeErr != nil {
		return 0, encodeErr
	}
	return 2, nil
}

func verifyPolicyReview(args []string) (int, error) {
	inputPath := ""
	format := "text"
	verifyFiles := false
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--input", "-i":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a review path", arg)
			}
			index++
			inputPath = args[index]
		case "--format", "-f":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires text or json", arg)
			}
			index++
			format = args[index]
		case "--files":
			verifyFiles = true
		default:
			return 0, fmt.Errorf("unknown option %q", arg)
		}
	}
	if inputPath == "" {
		return 0, fmt.Errorf("policy review verify requires --input <review.json>")
	}
	document, err := paddockpolicyreview.Load(inputPath)
	if err != nil {
		return 0, err
	}
	if verifyFiles {
		if err := paddockpolicyreview.VerifyFiles(document); err != nil {
			return 0, err
		}
	}
	switch format {
	case "text":
		_, err = fmt.Fprintf(os.Stdout, "VERIFIED %s (%s)\n", inputPath, document.Status)
	case "json":
		err = paddockpolicyreview.JSON(os.Stdout, document)
	default:
		return 0, fmt.Errorf("unsupported format %q; use text or json", format)
	}
	if err != nil {
		return 0, err
	}
	return 0, nil
}

func sealPolicy(args []string) error {
	inputPath := ""
	outputPath := ""
	force := false
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--input", "-i":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires a policy path", arg)
			}
			index++
			inputPath = args[index]
		case "--output", "-o":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires a lock path", arg)
			}
			index++
			outputPath = args[index]
		case "--force":
			force = true
		default:
			return fmt.Errorf("unknown option %q", arg)
		}
	}
	if inputPath == "" || outputPath == "" {
		return fmt.Errorf("policy seal requires --input <policy.yaml> and --output <policy.lock.json>")
	}
	if !force {
		if _, err := os.Stat(outputPath); err == nil {
			return fmt.Errorf("%s already exists; use --force to replace it", outputPath)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("check output %s: %w", outputPath, err)
		}
	}
	sealed, err := paddockpolicylock.Build(inputPath)
	if err != nil {
		return err
	}
	if err := paddockpolicylock.Save(outputPath, sealed); err != nil {
		return err
	}
	_, err = fmt.Fprintf(os.Stdout, "SEALED %s\n", outputPath)
	return err
}

func verifyPolicy(args []string) error {
	policyPath := ""
	lockPath := ""
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--policy", "-p":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires a policy path", arg)
			}
			index++
			policyPath = args[index]
		case "--lock", "-l":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires a lock path", arg)
			}
			index++
			lockPath = args[index]
		default:
			return fmt.Errorf("unknown option %q", arg)
		}
	}
	if policyPath == "" || lockPath == "" {
		return fmt.Errorf("policy verify requires --policy <policy.yaml> and --lock <policy.lock.json>")
	}
	if err := paddockpolicylock.Verify(policyPath, lockPath); err != nil {
		return err
	}
	_, err := fmt.Fprintf(os.Stdout, "VERIFIED %s\n", lockPath)
	return err
}

type evaluationPolicy struct {
	Config          paddockpolicy.Policy
	Path            string
	CanonicalSHA256 string
	Lock            *paddockpolicylock.Artifact
}

func loadGraphInput(path string, config paddockpolicy.Policy) (*model.Graph, error) {
	loaded, document, err := paddockgraph.LoadDocument(path)
	if err != nil {
		return nil, err
	}
	if document.Language != config.Source.Language {
		return nil, fmt.Errorf("graph input language %q does not match policy language %q", document.Language, config.Source.Language)
	}
	if document.Unit != "" && document.Unit != config.Source.Unit {
		return nil, fmt.Errorf("graph input source unit %q does not match policy source unit %q", document.Unit, config.Source.Unit)
	}
	return paddockgraph.FilterGraph(loaded, config.Source.Include, config.Source.Exclude)
}

func loadExternalGraph(root string, config paddockpolicy.Policy, executable string, args []string) (*model.Graph, error) {
	loaded, _, err := loadExternalGraphDocument(root, config, executable, args)
	return loaded, err
}

func loadExternalGraphDocument(root string, config paddockpolicy.Policy, executable string, args []string) (*model.Graph, paddockgraph.Document, error) {
	return loadExternalGraphRequest(root, config.Source.Language, config.Source.Unit, config.Source.Roots, config.Source.Include, config.Source.Exclude, executable, args)
}

func loadExternalGraphRequest(root, language, unit string, roots, include, exclude []string, executable string, args []string) (*model.Graph, paddockgraph.Document, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, paddockgraph.Document{}, fmt.Errorf("resolve source root: %w", err)
	}
	requiredCapabilities := paddockgraph.Capabilities{}
	if unit != "" {
		requiredCapabilities.SourceUnits = []string{unit}
	}
	return paddockgraph.LoadExternal(context.Background(), executable, args, paddockgraph.Request{
		Schema:               paddockgraph.RequestSchema,
		Language:             language,
		Unit:                 unit,
		Root:                 absRoot,
		Roots:                append([]string(nil), roots...),
		Include:              append([]string(nil), include...),
		Exclude:              append([]string(nil), exclude...),
		RequiredCapabilities: requiredCapabilities,
	})
}

func saveGraphDocument(path string, document paddockgraph.Document) error {
	if err := document.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("encode graph document: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write graph document: %w", err)
	}
	return nil
}

func loadEvaluationPolicy(policyPath, policyLockPath string) (evaluationPolicy, error) {
	if policyLockPath != "" {
		locked, err := paddockpolicylock.Load(policyLockPath)
		if err != nil {
			return evaluationPolicy{}, err
		}
		if policyPath != "" {
			if err := paddockpolicylock.Verify(policyPath, policyLockPath); err != nil {
				return evaluationPolicy{}, err
			}
			config, err := paddockpolicy.Load(policyPath)
			if err != nil {
				return evaluationPolicy{}, err
			}
			return evaluationPolicy{
				Config:          config,
				Path:            policyPath,
				CanonicalSHA256: locked.CanonicalSHA256,
				Lock:            &locked,
			}, nil
		}
		config, err := paddockpolicy.ParseJSON(locked.Policy)
		if err != nil {
			return evaluationPolicy{}, err
		}
		return evaluationPolicy{
			Config:          config,
			Path:            locked.PolicyPath,
			CanonicalSHA256: locked.CanonicalSHA256,
			Lock:            &locked,
		}, nil
	}
	config, err := paddockpolicy.Load(policyPath)
	if err != nil {
		return evaluationPolicy{}, err
	}
	canonicalHash, err := paddockpolicy.CanonicalSHA256(config)
	if err != nil {
		return evaluationPolicy{}, err
	}
	return evaluationPolicy{Config: config, Path: policyPath, CanonicalSHA256: canonicalHash}, nil
}

func applyBaseline(result *model.Result, baselinePath, policyPath string) error {
	snapshot, err := paddockbaseline.Load(baselinePath)
	if err != nil {
		return err
	}
	policyHash, err := canonicalPolicyHash(policyPath)
	if err != nil {
		return err
	}
	return applyBaselineSnapshot(result, snapshot, baselinePath, policyHash)
}

func applyBaselineHash(result *model.Result, baselinePath, policyHash string) error {
	snapshot, err := paddockbaseline.Load(baselinePath)
	if err != nil {
		return err
	}
	return applyBaselineSnapshot(result, snapshot, baselinePath, policyHash)
}

func applyBaselineSnapshot(result *model.Result, snapshot paddockbaseline.Snapshot, baselinePath, policyHash string) error {
	return paddockbaseline.Apply(result, snapshot, baselinePath, policyHash)
}

func canonicalPolicyHash(path string) (string, error) {
	config, err := paddockpolicy.Load(path)
	if err != nil {
		return "", err
	}
	return paddockpolicy.CanonicalSHA256(config)
}

func createBaseline(args []string) error {
	root := "."
	policyPath := ""
	graphInputPath := ""
	adapterExecutable := ""
	adapterArgs := []string(nil)
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
		case "--graph":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires a graph document path", arg)
			}
			index++
			graphInputPath = args[index]
		case "--adapter":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires an executable path", arg)
			}
			index++
			adapterExecutable = args[index]
		case "--adapter-arg":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires an argument", arg)
			}
			index++
			adapterArgs = append(adapterArgs, args[index])
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
	if graphInputPath != "" && adapterExecutable != "" {
		return fmt.Errorf("baseline accepts either --graph or --adapter, not both")
	}
	if outputPath == "" {
		return fmt.Errorf("baseline requires --output <path>")
	}
	config, err := paddockpolicy.Load(policyPath)
	if err != nil {
		return err
	}
	var result *model.Result
	if graphInputPath != "" {
		loaded, err := loadGraphInput(graphInputPath, config)
		if err != nil {
			return err
		}
		result, err = checker.CheckGraph(root, policyPath, config, loaded)
	} else if adapterExecutable != "" {
		loaded, err := loadExternalGraph(root, config, adapterExecutable, adapterArgs)
		if err != nil {
			return err
		}
		result, err = checker.CheckGraph(root, policyPath, config, loaded)
	} else {
		result, err = checker.Check(root, policyPath)
	}
	if err != nil {
		return err
	}
	policyHash, err := paddockpolicy.CanonicalSHA256(config)
	if err != nil {
		return err
	}
	snapshot := paddockbaseline.Build(result, policyHash)
	if err := paddockbaseline.Save(outputPath, snapshot); err != nil {
		return err
	}
	_, err = fmt.Fprintf(os.Stdout, "BASELINE %s (%d findings)\n", outputPath, len(snapshot.Entries))
	return err
}

func createCIArtifact(args []string) (int, error) {
	root := "."
	policyPath := ""
	policyLockPath := ""
	graphInputPath := ""
	graphOutputPath := ""
	adapterExecutable := ""
	adapterArgs := []string(nil)
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
		case "--policy-lock":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a path", arg)
			}
			index++
			policyLockPath = args[index]
		case "--graph":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a graph document path", arg)
			}
			index++
			graphInputPath = args[index]
		case "--graph-output":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a graph document path", arg)
			}
			index++
			graphOutputPath = args[index]
		case "--adapter":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires an executable path", arg)
			}
			index++
			adapterExecutable = args[index]
		case "--adapter-arg":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires an argument", arg)
			}
			index++
			adapterArgs = append(adapterArgs, args[index])
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
	if policyPath == "" && policyLockPath == "" {
		return 0, fmt.Errorf("ci requires --policy <path> or --policy-lock <path>")
	}
	if graphInputPath != "" && adapterExecutable != "" {
		return 0, fmt.Errorf("ci accepts either --graph or --adapter, not both")
	}
	if adapterExecutable != "" && graphOutputPath == "" {
		return 0, fmt.Errorf("ci requires --graph-output <path> when --adapter is used")
	}
	if adapterExecutable == "" && graphOutputPath != "" {
		return 0, fmt.Errorf("ci --graph-output requires --adapter")
	}
	if outputPath == "" {
		return 0, fmt.Errorf("ci requires --output <path>")
	}

	createdAt := time.Now().UTC()
	policyRef := paddockartifact.FileRef{Path: policyPath}
	var policyLockRef *paddockartifact.FileRef
	if policyPath == "" {
		policyRef.Path = policyLockPath
	} else {
		if ref, err := paddockartifact.File(policyPath); err == nil {
			policyRef = ref
		} else {
			return saveCIErrorWithPolicyLock(outputPath, root, policyRef, nil, nil, err, createdAt)
		}
	}
	if policyLockPath != "" {
		ref, err := paddockartifact.File(policyLockPath)
		if err != nil {
			return saveCIErrorWithPolicyLock(outputPath, root, policyRef, nil, nil, err, createdAt)
		}
		policyLockRef = &ref
	}
	var graphRef *paddockartifact.FileRef
	if graphInputPath != "" {
		ref, err := paddockartifact.File(graphInputPath)
		if err != nil {
			return saveCIErrorWithInputs(outputPath, root, policyRef, policyLockRef, nil, nil, err, createdAt)
		}
		graphRef = &ref
	}
	evaluation, err := loadEvaluationPolicy(policyPath, policyLockPath)
	if err != nil {
		return saveCIErrorWithInputs(outputPath, root, policyRef, policyLockRef, graphRef, nil, err, createdAt)
	}
	if policyPath == "" {
		policyRef = paddockartifact.FileRef{
			Path:   evaluation.Path,
			SHA256: evaluation.Lock.SourceSHA256,
		}
	}
	var externalGraph *model.Graph
	if adapterExecutable != "" {
		loaded, document, err := loadExternalGraphDocument(root, evaluation.Config, adapterExecutable, adapterArgs)
		if err != nil {
			return saveCIErrorWithInputs(outputPath, root, policyRef, policyLockRef, nil, nil, err, createdAt)
		}
		if err := saveGraphDocument(graphOutputPath, document); err != nil {
			return saveCIErrorWithInputs(outputPath, root, policyRef, policyLockRef, nil, nil, err, createdAt)
		}
		ref, err := paddockartifact.File(graphOutputPath)
		if err != nil {
			return saveCIErrorWithInputs(outputPath, root, policyRef, policyLockRef, nil, nil, err, createdAt)
		}
		graphRef = &ref
		externalGraph = loaded
	}
	var baselineRef *paddockartifact.FileRef
	if baselinePath != "" {
		ref, err := paddockartifact.File(baselinePath)
		if err != nil {
			return saveCIErrorWithInputs(outputPath, root, policyRef, policyLockRef, graphRef, &ref, err, createdAt)
		}
		baselineRef = &ref
	}

	var result *model.Result
	if graphInputPath != "" {
		loaded, err := loadGraphInput(graphInputPath, evaluation.Config)
		if err != nil {
			return saveCIErrorWithInputs(outputPath, root, policyRef, policyLockRef, graphRef, baselineRef, err, createdAt)
		}
		result, err = checker.CheckGraph(root, evaluation.Path, evaluation.Config, loaded)
	} else if externalGraph != nil {
		result, err = checker.CheckGraph(root, evaluation.Path, evaluation.Config, externalGraph)
	} else {
		result, err = checker.CheckPolicy(root, evaluation.Path, evaluation.Config)
	}
	if err != nil {
		return saveCIErrorWithInputs(outputPath, root, policyRef, policyLockRef, graphRef, baselineRef, err, createdAt)
	}
	if baselinePath != "" {
		if err := applyBaselineHash(result, baselinePath, evaluation.CanonicalSHA256); err != nil {
			return saveCIErrorWithInputs(outputPath, root, policyRef, policyLockRef, graphRef, baselineRef, err, createdAt)
		}
	}
	ciArtifact := paddockartifact.NewWithInputs(result, policyRef, policyLockRef, graphRef, baselineRef, createdAt)
	if err := paddockartifact.Save(outputPath, ciArtifact); err != nil {
		return 0, err
	}
	if _, err := fmt.Fprintf(os.Stdout, "CI-RESULT %s (%s)\n", outputPath, ciArtifact.Status); err != nil {
		return 0, err
	}
	return ciArtifact.ExitCode, nil
}

func validateCIArtifact(args []string) error {
	inputPath := ""
	format := "text"
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--input", "-i":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires a CI result path", args[index])
			}
			index++
			inputPath = args[index]
		case "--format", "-f":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires text or json", args[index])
			}
			index++
			format = args[index]
		default:
			return fmt.Errorf("unknown option %q", args[index])
		}
	}
	if inputPath == "" {
		return fmt.Errorf("ci validate requires --input <ci-result.json>")
	}
	artifact, err := ciresult.LoadFile(inputPath)
	if err != nil {
		return err
	}

	switch format {
	case "text":
		fmt.Fprintln(os.Stdout, "CI-RESULT VALID")
		fmt.Fprintf(os.Stdout, "artifact: %s\n", inputPath)
		fmt.Fprintf(os.Stdout, "tool: %s\n", artifact.Tool)
		fmt.Fprintf(os.Stdout, "kind: %s\n", artifact.Kind)
		fmt.Fprintf(os.Stdout, "status: %s\n", artifact.Status)
		fmt.Fprintf(os.Stdout, "exit_code: %d\n", artifact.ExitCode)
		return nil
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(artifact)
	default:
		return fmt.Errorf("unsupported format %q; use text or json", format)
	}
}

func validateAdapter(args []string) (int, error) {
	root := "."
	language := ""
	unit := ""
	adapterExecutable := ""
	adapterArgs := []string(nil)
	format := "text"
	rootSet := false
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--language", "-l":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a language", args[index])
			}
			index++
			language = args[index]
		case "--unit", "--source-unit":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a source unit", args[index])
			}
			index++
			unit = args[index]
		case "--adapter":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires an executable path", args[index])
			}
			index++
			adapterExecutable = args[index]
		case "--adapter-arg":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires an argument", args[index])
			}
			index++
			adapterArgs = append(adapterArgs, args[index])
		case "--format", "-f":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires text or json", args[index])
			}
			index++
			format = args[index]
		default:
			if strings.HasPrefix(args[index], "-") {
				return 0, fmt.Errorf("unknown option %q", args[index])
			}
			if rootSet {
				return 0, fmt.Errorf("unexpected argument %q", args[index])
			}
			root = args[index]
			rootSet = true
		}
	}
	if language == "" {
		return 0, fmt.Errorf("adapter validate requires --language <language>")
	}
	if adapterExecutable == "" {
		return 0, fmt.Errorf("adapter validate requires --adapter <program>")
	}
	if format != "text" && format != "json" {
		return 0, fmt.Errorf("unsupported format %q; use text or json", format)
	}
	_, document, err := loadExternalGraphRequest(root, language, unit, nil, nil, nil, adapterExecutable, adapterArgs)
	if err != nil {
		if format != "json" {
			return 0, err
		}
		diagnostics := paddockgraph.ValidationDocumentForError("adapter-validate", root, language, unit, adapterExecutable, err)
		if encodeErr := json.NewEncoder(os.Stdout).Encode(diagnostics); encodeErr != nil {
			return 0, encodeErr
		}
		return 2, nil
	}

	switch format {
	case "text":
		fmt.Fprintln(os.Stdout, "ADAPTER VALID")
		fmt.Fprintf(os.Stdout, "language: %s\n", document.Language)
		fmt.Fprintf(os.Stdout, "source_unit: %s\n", document.Unit)
		fmt.Fprintf(os.Stdout, "root: %s\n", document.Root)
		fmt.Fprintf(os.Stdout, "packages: %d\n", document.PackageCount)
		fmt.Fprintf(os.Stdout, "edges: %d\n", document.EdgeCount)
		return 0, nil
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return 0, encoder.Encode(document)
	}
	return 0, nil
}

func adapterTestCommand(args []string) (int, error) {
	if len(args) > 0 && args[0] == "validate" {
		return 0, validateAdapterTests(args[1:])
	}
	if len(args) > 0 && args[0] == "verify" {
		return verifyAdapterTestResult(args[1:])
	}
	manifestPath := ""
	outputPath := ""
	ciResultPath := ""
	format := "text"
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--cases", "-c":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires an adapter test manifest path", args[index])
			}
			index++
			manifestPath = args[index]
		case "--format", "-f":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires text or json", args[index])
			}
			index++
			format = args[index]
		case "--output", "-o":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires an output path", args[index])
			}
			index++
			outputPath = args[index]
		case "--ci-result":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires an output path", args[index])
			}
			index++
			ciResultPath = args[index]
		default:
			return 0, fmt.Errorf("unknown option %q", args[index])
		}
	}
	if manifestPath == "" {
		return 0, fmt.Errorf("adapter test requires --cases <manifest.yaml>")
	}
	if format != "text" && format != "json" {
		return 0, fmt.Errorf("unsupported format %q; use text or json", format)
	}
	document, err := paddockadaptertest.Run(manifestPath)
	if err != nil {
		return 0, err
	}
	if err := document.Validate(); err != nil {
		return 0, err
	}
	if outputPath != "" {
		if err := paddockadaptertest.Save(outputPath, document); err != nil {
			return 0, err
		}
	}
	if ciResultPath != "" {
		if err := saveAdapterTestCIResult(ciResultPath, document); err != nil {
			return 0, err
		}
	}
	switch format {
	case "text":
		if err := paddockadaptertest.Text(os.Stdout, document); err != nil {
			return 0, err
		}
	case "json":
		if err := paddockadaptertest.JSON(os.Stdout, document); err != nil {
			return 0, err
		}
	}
	if outputPath != "" && format == "text" {
		if _, err := fmt.Fprintf(os.Stdout, "  output: %s\n", outputPath); err != nil {
			return 0, err
		}
	}
	if ciResultPath != "" && format == "text" {
		if _, err := fmt.Fprintf(os.Stdout, "  ci_result: %s\n", ciResultPath); err != nil {
			return 0, err
		}
	}
	if document.Status == "FAIL" {
		return 1, nil
	}
	return 0, nil
}

func saveAdapterTestCIResult(path string, document paddockadaptertest.Document) error {
	report, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode adapter test report for CI result: %w", err)
	}
	explanation, err := json.Marshal(document.Explain())
	if err != nil {
		return fmt.Errorf("encode adapter test explanation for CI result: %w", err)
	}
	status := "passed"
	exitCode := 0
	if document.Status == "FAIL" {
		status = "failed"
		exitCode = 1
	}
	shared := ciresult.Artifact{
		Schema:    ciresult.Schema,
		Tool:      "paddock",
		Kind:      "adapter-conformance",
		Status:    status,
		ExitCode:  exitCode,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Source: ciresult.Source{
			Root: filepath.Dir(document.Manifest.Path),
		},
		Inputs: map[string]ciresult.FileRef{
			"manifest": {
				Path:   document.Manifest.Path,
				SHA256: document.Manifest.SHA256,
			},
		},
		Report:      report,
		Explanation: explanation,
	}
	if err := ciresult.SaveFile(path, shared); err != nil {
		return err
	}
	return nil
}

func validateAdapterTests(args []string) error {
	manifestPath := ""
	format := "text"
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--cases", "-c":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires an adapter test manifest path", args[index])
			}
			index++
			manifestPath = args[index]
		case "--format", "-f":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires text or json", args[index])
			}
			index++
			format = args[index]
		default:
			return fmt.Errorf("unknown option %q", args[index])
		}
	}
	if manifestPath == "" {
		return fmt.Errorf("adapter test validate requires --cases <manifest.yaml>")
	}
	manifest, err := paddockadaptertest.Load(manifestPath)
	if err != nil {
		return err
	}
	switch format {
	case "text":
		fmt.Fprintln(os.Stdout, "ADAPTER-TEST-MANIFEST VALID")
		fmt.Fprintf(os.Stdout, "manifest: %s\n", manifestPath)
		fmt.Fprintf(os.Stdout, "adapter: %s\n", manifest.Adapter.Executable)
		fmt.Fprintf(os.Stdout, "cases: %d\n", len(manifest.Cases))
		for _, testCase := range manifest.Cases {
			unit := testCase.SourceUnit
			if unit == "" {
				unit = "any"
			}
			fmt.Fprintf(os.Stdout, "  %s (%s/%s, expected %s)\n", testCase.Name, testCase.Language, unit, testCase.Expect)
		}
		return nil
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(manifest)
	default:
		return fmt.Errorf("unsupported format %q; use text or json", format)
	}
}

func verifyAdapterTestResult(args []string) (int, error) {
	inputPath := ""
	format := "text"
	verifyFiles := false
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--input", "-i":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires an adapter test result path", args[index])
			}
			index++
			inputPath = args[index]
		case "--format", "-f":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires text or json", args[index])
			}
			index++
			format = args[index]
		case "--files":
			verifyFiles = true
		default:
			return 0, fmt.Errorf("unknown option %q", args[index])
		}
	}
	if inputPath == "" {
		return 0, fmt.Errorf("adapter test verify requires --input <adapter-test-result.json>")
	}
	document, err := paddockadaptertest.LoadResult(inputPath)
	if err != nil {
		return 0, err
	}
	if verifyFiles {
		if err := paddockadaptertest.VerifyFiles(document); err != nil {
			return 0, err
		}
	}
	switch format {
	case "text":
		_, err = fmt.Fprintf(os.Stdout, "VERIFIED %s (%s)\n", inputPath, document.Status)
	case "json":
		err = paddockadaptertest.JSON(os.Stdout, document)
	default:
		return 0, fmt.Errorf("unsupported format %q; use text or json", format)
	}
	if err != nil {
		return 0, err
	}
	return 0, nil
}

func saveCIError(outputPath, root string, policy paddockartifact.FileRef, baseline *paddockartifact.FileRef, cause error, createdAt time.Time) (int, error) {
	return saveCIErrorWithPolicyLock(outputPath, root, policy, nil, baseline, cause, createdAt)
}

func saveCIErrorWithPolicyLock(outputPath, root string, policy paddockartifact.FileRef, policyLock *paddockartifact.FileRef, baseline *paddockartifact.FileRef, cause error, createdAt time.Time) (int, error) {
	return saveCIErrorWithInputs(outputPath, root, policy, policyLock, nil, baseline, cause, createdAt)
}

func saveCIErrorWithInputs(outputPath, root string, policy paddockartifact.FileRef, policyLock, graph, baseline *paddockartifact.FileRef, cause error, createdAt time.Time) (int, error) {
	ciArtifact := paddockartifact.NewErrorWithInputs(root, policy, policyLock, graph, baseline, cause, createdAt)
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
	ruleID := ""
	status := ""
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--format", "-f":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires text or json", arg)
			}
			index++
			format = args[index]
		case "--rule":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires a rule ID", arg)
			}
			index++
			ruleID = args[index]
		case "--status":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires a finding status", arg)
			}
			index++
			status = args[index]
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
	var envelope struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("parse explanation input: %w", err)
	}

	var document paddockexplain.Document
	switch envelope.Schema {
	case paddockartifact.Schema:
		ciArtifact, err := paddockartifact.Load(reportPath)
		if err != nil {
			return err
		}
		if ciArtifact.Explanation == nil {
			if ciArtifact.Error != "" {
				return fmt.Errorf("CI artifact has no explanation: %s", ciArtifact.Error)
			}
			return fmt.Errorf("CI artifact has no explanation")
		}
		document = *ciArtifact.Explanation
		provenance, err := explanationProvenance(reportPath, ciArtifact)
		if err != nil {
			return err
		}
		document.Provenance = provenance
	case "paddock.report/v1":
		var result model.Result
		if err := json.Unmarshal(data, &result); err != nil {
			return fmt.Errorf("parse report: %w", err)
		}
		document = paddockexplain.Explain(&result)
	default:
		return fmt.Errorf("explanation input must be paddock.report/v1 or %s, got %q", paddockartifact.Schema, envelope.Schema)
	}
	document, err = paddockexplain.ApplyFilter(document, ruleID, status)
	if err != nil {
		return err
	}
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

func explanationProvenance(path string, artifact paddockartifact.Artifact) (*paddockexplain.Provenance, error) {
	artifactRef, err := paddockartifact.File(path)
	if err != nil {
		return nil, err
	}
	provenance := &paddockexplain.Provenance{
		ArtifactSchema:   artifact.Schema,
		ArtifactPath:     path,
		ArtifactSHA256:   artifactRef.SHA256,
		ArtifactStatus:   artifact.Status,
		ArtifactExitCode: artifact.ExitCode,
		CreatedAt:        artifact.CreatedAt,
		Policy: paddockexplain.FileReference{
			Path:   artifact.Policy.Path,
			SHA256: artifact.Policy.SHA256,
		},
	}
	if artifact.PolicyLock != nil {
		provenance.PolicyLock = &paddockexplain.FileReference{
			Path:   artifact.PolicyLock.Path,
			SHA256: artifact.PolicyLock.SHA256,
		}
	}
	if artifact.Graph != nil {
		provenance.Graph = &paddockexplain.FileReference{
			Path:   artifact.Graph.Path,
			SHA256: artifact.Graph.SHA256,
		}
		if _, graphDocument, err := paddockgraph.LoadDocument(artifact.Graph.Path); err == nil && graphDocument.Adapter != nil {
			provenance.Adapter = &paddockexplain.AdapterReference{
				Kind:               graphDocument.Adapter.Kind,
				Name:               graphDocument.Adapter.Name,
				Version:            graphDocument.Adapter.Version,
				Executable:         graphDocument.Adapter.Executable,
				ResolvedExecutable: graphDocument.Adapter.ResolvedExecutable,
				ExecutableSHA256:   graphDocument.Adapter.ExecutableSHA256,
				ArgsSHA256:         graphDocument.Adapter.ArgsSHA256,
			}
		}
	}
	if artifact.Baseline != nil {
		provenance.Baseline = &paddockexplain.FileReference{
			Path:   artifact.Baseline.Path,
			SHA256: artifact.Baseline.SHA256,
		}
	}
	return provenance, nil
}

func versionCommand(args []string) error {
	format := "text"
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--format", "-f":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires text or json", args[index])
			}
			index++
			format = args[index]
		default:
			return fmt.Errorf("unknown option %q", args[index])
		}
	}

	info := struct {
		Name      string `json:"name"`
		Version   string `json:"version"`
		Commit    string `json:"commit"`
		BuildDate string `json:"build_date"`
	}{
		Name:      "paddock",
		Version:   paddockversion.Version,
		Commit:    paddockversion.Commit,
		BuildDate: paddockversion.BuildDate,
	}

	switch format {
	case "text":
		_, err := fmt.Fprintf(os.Stdout, "%s %s\ncommit %s\nbuilt %s\n", info.Name, info.Version, info.Commit, info.BuildDate)
		return err
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(info)
	default:
		return fmt.Errorf("unsupported format %q; use text or json", format)
	}
}

func releaseCommand(args []string) (int, error) {
	if len(args) == 0 || args[0] != "verify" {
		return 0, fmt.Errorf("release requires verify")
	}
	manifestPath := ""
	directory := ""
	format := "text"
	for index := 1; index < len(args); index++ {
		switch args[index] {
		case "--manifest", "-m":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a path", args[index])
			}
			index++
			manifestPath = args[index]
		case "--directory", "-d":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a path", args[index])
			}
			index++
			directory = args[index]
		case "--format", "-f":
			if index+1 >= len(args) {
				return 0, fmt.Errorf("%s requires text or json", args[index])
			}
			index++
			format = args[index]
		default:
			return 0, fmt.Errorf("unknown option %q", args[index])
		}
	}
	if manifestPath == "" {
		return 0, fmt.Errorf("release verify requires --manifest <release-manifest.json>")
	}
	result, err := paddockrelease.Verify(manifestPath, directory)
	if err != nil {
		return 0, err
	}
	switch format {
	case "text":
		if _, err := fmt.Fprint(os.Stdout, paddockrelease.Text(result)); err != nil {
			return 0, err
		}
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			return 0, err
		}
	default:
		return 0, fmt.Errorf("unsupported format %q; use text or json", format)
	}
	if result.Status != "PASS" {
		return 1, nil
	}
	return 0, nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: paddock check <source-root> [--policy <policy.yaml> | --policy-lock <lock.json>] [--graph <graph.json> | --adapter <program> [--adapter-arg <arg>...]] [--baseline <file>] [--format text|json]")
	fmt.Fprintln(os.Stderr, "       paddock graph <source-root> [--policy <policy.yaml> | --language <language> | --input <graph.json> | --adapter <program> [--adapter-arg <arg>...]] [--format text|json]")
	fmt.Fprintln(os.Stderr, "       paddock map <source-root> [--policy <policy.yaml> | --policy-lock <lock.json>] [--graph <graph.json> | --adapter <program> [--adapter-arg <arg>...]] [--format text|json]")
	fmt.Fprintln(os.Stderr, "       paddock init <source-root> [--language <language>] [--unit package|file] [--graph <graph.json> | --adapter <program> [--adapter-arg <arg>...]] [--template <name>] [--output paddock.yaml] [--format text|json] [--force]")
	fmt.Fprintln(os.Stderr, "       paddock policy validate --policy <policy.yaml> [--format text|json]")
	fmt.Fprintln(os.Stderr, "       paddock policy diff --before <policy.yaml> --after <policy.yaml> [--cases <manifest.yaml>] [--adapter <program> [--adapter-arg <arg>...]] [--format text|json]")
	fmt.Fprintln(os.Stderr, "       paddock policy review --before <policy.yaml> --after <policy.yaml> --cases <manifest.yaml> [--adapter <program> [--adapter-arg <arg>...]] --output <review.json> [--format text|json]")
	fmt.Fprintln(os.Stderr, "       paddock policy review verify --input <review.json> [--files] [--format text|json]")
	fmt.Fprintln(os.Stderr, "       paddock policy seal --input <policy.yaml> --output <policy.lock.json> [--force]")
	fmt.Fprintln(os.Stderr, "       paddock policy verify --policy <policy.yaml> --lock <policy.lock.json>")
	fmt.Fprintln(os.Stderr, "       paddock policy test validate --cases <manifest.yaml> [--format text|json]")
	fmt.Fprintln(os.Stderr, "       paddock policy test --policy <policy.yaml> --cases <manifest.yaml> [--adapter <program> [--adapter-arg <arg>...]] [--format text|json]")
	fmt.Fprintln(os.Stderr, "       paddock release verify --manifest <release-manifest.json> [--directory <dir>] [--format text|json]")
	fmt.Fprintln(os.Stderr, "       paddock baseline <source-root> --policy <policy.yaml> [--graph <graph.json> | --adapter <program> [--adapter-arg <arg>...]] --output <baseline.json>")
	fmt.Fprintln(os.Stderr, "       paddock ci <source-root> [--policy <policy.yaml> | --policy-lock <lock.json>] [--graph <graph.json> | --adapter <program> [--adapter-arg <arg>...] --graph-output <graph.json>] [--baseline <file>] --output <ci-result.json>")
	fmt.Fprintln(os.Stderr, "       paddock ci validate --input <ci-result.json> [--format text|json]")
	fmt.Fprintln(os.Stderr, "       paddock adapter validate <source-root> --language <language> [--unit package|file] --adapter <program> [--adapter-arg <arg>...] [--format text|json]")
	fmt.Fprintln(os.Stderr, "       paddock adapter test --cases <manifest.yaml> [--output <result.json>] [--ci-result <ci-result.json>] [--format text|json]")
	fmt.Fprintln(os.Stderr, "       paddock adapter test validate --cases <manifest.yaml> [--format text|json]")
	fmt.Fprintln(os.Stderr, "       paddock adapter test verify --input <adapter-test-result.json> [--files] [--format text|json]")
	fmt.Fprintln(os.Stderr, "       paddock explain <paddock-report.json|paddock-ci-result.json> [--format text|json] [--rule <id>] [--status all|active|blocking|advisory|waived|baselined|expired-waiver]")
	fmt.Fprintln(os.Stderr, "       paddock version [--format text|json]")
}
