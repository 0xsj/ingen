package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	malcolmcontract "ingen/sorna/internal/malcolm"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}
	if args[0] == "-h" || args[0] == "--help" {
		printUsage(stdout)
		return 0
	}

	inputPath := args[0]
	flags := flag.NewFlagSet("sorna-malcolm", flag.ContinueOnError)
	flags.SetOutput(stderr)
	outputPath := flags.String("output", "", "write the Sorna contract to this path instead of stdout")
	mutationsOutputPath := flags.String("mutations-output", "", "write Malcolm mutation declarations as a Sorna catalogue")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "unexpected extra arguments")
		printUsage(stderr)
		return 2
	}

	ir, err := malcolmcontract.LoadFile(inputPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if len(ir.Specification.Mutations) > 0 && *mutationsOutputPath == "" {
		fmt.Fprintln(stderr, "Malcolm IR contains mutation declarations; use --mutations-output to preserve them")
		return 1
	}
	if len(ir.Specification.Mutations) == 0 && *mutationsOutputPath != "" {
		fmt.Fprintln(stderr, "--mutations-output requires at least one mutation declaration")
		return 1
	}
	document, err := malcolmcontract.Translate(ir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if *outputPath != "" {
		if err := malcolmcontract.WriteFile(*outputPath, document); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stdout, "wrote:", *outputPath)
	}

	if *outputPath == "" {
		encoder := json.NewEncoder(stdout)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(map[string]any{"contract": document.Contract}); err != nil {
			fmt.Fprintln(stderr, "encode translated contract:", err)
			return 1
		}
	}

	if *mutationsOutputPath != "" {
		catalogue, err := malcolmcontract.TranslateMutationCatalogue(ir)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if err := malcolmcontract.WriteMutationCatalogue(*mutationsOutputPath, catalogue); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stderr, "wrote:", *mutationsOutputPath)
	}
	return 0
}

func printUsage(writer io.Writer) {
	fmt.Fprintln(writer, "usage: sorna-malcolm <malcolm-ir.json> [--output <contract.json>] [--mutations-output <catalogue.json>]")
	fmt.Fprintln(writer, "translate Malcolm IR into a Sorna draft contract and, when requested, a mutation catalogue")
}
