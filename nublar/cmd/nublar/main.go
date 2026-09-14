package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"ingen/nublar/internal/aggregate"
	"ingen/nublar/internal/workflow"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	if args[0] == "workflow" {
		return workflowCommand(args[1:])
	}
	if args[0] != "aggregate" {
		usage()
		return 2
	}
	flags := flag.NewFlagSet("aggregate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	output := flags.String("output", "", "path for the Nublar aggregate; stdout when empty")
	workflowPath := flags.String("workflow", "", "Nublar workflow file declaring the result inputs")
	root := flags.String("root", ".", "workspace root for paths in --workflow")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if *workflowPath != "" && len(flags.Args()) > 0 {
		fmt.Fprintln(os.Stderr, "--workflow cannot be combined with positional CI results")
		return 2
	}
	if *workflowPath == "" && len(flags.Args()) == 0 {
		usage()
		return 2
	}
	var report aggregate.Report
	var err error
	if *workflowPath != "" {
		report, err = aggregate.ComposeWorkflowFile(*workflowPath, *root)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	} else {
		report, err = aggregate.ComposeFiles(flags.Args())
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	}
	if *output == "" {
		if err := aggregate.WriteJSON(os.Stdout, report); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	} else if err := aggregate.SaveFile(*output, report); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return report.ExitCode
}

func workflowCommand(args []string) int {
	if len(args) != 2 || args[0] != "validate" {
		usage()
		return 2
	}
	if _, err := workflow.LoadFile(args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("valid:", filepath.Clean(args[1]))
	return 0
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  nublar workflow validate <path>")
	fmt.Fprintln(os.Stderr, "  nublar aggregate [--workflow <path> --root <dir>] [--output <path>] <ci-result> [<ci-result> ...]")
}
