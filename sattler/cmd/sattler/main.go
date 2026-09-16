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
	if len(args) == 0 || args[0] != "compare" {
		usage()
		return 2
	}

	flags := flag.NewFlagSet("compare", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	format := flags.String("format", "text", "output format: text or json")
	output := flags.String("output", "", "output path; stdout when empty")
	if err := flags.Parse(args[1:]); err != nil {
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
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	var writer = os.Stdout
	var file *os.File
	if *output != "" {
		file, err = os.Create(*output)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
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
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: sattler compare [--format text|json] [--output path] BEFORE AFTER")
}
