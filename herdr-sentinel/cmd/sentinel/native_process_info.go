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

func nativeSessionProcessInfoCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel session native-process-info", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("root", ".", "project root containing the native session journal")
	path := flags.String("path", "", "native session journal path relative to the project root")
	socket := flags.String("socket", "", "explicit Herdr UNIX socket path")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *path == "" || *socket == "" || len(flags.Args()) != 0 {
		fmt.Fprintln(os.Stderr, "native-process-info requires --path and explicit --socket")
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
	observation, err := nativesession.ObservePaneProcesses(context.Background(), *root, *path, &herdrclient.Client{SocketPath: *socket})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := json.NewEncoder(os.Stdout).Encode(observation); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func nativeSessionSnapshotCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel session native-snapshot", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	socket := flags.String("socket", "", "explicit Herdr UNIX socket path")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *socket == "" || len(flags.Args()) != 0 {
		fmt.Fprintln(os.Stderr, "native-snapshot requires an explicit --socket")
		return 2
	}
	if os.Getenv("HERDR_ENV") != "1" {
		fmt.Fprintln(os.Stderr, "native Herdr operations require HERDR_ENV=1; run Sentinel inside the supported Herdr environment")
		return 1
	}
	client := &herdrclient.Client{SocketPath: *socket}
	server, err := client.Ping(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if server.Version != nativesession.SupportedHerdrVersion || server.Protocol != nativesession.SupportedHerdrProtocol {
		fmt.Fprintf(os.Stderr, "unsupported Herdr host %q protocol %d; Sentinel requires Herdr %s protocol %d\n", server.Version, server.Protocol, nativesession.SupportedHerdrVersion, nativesession.SupportedHerdrProtocol)
		return 1
	}
	snapshot, err := client.Snapshot(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if _, err := os.Stdout.Write(append(snapshot, '\n')); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
