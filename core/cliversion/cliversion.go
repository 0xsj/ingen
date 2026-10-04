// Package cliversion provides the common version document and CLI handling
// used by InGen command line tools.
package cliversion

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime"
)

// These values may be set by release builds with Go's -ldflags -X option.
// Defaults deliberately distinguish unknown build provenance from known
// runtime facts such as the Go toolchain and target platform.
var (
	Version            = "dev"
	Revision           = "unknown"
	SourceInputsSHA256 = "unknown"
	Dirty              = "unknown"
	BuildDate          = "unknown"
	Toolchain          = runtime.Version()
)

const Schema = "ingen.tool-version/v1"

// Info is the stable machine-readable version record shared by InGen tools.
// Commit remains as a compatibility alias for Revision; BuildDate remains for
// existing Sorna and Paddock consumers.
type Info struct {
	Schema             string `json:"schema"`
	Name               string `json:"name"`
	Version            string `json:"version"`
	Revision           string `json:"revision"`
	Commit             string `json:"commit"`
	BuildDate          string `json:"build_date"`
	SourceInputsSHA256 string `json:"source_inputs_sha256"`
	Dirty              string `json:"dirty"`
	GOOS               string `json:"goos"`
	GOARCH             string `json:"goarch"`
	Toolchain          string `json:"toolchain"`
}

// Legacy carries module-specific values still populated by existing Sorna and
// Paddock release scripts. Shared injected metadata takes precedence when set.
type Legacy struct {
	Version   string
	Commit    string
	BuildDate string
}

// Current constructs a version document for name. Legacy values are fallbacks
// for old release scripts that have not yet injected the shared variables.
func Current(name string, legacy Legacy) Info {
	version, revision, buildDate := Version, Revision, BuildDate
	if version == "dev" && legacy.Version != "" {
		version = legacy.Version
	}
	if revision == "unknown" && legacy.Commit != "" {
		revision = legacy.Commit
	}
	if buildDate == "unknown" && legacy.BuildDate != "" {
		buildDate = legacy.BuildDate
	}
	return Info{
		Schema:             Schema,
		Name:               name,
		Version:            version,
		Revision:           revision,
		Commit:             revision,
		BuildDate:          buildDate,
		SourceInputsSHA256: SourceInputsSHA256,
		Dirty:              Dirty,
		GOOS:               runtime.GOOS,
		GOARCH:             runtime.GOARCH,
		Toolchain:          Toolchain,
	}
}

// Dispatch handles the common top-level --version flag and version command.
// It returns handled=false for ordinary commands, leaving their dispatch to
// the caller. The version command accepts no arguments, or exactly
// `--format text|json` (also `-f`).
func Dispatch(name string, args []string, stdout, stderr io.Writer, legacy Legacy) (handled bool, exitCode int) {
	if len(args) == 0 || (args[0] != "--version" && args[0] != "version") {
		return false, 0
	}
	if args[0] == "--version" {
		if len(args) != 1 {
			fmt.Fprintln(stderr, "--version does not accept arguments")
			return true, 2
		}
		return true, writeVersion(name, "text", stdout, stderr, legacy)
	}

	format := "text"
	if len(args) > 1 {
		if len(args) != 3 || (args[1] != "--format" && args[1] != "-f") {
			fmt.Fprintln(stderr, "version accepts only --format text|json")
			return true, 2
		}
		format = args[2]
	}
	return true, writeVersion(name, format, stdout, stderr, legacy)
}

func writeVersion(name, format string, stdout, stderr io.Writer, legacy Legacy) int {
	info := Current(name, legacy)
	switch format {
	case "text":
		_, err := fmt.Fprintf(stdout, "%s %s\ncommit %s\nbuilt %s\nrevision %s\nsource_inputs_sha256 %s\ndirty %s\nplatform %s/%s\ntoolchain %s\n",
			info.Name, info.Version, info.Commit, info.BuildDate, info.Revision,
			info.SourceInputsSHA256, info.Dirty, info.GOOS, info.GOARCH, info.Toolchain)
		if err != nil {
			fmt.Fprintln(stderr, "version:", err)
			return 1
		}
		return 0
	case "json":
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(info); err != nil {
			fmt.Fprintln(stderr, "version:", err)
			return 1
		}
		return 0
	default:
		fmt.Fprintf(stderr, "version: unsupported format %q; use text or json\n", format)
		return 2
	}
}
