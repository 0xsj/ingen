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
		return 0
	}

	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(map[string]any{"contract": document.Contract}); err != nil {
		fmt.Fprintln(stderr, "encode translated contract:", err)
		return 1
	}
	return 0
}

func printUsage(writer io.Writer) {
	fmt.Fprintln(writer, "usage: sorna-malcolm <malcolm-ir.json> [--output <contract.json>]")
	fmt.Fprintln(writer, "translate the supported Malcolm IR subset into a Sorna draft contract")
}
