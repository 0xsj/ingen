package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

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
	case "sorna":
		return sornaCompareCommand(args[1:])
	case "custody":
		return custodyCompareCommand(args[1:])
	case "provenance":
		return provenanceCompareCommand(args[1:])
	case "bundle":
		return bundleCompareCommand(args[1:])
	case "series":
		return seriesCommand(args[1:])
	case "investigate":
		return investigateCommand(args[1:])
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
	var changeIDs stringListFlag
	flags.Var(&changeIDs, "change-id", "include only this stable change ID; repeatable or comma-separated")
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
	report = sattler.FilterComparisonChanges(report, changeIDs)

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
	var changeIDs stringListFlag
	flags.Var(&changeIDs, "change-id", "include only this stable change ID; repeatable or comma-separated")
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
	report = sattler.FilterNublarRunChanges(report, changeIDs)

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

func sornaCompareCommand(args []string) int {
	if len(args) == 0 || args[0] != "compare" {
		usage()
		return 2
	}
	flags := flag.NewFlagSet("sorna compare", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	format := flags.String("format", "text", "output format: text or json")
	output := flags.String("output", "", "output path; stdout when empty")
	var changeIDs stringListFlag
	flags.Var(&changeIDs, "change-id", "include only this stable change ID; repeatable or comma-separated")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if len(flags.Args()) != 2 {
		fmt.Fprintln(os.Stderr, "sorna compare requires BEFORE and AFTER Sorna run paths")
		return 2
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintln(os.Stderr, "--format must be text or json")
		return 2
	}

	report, err := sattler.CompareSornaRunFiles(flags.Arg(0), flags.Arg(1))
	if err != nil {
		return reportOperationError(*format, "sorna compare", err)
	}
	report = sattler.FilterSornaRunChanges(report, changeIDs)

	var writer = os.Stdout
	var file *os.File
	if *output != "" {
		file, err = os.Create(*output)
		if err != nil {
			return reportOperationError(*format, "sorna compare", err)
		}
		defer file.Close()
		writer = file
	}

	if *format == "json" {
		err = sattler.WriteSornaJSON(writer, report)
	} else {
		err = sattler.WriteSornaText(writer, report)
	}
	if err != nil {
		return reportOperationError(*format, "sorna compare", err)
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
	var changeIDs stringListFlag
	flags.Var(&changeIDs, "change-id", "include only this stable change ID; repeatable or comma-separated")
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
	report = sattler.FilterLockwoodCustodyChanges(report, changeIDs)

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
	var changeIDs stringListFlag
	flags.Var(&changeIDs, "change-id", "include only this stable change ID; repeatable or comma-separated")
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
	report = sattler.FilterAmberProvenanceChanges(report, changeIDs)

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
	summaryOnly := flags.Bool("summary-only", false, "emit only the bundle summary")
	var changeIDs stringListFlag
	flags.Var(&changeIDs, "change-id", "include only this stable change ID; repeatable or comma-separated")
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
	report = sattler.FilterBundleChanges(report, changeIDs)

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

	if *summaryOnly && *format == "json" {
		err = sattler.WriteBundleSummaryJSON(writer, report)
	} else if *summaryOnly {
		err = sattler.WriteBundleSummaryText(writer, report)
	} else if *format == "json" {
		err = sattler.WriteBundleJSON(writer, report)
	} else {
		err = sattler.WriteBundleText(writer, report)
	}
	if err != nil {
		return reportOperationError(*format, "bundle compare", err)
	}
	return 0
}

func seriesCommand(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	switch args[0] {
	case "compare":
		return seriesCompareCommand(args)
	case "query":
		return seriesQueryCommand(args[1:])
	default:
		usage()
		return 2
	}
}

func seriesCompareCommand(args []string) int {
	if len(args) == 0 || args[0] != "compare" {
		usage()
		return 2
	}
	flags := flag.NewFlagSet("series compare", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	format := flags.String("format", "text", "output format: text or json")
	output := flags.String("output", "", "output path; stdout when empty")
	summaryOnly := flags.Bool("summary-only", false, "emit aggregate series summary without points")
	latestOnly := flags.Bool("latest-only", false, "emit only the final ordered series point")
	var changeIDs stringListFlag
	flags.Var(&changeIDs, "change-id", "include only this stable change ID; repeatable or comma-separated")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if len(flags.Args()) != 1 {
		fmt.Fprintln(os.Stderr, "series compare requires one comparison series manifest path")
		return 2
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintln(os.Stderr, "--format must be text or json")
		return 2
	}
	if *summaryOnly && *latestOnly {
		fmt.Fprintln(os.Stderr, "--summary-only and --latest-only cannot be used together")
		return 2
	}

	series, err := sattler.CompareSeriesManifestFileWithChanges(flags.Arg(0), changeIDs)
	if err != nil {
		return reportOperationError(*format, "series compare", err)
	}

	var writer = os.Stdout
	var file *os.File
	if *output != "" {
		file, err = os.Create(*output)
		if err != nil {
			return reportOperationError(*format, "series compare", err)
		}
		defer file.Close()
		writer = file
	}

	if *latestOnly && *format == "json" {
		err = sattler.WriteSeriesLatestJSON(writer, series)
	} else if *latestOnly {
		err = sattler.WriteSeriesLatestText(writer, series)
	} else if *summaryOnly && *format == "json" {
		err = sattler.WriteSeriesSummaryJSON(writer, series)
	} else if *summaryOnly {
		err = sattler.WriteSeriesSummaryText(writer, series)
	} else if *format == "json" {
		err = sattler.WriteSeriesJSON(writer, series)
	} else {
		err = sattler.WriteSeriesText(writer, series)
	}
	if err != nil {
		return reportOperationError(*format, "series compare", err)
	}
	return 0
}

func seriesQueryCommand(args []string) int {
	flags := flag.NewFlagSet("series query", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	format := flags.String("format", "text", "output format: text or json")
	output := flags.String("output", "", "output path; stdout when empty")
	rule := flags.String("rule", "", "find this exact Sorna rule ID")
	mutation := flags.String("mutation", "", "find this exact mutation ID")
	contract := flags.String("contract", "", "find this exact contract ID, version, digest, or file reference")
	provider := flags.String("provider", "", "find this exact provider file reference")
	workflow := flags.String("workflow", "", "find this exact workflow ID, path, or digest")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if len(flags.Args()) != 1 {
		fmt.Fprintln(os.Stderr, "series query requires one comparison series manifest path")
		return 2
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintln(os.Stderr, "--format must be text or json")
		return 2
	}

	selectors := []struct {
		kind  sattler.SeriesQueryKind
		value string
	}{
		{kind: sattler.SeriesQueryRule, value: *rule},
		{kind: sattler.SeriesQueryMutation, value: *mutation},
		{kind: sattler.SeriesQueryContract, value: *contract},
		{kind: sattler.SeriesQueryProvider, value: *provider},
		{kind: sattler.SeriesQueryWorkflow, value: *workflow},
	}
	var selected sattler.SeriesQuerySelector
	selectorCount := 0
	for _, candidate := range selectors {
		if strings.TrimSpace(candidate.value) == "" {
			continue
		}
		selectorCount++
		selected = sattler.SeriesQuerySelector{Kind: candidate.kind, Value: strings.TrimSpace(candidate.value)}
	}
	if selectorCount != 1 {
		fmt.Fprintln(os.Stderr, "series query requires exactly one of --rule, --mutation, --contract, --provider, or --workflow")
		return 2
	}

	report, err := sattler.QuerySeriesManifestFile(flags.Arg(0), selected)
	if err != nil {
		return reportOperationError(*format, "series query", err)
	}

	var writer = os.Stdout
	var file *os.File
	if *output != "" {
		file, err = os.Create(*output)
		if err != nil {
			return reportOperationError(*format, "series query", err)
		}
		defer file.Close()
		writer = file
	}

	if *format == "json" {
		err = sattler.WriteSeriesQueryJSON(writer, report)
	} else {
		err = sattler.WriteSeriesQueryText(writer, report)
	}
	if err != nil {
		return reportOperationError(*format, "series query", err)
	}
	return 0
}

func investigateCommand(args []string) int {
	flags := flag.NewFlagSet("investigate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	format := flags.String("format", "text", "output format: text or json")
	output := flags.String("output", "", "output path; stdout when empty")
	var changeIDs stringListFlag
	flags.Var(&changeIDs, "change-id", "include only this stable change ID; repeatable or comma-separated")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if len(flags.Args()) != 1 {
		fmt.Fprintln(os.Stderr, "investigate requires one comparison manifest path")
		return 2
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintln(os.Stderr, "--format must be text or json")
		return 2
	}

	bundle, err := sattler.CompareBundleFile(flags.Arg(0))
	if err != nil {
		return reportOperationError(*format, "investigate", err)
	}
	bundle = sattler.FilterBundleChanges(bundle, changeIDs)
	report := sattler.NewInvestigationReport(bundle)

	var writer = os.Stdout
	var file *os.File
	if *output != "" {
		file, err = os.Create(*output)
		if err != nil {
			return reportOperationError(*format, "investigate", err)
		}
		defer file.Close()
		writer = file
	}

	if *format == "json" {
		err = sattler.WriteInvestigationJSON(writer, report)
	} else {
		err = sattler.WriteInvestigationText(writer, report)
	}
	if err != nil {
		return reportOperationError(*format, "investigate", err)
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

type stringListFlag []string

func (flag *stringListFlag) String() string {
	return strings.Join(*flag, ",")
}

func (flag *stringListFlag) Set(value string) error {
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			return fmt.Errorf("change ID cannot be empty")
		}
		*flag = append(*flag, item)
	}
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: sattler compare [--format text|json] [--change-id id] [--output path] BEFORE AFTER")
	fmt.Fprintln(os.Stderr, "       sattler run compare [--format text|json] [--change-id id] [--output path] BEFORE AFTER")
	fmt.Fprintln(os.Stderr, "       sattler sorna compare [--format text|json] [--change-id id] [--output path] BEFORE AFTER")
	fmt.Fprintln(os.Stderr, "       sattler custody compare [--format text|json] [--change-id id] [--output path] BEFORE AFTER")
	fmt.Fprintln(os.Stderr, "       sattler provenance compare [--format text|json] [--change-id id] [--output path] BEFORE AFTER")
	fmt.Fprintln(os.Stderr, "       sattler bundle compare [--format text|json] [--summary-only] [--change-id id] [--output path] MANIFEST")
	fmt.Fprintln(os.Stderr, "       sattler series compare [--format text|json] [--summary-only|--latest-only] [--change-id id] [--output path] MANIFEST")
	fmt.Fprintln(os.Stderr, "       sattler series query [--format text|json] (--rule id|--mutation id|--contract value|--provider value|--workflow value) [--output path] MANIFEST")
	fmt.Fprintln(os.Stderr, "       sattler investigate [--format text|json] [--change-id id] [--output path] MANIFEST")
}
