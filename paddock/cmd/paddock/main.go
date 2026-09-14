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
	paddockpolicydiff "ingen/paddock/internal/policydiff"
	paddockpolicylock "ingen/paddock/internal/policylock"
	"ingen/paddock/internal/report"
	paddockscaffold "ingen/paddock/internal/scaffold"
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
	case "init":
		if err := initCommand(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "paddock:", err)
			os.Exit(2)
		}
	case "policy":
		if err := policyCommand(os.Args[2:]); err != nil {
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
	policyLockPath := ""
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
	evaluation, err := loadEvaluationPolicy(policyPath, policyLockPath)
	if err != nil {
		return err
	}
	result, err := checker.CheckPolicy(root, evaluation.Path, evaluation.Config)
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
		return fmt.Errorf("graph requires --language <go|typescript|python> or --policy <path>")
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

func initCommand(args []string) error {
	root := "."
	language := ""
	template := ""
	outputPath := "paddock.yaml"
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

	if language == "" {
		language = detectLanguage(root)
	}
	if language == "" {
		return fmt.Errorf("cannot detect source language; use --language go|typescript|python")
	}
	if template == "" {
		template = defaultTemplate(language)
	}
	request := paddockgraph.LoadRequest{Root: root, Unit: sourceUnit(language)}
	registry := paddockgraph.DefaultRegistry()
	loaded, err := registry.Load(language, request)
	if err != nil {
		return err
	}
	contents, err := paddockscaffold.Generate(paddockscaffold.Options{
		Language: language,
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
	_, err = fmt.Fprintf(os.Stdout, "INIT %s (draft; review before CI)\n", outputPath)
	return err
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

func policyCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("policy requires diff, seal, or verify subcommand")
	}
	switch args[0] {
	case "diff":
		return diffPolicies(args[1:])
	case "seal":
		return sealPolicy(args[1:])
	case "verify":
		return verifyPolicy(args[1:])
	default:
		return fmt.Errorf("unsupported policy subcommand %q", args[0])
	}
}

func diffPolicies(args []string) error {
	beforePath := ""
	afterPath := ""
	format := "text"
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--before", "-b":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires a policy path", arg)
			}
			index++
			beforePath = args[index]
		case "--after", "-a":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires a policy path", arg)
			}
			index++
			afterPath = args[index]
		case "--format", "-f":
			if index+1 >= len(args) {
				return fmt.Errorf("%s requires text or json", arg)
			}
			index++
			format = args[index]
		default:
			return fmt.Errorf("unknown option %q", arg)
		}
	}
	if beforePath == "" || afterPath == "" {
		return fmt.Errorf("policy diff requires --before <policy.yaml> and --after <policy.yaml>")
	}
	before, err := paddockpolicy.Load(beforePath)
	if err != nil {
		return err
	}
	after, err := paddockpolicy.Load(afterPath)
	if err != nil {
		return err
	}
	beforeRef, err := paddockartifact.File(beforePath)
	if err != nil {
		return err
	}
	afterRef, err := paddockartifact.File(afterPath)
	if err != nil {
		return err
	}
	beforeCanonicalHash, err := paddockpolicy.CanonicalSHA256(before)
	if err != nil {
		return err
	}
	afterCanonicalHash, err := paddockpolicy.CanonicalSHA256(after)
	if err != nil {
		return err
	}
	document := paddockpolicydiff.Compare(before, after)
	document.Before = paddockpolicydiff.Input{Path: beforeRef.Path, SHA256: beforeRef.SHA256, CanonicalSHA256: beforeCanonicalHash}
	document.After = paddockpolicydiff.Input{Path: afterRef.Path, SHA256: afterRef.SHA256, CanonicalSHA256: afterCanonicalHash}
	switch format {
	case "text":
		return paddockpolicydiff.Text(os.Stdout, document)
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(document)
	default:
		return fmt.Errorf("unsupported format %q; use text or json", format)
	}
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
	policyHash, err := canonicalPolicyHash(policyPath)
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
	evaluation, err := loadEvaluationPolicy(policyPath, policyLockPath)
	if err != nil {
		return saveCIErrorWithPolicyLock(outputPath, root, policyRef, policyLockRef, nil, err, createdAt)
	}
	if policyPath == "" {
		policyRef = paddockartifact.FileRef{
			Path:   evaluation.Path,
			SHA256: evaluation.Lock.SourceSHA256,
		}
	}
	var baselineRef *paddockartifact.FileRef
	if baselinePath != "" {
		ref, err := paddockartifact.File(baselinePath)
		if err != nil {
			return saveCIErrorWithPolicyLock(outputPath, root, policyRef, policyLockRef, &ref, err, createdAt)
		}
		baselineRef = &ref
	}

	result, err := checker.CheckPolicy(root, evaluation.Path, evaluation.Config)
	if err != nil {
		return saveCIErrorWithPolicyLock(outputPath, root, policyRef, policyLockRef, baselineRef, err, createdAt)
	}
	if baselinePath != "" {
		if err := applyBaselineHash(result, baselinePath, evaluation.CanonicalSHA256); err != nil {
			return saveCIErrorWithPolicyLock(outputPath, root, policyRef, policyLockRef, baselineRef, err, createdAt)
		}
	}
	ciArtifact := paddockartifact.NewWithPolicyLock(result, policyRef, policyLockRef, baselineRef, createdAt)
	if err := paddockartifact.Save(outputPath, ciArtifact); err != nil {
		return 0, err
	}
	if _, err := fmt.Fprintf(os.Stdout, "CI-RESULT %s (%s)\n", outputPath, ciArtifact.Status); err != nil {
		return 0, err
	}
	return ciArtifact.ExitCode, nil
}

func saveCIError(outputPath, root string, policy paddockartifact.FileRef, baseline *paddockartifact.FileRef, cause error, createdAt time.Time) (int, error) {
	return saveCIErrorWithPolicyLock(outputPath, root, policy, nil, baseline, cause, createdAt)
}

func saveCIErrorWithPolicyLock(outputPath, root string, policy paddockartifact.FileRef, policyLock *paddockartifact.FileRef, baseline *paddockartifact.FileRef, cause error, createdAt time.Time) (int, error) {
	ciArtifact := paddockartifact.NewErrorWithPolicyLock(root, policy, policyLock, baseline, cause, createdAt)
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
	fmt.Fprintln(os.Stderr, "usage: paddock check <source-root> [--policy <policy.yaml> | --policy-lock <lock.json>] [--baseline <file>] [--format text|json]")
	fmt.Fprintln(os.Stderr, "       paddock graph <source-root> [--policy <policy.yaml> | --language go|typescript|python] [--format text|json]")
	fmt.Fprintln(os.Stderr, "       paddock init <source-root> [--language go|typescript|python] [--template <name>] [--output paddock.yaml] [--force]")
	fmt.Fprintln(os.Stderr, "       paddock policy diff --before <policy.yaml> --after <policy.yaml> [--format text|json]")
	fmt.Fprintln(os.Stderr, "       paddock policy seal --input <policy.yaml> --output <policy.lock.json> [--force]")
	fmt.Fprintln(os.Stderr, "       paddock policy verify --policy <policy.yaml> --lock <policy.lock.json>")
	fmt.Fprintln(os.Stderr, "       paddock baseline <source-root> --policy <policy.yaml> --output <baseline.json>")
	fmt.Fprintln(os.Stderr, "       paddock ci <source-root> [--policy <policy.yaml> | --policy-lock <lock.json>] [--baseline <file>] --output <ci-result.json>")
	fmt.Fprintln(os.Stderr, "       paddock explain <paddock-report.json> [--format text|json]")
}
