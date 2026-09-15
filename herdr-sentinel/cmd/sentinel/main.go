package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	sentinelrun "ingen/herdr-sentinel/internal/run"
	"ingen/herdr-sentinel/internal/workspace"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) < 3 || args[0] != "workspace" || args[1] != "validate" {
		if len(args) == 0 || args[0] != "run" {
			usage()
			return 2
		}
		return runCommand(args[1:])
	}
	flags := flag.NewFlagSet("sentinel workspace validate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args[2:]); err != nil {
		return 2
	}
	if len(flags.Args()) != 1 {
		usage()
		return 2
	}
	if _, err := workspace.LoadFile(flags.Args()[0]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("valid:", filepath.Clean(flags.Args()[0]))
	return 0
}

func runCommand(args []string) int {
	if len(args) == 0 || args[0] != "bootstrap" {
		usage()
		return 2
	}
	flags := flag.NewFlagSet("sentinel run bootstrap", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	workspacePath := flags.String("workspace", "", "Sentinel workspace manifest")
	outputPath := flags.String("output", "", "path for the Sentinel lifecycle receipt")
	runID := flags.String("run-id", "", "optional explicit run ID")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if *workspacePath == "" || *outputPath == "" || len(flags.Args()) != 0 {
		usage()
		return 2
	}
	receipt, err := sentinelrun.New(*workspacePath, time.Now().UTC())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if *runID != "" {
		receipt.RunID = *runID
	}
	if err := os.MkdirAll(filepath.Dir(*outputPath), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := sentinelrun.SaveFile(*outputPath, receipt); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("created:", filepath.Clean(*outputPath))
	return 0
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: sentinel workspace validate <path>")
	fmt.Fprintln(os.Stderr, "       sentinel run bootstrap --workspace <path> --output <path>")
}
