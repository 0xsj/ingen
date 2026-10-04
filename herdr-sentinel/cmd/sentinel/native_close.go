package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"ingen/herdr-sentinel/internal/herdrclient"
	"ingen/herdr-sentinel/internal/nativesession"
)

func nativeSessionCloseCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel session native-close", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("root", ".", "project root containing the native session journal")
	path := flags.String("path", "", "native session journal path relative to the project root")
	socket := flags.String("socket", "", "explicit Herdr UNIX socket path")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *path == "" || *socket == "" || len(flags.Args()) != 0 {
		fmt.Fprintln(os.Stderr, "native-close requires --path and explicit --socket")
		return 2
	}
	if os.Getenv("HERDR_ENV") != "1" {
		fmt.Fprintln(os.Stderr, "native Herdr operations require HERDR_ENV=1; run Sentinel inside the supported Herdr environment")
		return 1
	}
	loaded, err := nativesession.Load(*root, *path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if loaded.Intent.HerdrSocket != *socket {
		fmt.Fprintln(os.Stderr, "--socket does not match the explicit Herdr endpoint bound to this native session journal")
		return 2
	}
	result, err := nativesession.CloseOwnedWorkspace(context.Background(), *root, *path, &herdrclient.Client{SocketPath: *socket})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
