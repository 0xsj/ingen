// Command sorna-go-provider prepares the first source-level Go mutation
// variant for the document-pipeline lab.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"

	"ingen/sorna/internal/campaign"
	"ingen/sorna/internal/mutation"
	"ingen/sorna/providers/golang"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	flags := flag.NewFlagSet("sorna-go-provider", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	planPath := flags.String("plan", ".artifacts/document-pipeline-mutation-plan.json", "canonical mutation campaign plan")
	sourceRoot := flags.String("source-root", ".", "clean Go source root")
	outputDir := flags.String("output-dir", ".artifacts/document-pipeline-go-provider", "retained copied mutation source variants")
	binaryDir := flags.String("binary-dir", ".artifacts/document-pipeline-subject/go-mutations", "runnable mutation binaries")
	buildPackage := flags.String("build-package", "./examples/document-pipeline-lab/subject/cmd/document-pipeline", "Go package to build in each copied variant")
	providerID := flags.String("provider-id", "document-pipeline-go-source", "generated provider manifest ID")
	providerPath := flags.String("provider", "", "generated provider manifest path; defaults inside output-dir")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if len(flags.Args()) != 0 {
		fmt.Fprintln(os.Stderr, "usage: sorna-go-provider [--plan <path>] [--source-root <dir>] [--output-dir <dir>] [--binary-dir <dir>] [--build-package <package>] [--provider <path>]")
		return 2
	}
	planBytes, err := os.ReadFile(*planPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read plan:", err)
		return 1
	}
	plan, err := campaign.LoadBytes(*planPath, planBytes)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	providerOutput := *providerPath
	if providerOutput == "" {
		providerOutput = filepath.Join(*outputDir, "provider.yaml")
	}
	result, err := goprovider.Build(goprovider.Request{
		Plan:         plan,
		PlanSHA256:   campaign.HashBytes(planBytes),
		SourceRoot:   *sourceRoot,
		OutputDir:    *outputDir,
		BinaryDir:    *binaryDir,
		BuildPackage: *buildPackage,
		BinaryName:   "document-pipeline",
		ProviderID:   *providerID,
		SubjectArgs:  []string{"-addr", "${SORA_ADDR}"},
		Mutate:       mutateDocumentPipeline,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	providerHash, err := goprovider.WriteManifest(providerOutput, result.Provider)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if _, err := campaign.LoadProviderFile(providerOutput); err != nil {
		fmt.Fprintln(os.Stderr, "verify generated provider:", err)
		return 1
	}
	fmt.Printf("provider: %s\nhash: %s\nvariants: %d\n", providerOutput, providerHash, len(result.Variants))
	for _, variant := range result.Variants {
		fmt.Printf("mutation: %s\nbinary: %s\nbinary hash: %s\n", variant.MutationID, variant.BinaryPath, variant.BinarySHA256)
	}
	return 0
}

// mutateDocumentPipeline is intentionally narrow. It proves that the Go
// provider can apply one reviewed AST change to an isolated copy; the generic
// provider package owns copying/building, while this lab owns target meaning.
func mutateDocumentPipeline(variantRoot string, spec mutation.Spec) error {
	if spec.Plane != "implementation" {
		return fmt.Errorf("Go document provider only supports implementation mutations")
	}
	if spec.Operator != "response.status.replace" || spec.Target != "POST /documents" {
		return fmt.Errorf("unsupported document mutation %q at %q", spec.Operator, spec.Target)
	}
	from, err := changeInteger(spec, "from")
	if err != nil {
		return err
	}
	to, err := changeInteger(spec, "to")
	if err != nil {
		return err
	}
	if from != 202 || to != 200 {
		return fmt.Errorf("document provider supports only status 202 -> 200, got %d -> %d", from, to)
	}

	path := filepath.Join(variantRoot, "examples", "document-pipeline-lab", "subject", "server.go")
	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read document subject source: %w", err)
	}
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, contents, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse document subject source: %w", err)
	}
	changed := 0
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "createDocument" || function.Body == nil {
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) < 2 {
				return true
			}
			callee, ok := call.Fun.(*ast.Ident)
			if !ok || callee.Name != "writeJSON" {
				return true
			}
			status, ok := call.Args[1].(*ast.SelectorExpr)
			if !ok || status.Sel.Name != "StatusAccepted" {
				return true
			}
			packageName, ok := status.X.(*ast.Ident)
			if !ok || packageName.Name != "http" {
				return true
			}
			status.Sel.Name = "StatusOK"
			changed++
			return true
		})
	}
	if changed != 1 {
		return fmt.Errorf("expected exactly one create response status to change, found %d", changed)
	}
	var formatted bytes.Buffer
	if err := format.Node(&formatted, fileSet, file); err != nil {
		return fmt.Errorf("format mutated document subject source: %w", err)
	}
	if err := os.WriteFile(path, formatted.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write mutated document subject source: %w", err)
	}
	return nil
}

func changeInteger(spec mutation.Spec, key string) (int, error) {
	value, ok := spec.Change[key]
	if !ok {
		return 0, fmt.Errorf("mutation %s is missing change.%s", spec.ID, key)
	}
	switch value := value.(type) {
	case int:
		return value, nil
	case int64:
		return int(value), nil
	case int32:
		return int(value), nil
	case float64:
		converted := int(value)
		if float64(converted) == value {
			return converted, nil
		}
	case string:
		converted, err := strconv.Atoi(value)
		if err == nil {
			return converted, nil
		}
	}
	return 0, fmt.Errorf("mutation %s change.%s must be an integer", spec.ID, key)
}
