package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"ingen/herdr-sentinel/internal/provenance"
)

func provenanceCommand(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	switch args[0] {
	case "start":
		return provenanceStartCommand(args[1:])
	case "execute":
		return provenanceExecuteCommand(args[1:])
	default:
		usage()
		return 2
	}
}

func provenanceStartCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel provenance start", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("root", "", "absolute project root")
	output := flags.String("output", "", "project-relative Amber context path")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *root == "" || !filepath.IsAbs(*root) || *output == "" || len(flags.Args()) != 0 {
		fmt.Fprintln(os.Stderr, "provenance start requires --root ABS --output REL")
		return 2
	}
	value, digest, err := provenance.Start(*root, *output)
	if err != nil {
		fmt.Fprintln(os.Stderr, "provenance start:", err)
		return 2
	}
	fmt.Printf("Amber work: %s\nexecution: %s\ncontext: %s\nsha256: %s\n", value.WorkID(), value.ExecutionID(), *output, digest)
	return 0
}

func provenanceExecuteCommand(args []string) int {
	separator := -1
	for index, arg := range args {
		if arg == "--" {
			separator = index
			break
		}
	}
	if separator < 0 || separator == len(args)-1 {
		fmt.Fprintln(os.Stderr, "provenance execute requires -- before the child command")
		return 2
	}
	flags := flag.NewFlagSet("sentinel provenance execute", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("root", "", "absolute project root")
	parent := flags.String("parent", "", "project-relative Amber parent context")
	output := flags.String("output", "", "project-relative Amber child context path")
	receiptPath := flags.String("receipt", "", "project-relative Sentinel execution receipt")
	operation := flags.String("operation", "", "simple operation name")
	expectedParent := flags.String("expected-parent-sha256", "", "expected parent context digest")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *root == "" || !filepath.IsAbs(*root) || *parent == "" || *output == "" || *receiptPath == "" || *operation == "" || len(flags.Args()) == 0 {
		fmt.Fprintln(os.Stderr, "provenance execute requires --root ABS --parent REL --output REL --receipt REL --operation NAME -- COMMAND...")
		return 2
	}
	ctx, stop := provenance.SignalContext(context.Background())
	defer stop()
	receipt, err := provenance.Execute(ctx, provenance.Request{
		Root: *root, ParentPath: *parent, OutputPath: *output, ReceiptPath: *receiptPath,
		ExpectedParentSHA256: *expectedParent, Operation: *operation, Command: flags.Args(),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "provenance execute: %v\n", err)
	}
	if receipt.Schema != provenance.ReceiptSchema {
		return 2
	}
	if err != nil && receipt.Status == "completed" {
		return 2
	}
	fmt.Printf("Amber execution: %s\nstatus: %s\ncontext: %s\nreceipt: %s\nstdout: %s\nstderr: %s\n", receipt.ExecutionID, receipt.Status, receipt.ContextPath, receipt.ReceiptPath, receipt.StdoutPath, receipt.StderrPath)
	switch receipt.Status {
	case "completed":
		return 0
	case "failed":
		return 1
	default:
		return 2
	}
}
