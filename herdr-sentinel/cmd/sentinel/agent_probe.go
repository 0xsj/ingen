package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"ingen/herdr-sentinel/internal/agentprobe"
)

func agentCommand(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	switch args[0] {
	case "diagnose":
		return agentDiagnoseCommand(args[1:])
	case "compare":
		return agentCompareCommand(args[1:])
	default:
		usage()
		return 2
	}
}

func agentDiagnoseCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel agent diagnose", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	executable := flags.String("agent-executable", "", "absolute path to the Codex CLI executable")
	timeoutSeconds := flags.Int("timeout-seconds", 45, "maximum diagnostic runtime in seconds (1-120)")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: sentinel agent diagnose --agent-executable <absolute-path> [--timeout-seconds 1..120]")
		return 2
	}
	if *executable == "" || !filepath.IsAbs(*executable) || filepath.Clean(*executable) != *executable || *timeoutSeconds < 1 || *timeoutSeconds > 120 {
		fmt.Fprintln(os.Stderr, "sentinel agent diagnose requires a clean absolute Codex executable path and timeout from 1 to 120 seconds")
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	report, err := agentprobe.Diagnose(ctx, agentprobe.Request{ExecutablePath: *executable, Timeout: time.Duration(*timeoutSeconds) * time.Second})
	if err != nil {
		fmt.Fprintf(os.Stderr, "sentinel agent diagnose: %v\n", err)
		return 2
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintln(os.Stderr, "sentinel agent diagnose could not write its JSON result")
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
		if errors.Is(ctx.Err(), context.Canceled) {
			return 2
		}
		return 2
	}
}
