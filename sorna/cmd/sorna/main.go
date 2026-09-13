package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"ingen/sorna/internal/contract"
	"ingen/sorna/internal/mutation"
	"ingen/sorna/internal/runner"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}

	switch args[0] {
	case "contract":
		return contractCommand(args[1:])
	case "run":
		return runSubject(args[1:])
	default:
		fmt.Fprintln(os.Stderr, "unknown command:", args[0])
		usage()
		return 2
	}
}

func contractCommand(args []string) int {
	if len(args) < 1 {
		usage()
		return 2
	}

	switch args[0] {
	case "validate":
		if len(args) != 2 {
			fmt.Fprintln(os.Stderr, "usage: sorna contract validate <path>")
			return 2
		}
		if _, err := contract.LoadFile(args[1]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println("valid:", args[1])
		return 0
	case "seal":
		return seal(args[1:])
	default:
		fmt.Fprintln(os.Stderr, "unknown contract command:", args[0])
		usage()
		return 2
	}
}

func seal(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: sorna contract seal <path> --output-dir <dir>")
		return 2
	}
	flags := flag.NewFlagSet("seal", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	outputDir := flags.String("output-dir", "", "directory for sealed artifacts")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if *outputDir == "" {
		fmt.Fprintln(os.Stderr, "--output-dir is required")
		return 2
	}
	sealed, err := contract.SealFile(args[0], *outputDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("sealed:", sealed.SHA256)
	return 0
}

func runSubject(args []string) int {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	contractPath := flags.String("contract", "", "path to the contract")
	baseURL := flags.String("base-url", "", "absolute URL of the running subject")
	outputDir := flags.String("output-dir", ".artifacts/document-pipeline-run", "directory for the run record")
	variant := flags.String("subject-variant", "clean-baseline", "label for the subject variant")
	mutationID := flags.String("mutation-id", "", "identity of the mutation being evaluated")
	mutationPlane := flags.String("mutation-plane", "behavior", "mutation plane")
	mutationDescription := flags.String("mutation-description", "", "description of the mutation")
	expectedRule := flags.String("expected-rule", "", "rule ID expected to observe the mutation")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *contractPath == "" || *baseURL == "" {
		fmt.Fprintln(os.Stderr, "usage: sorna run --contract <path> --base-url <url> [--output-dir <dir>]")
		return 2
	}

	document, err := contract.LoadFile(*contractPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	sealed, err := contract.Seal(document)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	var mutationSpec *mutation.Spec
	if *mutationID != "" {
		if *expectedRule == "" {
			fmt.Fprintln(os.Stderr, "--expected-rule is required when --mutation-id is provided")
			return 2
		}
		mutationSpec = &mutation.Spec{
			ID:              *mutationID,
			Plane:           *mutationPlane,
			Description:     *mutationDescription,
			ExpectedRuleIDs: []string{*expectedRule},
		}
	}
	record, err := runner.Execute(context.Background(), sealed, runner.Config{BaseURL: *baseURL, Variant: *variant, Mutation: mutationSpec})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	path, err := runner.Write(*outputDir, record)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("run: %s\ncontract: %s (%s)\nsummary: %d passed, %d failed, %d errors, %d inconclusive, %d skipped\n", path, record.Verdict.Status, record.Verdict.Reason, record.Summary.Passed, record.Summary.Failed, record.Summary.Errors, record.Summary.Inconclusive, record.Summary.Skipped)
	if record.Mutation != nil {
		fmt.Printf("mutation: %s -> %s\n", record.Mutation.Spec.ID, record.Mutation.Outcome)
	}
	return exitCodeFor(record)
}

func exitCodeFor(record runner.RunRecord) int {
	if record.Mutation != nil {
		if record.Mutation.Outcome == "killed" {
			return 0
		}
		return 1
	}
	if record.Verdict.Status != "pass" {
		return 1
	}
	return 0
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  sorna contract validate <path>")
	fmt.Fprintln(os.Stderr, "  sorna contract seal <path> --output-dir <dir>")
	fmt.Fprintln(os.Stderr, "  sorna run --contract <path> --base-url <url> [--output-dir <dir>]")
}
