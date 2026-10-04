package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"ingen/herdr-sentinel/internal/agentprobe"
)

type compareAgentFunc func(context.Context, agentprobe.MatrixRequest) (agentprobe.MatrixReport, error)

func agentCompareCommand(args []string) int {
	return agentCompareCommandWith(args, os.Stdout, os.Stderr, agentprobe.Compare)
}

func agentCompareCommandWith(args []string, stdout, stderr io.Writer, compareAgent compareAgentFunc) int {
	flags := flag.NewFlagSet("sentinel agent compare", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var executables repeatedString
	flags.Var(&executables, "agent-executable", "absolute Codex CLI executable candidate (repeat 1-4 times)")
	timeoutSeconds := flags.Int("timeout-seconds", 45, "maximum synthetic diagnostic runtime per candidate (1-120 seconds)")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: sentinel agent compare --agent-executable <absolute-path> [--agent-executable <absolute-path> ...] [--timeout-seconds 1..120]")
		return 2
	}
	if len(executables) < 1 || len(executables) > 4 || *timeoutSeconds < 1 || *timeoutSeconds > 120 {
		fmt.Fprintln(stderr, "sentinel agent compare requires 1-4 executable candidates and a timeout from 1 to 120 seconds")
		return 2
	}
	if compareAgent == nil {
		fmt.Fprintln(stderr, "sentinel agent compare is unavailable")
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	report, err := compareAgent(ctx, agentprobe.MatrixRequest{
		ExecutablePaths: append([]string(nil), executables...),
		Timeout:         time.Duration(*timeoutSeconds) * time.Second,
	})
	if err != nil {
		fmt.Fprintf(stderr, "sentinel agent compare: %v\n", err)
		return 2
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintln(stderr, "sentinel agent compare could not write its JSON result")
		return 2
	}
	switch report.Status {
	case "supported":
		return 0
	case "unsupported":
		return 1
	case "indeterminate":
		return 2
	default:
		return 2
	}
}
