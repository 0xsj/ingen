package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	paddockbaseline "ingen/paddock/internal/baseline"
	"ingen/paddock/internal/checker"
	paddockexplain "ingen/paddock/internal/explain"
	"ingen/paddock/internal/model"
	"ingen/paddock/internal/report"
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
	case "baseline":
		if err := createBaseline(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "paddock:", err)
			os.Exit(2)
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
		return fmt.Errorf("check requires --policy <path>")
	}
	result, err := checker.Check(root, policyPath)
	if err != nil {
		return err
	}
	if baselinePath != "" {
		snapshot, err := paddockbaseline.Load(baselinePath)
		if err != nil {
			return err
		}
		if err := paddockbaseline.Apply(result, snapshot, baselinePath); err != nil {
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
	snapshot := paddockbaseline.Build(result)
	if err := paddockbaseline.Save(outputPath, snapshot); err != nil {
		return err
	}
	_, err = fmt.Fprintf(os.Stdout, "BASELINE %s (%d findings)\n", outputPath, len(snapshot.Entries))
	return err
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
	fmt.Fprintln(os.Stderr, "usage: paddock check <source-root> --policy <policy.yaml> [--baseline <file>] [--format text|json]")
	fmt.Fprintln(os.Stderr, "       paddock baseline <source-root> --policy <policy.yaml> --output <baseline.json>")
	fmt.Fprintln(os.Stderr, "       paddock explain <paddock-report.json> [--format text|json]")
}
