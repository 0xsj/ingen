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
	summaryPath := flags.String("summary-output", "", "preparation summary path; defaults inside output-dir")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if len(flags.Args()) != 0 {
		fmt.Fprintln(os.Stderr, "usage: sorna-go-provider [--plan <path>] [--source-root <dir>] [--output-dir <dir>] [--binary-dir <dir>] [--build-package <package>] [--provider <path>] [--summary-output <path>]")
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
	preparationOutput := *summaryPath
	if preparationOutput == "" {
		preparationOutput = filepath.Join(*outputDir, "preparation.json")
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
		Capabilities: []campaign.ProviderCapability{
			{Plane: "implementation", Operator: "response.status.replace", Target: "POST /documents"},
			{Plane: "implementation", Operator: "response.field.remove", Target: "POST /documents"},
			{Plane: "implementation", Operator: "response.field.add", Target: "POST /documents"},
			{Plane: "implementation", Operator: "response.error.status.replace", Target: "POST /documents"},
		},
		SubjectArgs: []string{"-addr", "${SORA_ADDR}"},
		Mutate:      mutateDocumentPipeline,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	result.Preparation.PlanPath = *planPath
	providerHash, err := goprovider.WriteManifest(providerOutput, result.Provider)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if _, err := campaign.LoadProviderFile(providerOutput); err != nil {
		fmt.Fprintln(os.Stderr, "verify generated provider:", err)
		return 1
	}
	preparationHash, err := campaign.WritePreparationSummary(preparationOutput, result.Preparation)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("provider: %s\nhash: %s\npreparation: %s\npreparation hash: %s\nvariants: %d\n", providerOutput, providerHash, preparationOutput, preparationHash, len(result.Variants))
	for _, variant := range result.Variants {
		fmt.Printf("mutation: %s\nbinary: %s\nbinary hash: %s\n", variant.MutationID, variant.BinaryPath, variant.BinarySHA256)
	}
	return 0
}

// mutateDocumentPipeline is intentionally narrow. It proves that the Go
// provider can apply reviewed AST changes to an isolated copy; the generic
// provider package owns copying/building, while this lab owns target meaning.
func mutateDocumentPipeline(variantRoot string, spec mutation.Spec) (campaign.ProviderProvenance, error) {
	if spec.Plane != "implementation" {
		return campaign.ProviderProvenance{}, fmt.Errorf("Go document provider only supports implementation mutations")
	}
	if spec.Target != "POST /documents" {
		return campaign.ProviderProvenance{}, fmt.Errorf("unsupported document mutation %q at %q", spec.Operator, spec.Target)
	}

	operator := spec.Operator
	var provenance campaign.ProviderProvenance
	var fieldToRemove string
	var fieldToAdd string
	var valueToAdd string
	switch operator {
	case "response.status.replace":
		from, err := changeInteger(spec, "from")
		if err != nil {
			return campaign.ProviderProvenance{}, err
		}
		to, err := changeInteger(spec, "to")
		if err != nil {
			return campaign.ProviderProvenance{}, err
		}
		if from != 202 || to != 200 {
			return campaign.ProviderProvenance{}, fmt.Errorf("document provider supports only status 202 -> 200, got %d -> %d", from, to)
		}
		provenance = campaign.ProviderProvenance{
			Location: "examples/document-pipeline-lab/subject/server.go:createDocument:writeJSON.status",
			Before:   "http.StatusAccepted",
			After:    "http.StatusOK",
		}
	case "response.field.remove":
		field, err := changeString(spec, "field")
		if err != nil {
			return campaign.ProviderProvenance{}, err
		}
		if field != "name" {
			return campaign.ProviderProvenance{}, fmt.Errorf("document provider supports removing only response field %q, got %q", "name", field)
		}
		fieldToRemove = field
		provenance = campaign.ProviderProvenance{
			Location: "examples/document-pipeline-lab/subject/server.go:createDocument:writeJSON.body.name",
			Before:   "name: name",
			After:    "name field removed",
		}
	case "response.field.add":
		field, err := changeString(spec, "field")
		if err != nil {
			return campaign.ProviderProvenance{}, err
		}
		value, err := changeString(spec, "value")
		if err != nil {
			return campaign.ProviderProvenance{}, err
		}
		if field != "debug" || value != "mutation" {
			return campaign.ProviderProvenance{}, fmt.Errorf("document provider supports only adding debug=mutation, got %s=%s", field, value)
		}
		fieldToAdd = field
		valueToAdd = value
		provenance = campaign.ProviderProvenance{
			Location: "examples/document-pipeline-lab/subject/server.go:createDocument:writeJSON.body",
			Before:   "id, name, status",
			After:    "id, name, status, debug=mutation",
		}
	case "response.error.status.replace":
		code, err := changeString(spec, "code")
		if err != nil {
			return campaign.ProviderProvenance{}, err
		}
		from, err := changeInteger(spec, "from")
		if err != nil {
			return campaign.ProviderProvenance{}, err
		}
		to, err := changeInteger(spec, "to")
		if err != nil {
			return campaign.ProviderProvenance{}, err
		}
		if code != "unsupported_document_type" || from != 400 || to != 500 {
			return campaign.ProviderProvenance{}, fmt.Errorf("document provider supports only unsupported_document_type status 400 -> 500, got %s %d -> %d", code, from, to)
		}
		provenance = campaign.ProviderProvenance{
			Location: "examples/document-pipeline-lab/subject/server.go:createDocument:writeError.unsupported_document_type.status",
			Before:   "http.StatusBadRequest",
			After:    "http.StatusInternalServerError",
		}
	default:
		return campaign.ProviderProvenance{}, fmt.Errorf("unsupported document mutation %q at %q", spec.Operator, spec.Target)
	}

	path := filepath.Join(variantRoot, "examples", "document-pipeline-lab", "subject", "server.go")
	contents, err := os.ReadFile(path)
	if err != nil {
		return campaign.ProviderProvenance{}, fmt.Errorf("read document subject source: %w", err)
	}
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, contents, parser.ParseComments)
	if err != nil {
		return campaign.ProviderProvenance{}, fmt.Errorf("parse document subject source: %w", err)
	}
	var statusTargets []*ast.SelectorExpr
	var errorStatusTargets []*ast.SelectorExpr
	type fieldTarget struct {
		body  *ast.CompositeLit
		index int
	}
	var fieldTargets []fieldTarget
	var bodyTargets []*ast.CompositeLit
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
			if !ok {
				return true
			}
			if operator == "response.error.status.replace" {
				if callee.Name != "writeError" || len(call.Args) < 3 {
					return true
				}
				status, ok := call.Args[1].(*ast.SelectorExpr)
				if !ok || status.Sel.Name != "StatusBadRequest" {
					return true
				}
				packageName, ok := status.X.(*ast.Ident)
				if !ok || packageName.Name != "http" {
					return true
				}
				code, ok := call.Args[2].(*ast.BasicLit)
				if !ok || code.Kind != token.STRING {
					return true
				}
				value, err := strconv.Unquote(code.Value)
				if err != nil || value != "unsupported_document_type" {
					return true
				}
				errorStatusTargets = append(errorStatusTargets, status)
				return true
			}
			if callee.Name != "writeJSON" {
				return true
			}
			if operator == "response.status.replace" {
				status, ok := call.Args[1].(*ast.SelectorExpr)
				if !ok || status.Sel.Name != "StatusAccepted" {
					return true
				}
				packageName, ok := status.X.(*ast.Ident)
				if !ok || packageName.Name != "http" {
					return true
				}
				statusTargets = append(statusTargets, status)
				return true
			}
			if len(call.Args) < 3 {
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
			body, ok := call.Args[2].(*ast.CompositeLit)
			if !ok {
				return true
			}
			if _, ok := body.Type.(*ast.MapType); !ok {
				return true
			}
			if operator == "response.field.add" {
				for _, element := range body.Elts {
					keyValue, ok := element.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					key, ok := keyValue.Key.(*ast.BasicLit)
					if !ok || key.Kind != token.STRING {
						continue
					}
					name, err := strconv.Unquote(key.Value)
					if err == nil && name == fieldToAdd {
						return true
					}
				}
				bodyTargets = append(bodyTargets, body)
				return true
			}
			for index, element := range body.Elts {
				keyValue, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := keyValue.Key.(*ast.BasicLit)
				if !ok || key.Kind != token.STRING {
					continue
				}
				name, err := strconv.Unquote(key.Value)
				if err != nil || name != fieldToRemove {
					continue
				}
				fieldTargets = append(fieldTargets, fieldTarget{body: body, index: index})
			}
			return true
		})
	}
	candidateCount := len(statusTargets)
	if operator == "response.field.remove" {
		candidateCount = len(fieldTargets)
	} else if operator == "response.field.add" {
		candidateCount = len(bodyTargets)
	} else if operator == "response.error.status.replace" {
		candidateCount = len(errorStatusTargets)
	}
	if candidateCount != 1 {
		return campaign.ProviderProvenance{}, campaign.NewTargetResolutionError(spec, provenance.Location, candidateCount)
	}
	if operator == "response.status.replace" {
		statusTargets[0].Sel.Name = "StatusOK"
	} else if operator == "response.field.remove" {
		target := fieldTargets[0]
		target.body.Elts = append(target.body.Elts[:target.index], target.body.Elts[target.index+1:]...)
	} else if operator == "response.field.add" {
		target := bodyTargets[0]
		target.Elts = append(target.Elts, &ast.KeyValueExpr{
			Key:   &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(fieldToAdd)},
			Value: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(valueToAdd)},
		})
	} else {
		errorStatusTargets[0].Sel.Name = "StatusInternalServerError"
	}
	provenance.TargetResolution = &campaign.TargetResolution{
		Selector:       provenance.Location,
		CandidateCount: candidateCount,
		AppliedCount:   1,
	}
	var formatted bytes.Buffer
	if err := format.Node(&formatted, fileSet, file); err != nil {
		return campaign.ProviderProvenance{}, fmt.Errorf("format mutated document subject source: %w", err)
	}
	if err := os.WriteFile(path, formatted.Bytes(), 0o644); err != nil {
		return campaign.ProviderProvenance{}, fmt.Errorf("write mutated document subject source: %w", err)
	}
	return provenance, nil
}

func changeString(spec mutation.Spec, key string) (string, error) {
	value, ok := spec.Change[key]
	if !ok {
		return "", fmt.Errorf("mutation %s is missing change.%s", spec.ID, key)
	}
	converted, ok := value.(string)
	if !ok || converted == "" {
		return "", fmt.Errorf("mutation %s change.%s must be a non-empty string", spec.ID, key)
	}
	return converted, nil
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
