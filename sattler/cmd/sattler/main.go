package main

import (
	"flag"
	"fmt"
	"os"

	"ingen/sattler"
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
	case "compare":
		return compareCommand(args[1:])
	case "run":
		return runCompareCommand(args[1:])
	case "custody":
		return custodyCompareCommand(args[1:])
	case "provenance":
		return provenanceCompareCommand(args[1:])
	case "bundle":
		return bundleCompareCommand(args[1:])
	default:
		usage()
		return 2
	}
}

func compareCommand(args []string) int {
	flags := flag.NewFlagSet("compare", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	format := flags.String("format", "text", "output format: text or json")
	output := flags.String("output", "", "output path; stdout when empty")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if len(flags.Args()) != 2 {
		fmt.Fprintln(os.Stderr, "compare requires BEFORE and AFTER CI result paths")
		return 2
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintln(os.Stderr, "--format must be text or json")
		return 2
	}

	report, err := sattler.CompareFiles(flags.Arg(0), flags.Arg(1))
	if err != nil {
		return reportOperationError(*format, "compare", err)
	}

	var writer = os.Stdout
	var file *os.File
	if *output != "" {
		file, err = os.Create(*output)
		if err != nil {
			return reportOperationError(*format, "compare", err)
		}
		defer file.Close()
		writer = file
	}

	if *format == "json" {
		err = sattler.WriteJSON(writer, report)
	} else {
		err = sattler.WriteText(writer, report)
	}
	if err != nil {
		return reportOperationError(*format, "compare", err)
	}
	return 0
}

func runCompareCommand(args []string) int {
	if len(args) == 0 || args[0] != "compare" {
		usage()
		return 2
	}
	flags := flag.NewFlagSet("run compare", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	format := flags.String("format", "text", "output format: text or json")
	output := flags.String("output", "", "output path; stdout when empty")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if len(flags.Args()) != 2 {
		fmt.Fprintln(os.Stderr, "run compare requires BEFORE and AFTER Nublar run paths")
		return 2
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintln(os.Stderr, "--format must be text or json")
		return 2
	}

	report, err := sattler.CompareNublarRunFiles(flags.Arg(0), flags.Arg(1))
	if err != nil {
		return reportOperationError(*format, "run compare", err)
	}

	var writer = os.Stdout
	var file *os.File
	if *output != "" {
		file, err = os.Create(*output)
		if err != nil {
			return reportOperationError(*format, "run compare", err)
		}
		defer file.Close()
		writer = file
	}

	if *format == "json" {
		err = sattler.WriteNublarJSON(writer, report)
	} else {
		err = sattler.WriteNublarText(writer, report)
	}
	if err != nil {
		return reportOperationError(*format, "run compare", err)
	}
	return 0
}

func custodyCompareCommand(args []string) int {
	if len(args) == 0 || args[0] != "compare" {
		usage()
		return 2
	}
	flags := flag.NewFlagSet("custody compare", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	format := flags.String("format", "text", "output format: text or json")
	output := flags.String("output", "", "output path; stdout when empty")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if len(flags.Args()) != 2 {
		fmt.Fprintln(os.Stderr, "custody compare requires BEFORE and AFTER Lockwood custody paths")
		return 2
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintln(os.Stderr, "--format must be text or json")
		return 2
	}

	report, err := sattler.CompareLockwoodCustodyFiles(flags.Arg(0), flags.Arg(1))
	if err != nil {
		return reportOperationError(*format, "custody compare", err)
	}

	var writer = os.Stdout
	var file *os.File
	if *output != "" {
		file, err = os.Create(*output)
		if err != nil {
			return reportOperationError(*format, "custody compare", err)
		}
		defer file.Close()
		writer = file
	}

	if *format == "json" {
		err = sattler.WriteLockwoodJSON(writer, report)
	} else {
		err = sattler.WriteLockwoodText(writer, report)
	}
	if err != nil {
		return reportOperationError(*format, "custody compare", err)
	}
	return 0
}

func provenanceCompareCommand(args []string) int {
	if len(args) == 0 || args[0] != "compare" {
		usage()
		return 2
	}
	flags := flag.NewFlagSet("provenance compare", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	format := flags.String("format", "text", "output format: text or json")
	output := flags.String("output", "", "output path; stdout when empty")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if len(flags.Args()) != 2 {
		fmt.Fprintln(os.Stderr, "provenance compare requires BEFORE and AFTER Amber provenance paths")
		return 2
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintln(os.Stderr, "--format must be text or json")
		return 2
	}

	report, err := sattler.CompareAmberProvenanceFiles(flags.Arg(0), flags.Arg(1))
	if err != nil {
		return reportOperationError(*format, "provenance compare", err)
	}

	var writer = os.Stdout
	var file *os.File
	if *output != "" {
		file, err = os.Create(*output)
		if err != nil {
			return reportOperationError(*format, "provenance compare", err)
		}
		defer file.Close()
		writer = file
	}

	if *format == "json" {
		err = sattler.WriteAmberProvenanceJSON(writer, report)
	} else {
		err = sattler.WriteAmberProvenanceText(writer, report)
	}
	if err != nil {
		return reportOperationError(*format, "provenance compare", err)
	}
	return 0
}

func bundleCompareCommand(args []string) int {
	if len(args) == 0 || args[0] != "compare" {
		usage()
		return 2
	}
	flags := flag.NewFlagSet("bundle compare", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	format := flags.String("format", "text", "output format: text or json")
	output := flags.String("output", "", "output path; stdout when empty")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if len(flags.Args()) != 1 {
		fmt.Fprintln(os.Stderr, "bundle compare requires one comparison manifest path")
		return 2
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintln(os.Stderr, "--format must be text or json")
		return 2
	}

	report, err := sattler.CompareBundleFile(flags.Arg(0))
	if err != nil {
		return reportOperationError(*format, "bundle compare", err)
	}

	var writer = os.Stdout
	var file *os.File
	if *output != "" {
		file, err = os.Create(*output)
		if err != nil {
			return reportOperationError(*format, "bundle compare", err)
		}
		defer file.Close()
		writer = file
	}

	if *format == "json" {
		err = sattler.WriteBundleJSON(writer, report)
	} else {
		err = sattler.WriteBundleText(writer, report)
	}
	if err != nil {
		return reportOperationError(*format, "bundle compare", err)
	}
	return 0
}

func reportOperationError(format, operation string, err error) int {
	if format == "json" {
		if writeErr := sattler.WriteErrorJSON(os.Stderr, operation, err); writeErr == nil {
			return 1
		}
	}
	fmt.Fprintln(os.Stderr, err)
	return 1
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: sattler compare [--format text|json] [--output path] BEFORE AFTER")
	fmt.Fprintln(os.Stderr, "       sattler run compare [--format text|json] [--output path] BEFORE AFTER")
	fmt.Fprintln(os.Stderr, "       sattler custody compare [--format text|json] [--output path] BEFORE AFTER")
	fmt.Fprintln(os.Stderr, "       sattler provenance compare [--format text|json] [--output path] BEFORE AFTER")
	fmt.Fprintln(os.Stderr, "       sattler bundle compare [--format text|json] [--output path] MANIFEST")
}
