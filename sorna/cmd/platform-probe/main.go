package main

import (
	"encoding/json"
	"fmt"
	"os"

	"ingen/sorna/internal/platformprobe"
)

const seccompChildArgument = "--internal-seccomp-probe-child"

func main() {
	if len(os.Args) == 2 && os.Args[1] == seccompChildArgument {
		if err := writeJSON(os.Stdout, platformprobe.RunSeccompProbeChild()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: platform-probe")
		os.Exit(2)
	}
	report := platformprobe.Probe(platformprobe.Options{
		SeccompChildCommand: []string{os.Args[0], seccompChildArgument},
		SeccompChildEnv:     []string{},
	})
	if err := writeJSON(os.Stdout, report); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func writeJSON(output *os.File, value any) error {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("write capability report: %w", err)
	}
	return nil
}
