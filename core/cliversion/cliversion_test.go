package cliversion

import (
	"bytes"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

func TestCurrentUsesTruthfulDevelopmentDefaultsAndLegacyFallback(t *testing.T) {
	info := Current("example", Legacy{Version: "1.2.3", Commit: "abc123", BuildDate: "2026-10-04T00:00:00Z"})
	if info.Schema != Schema || info.Name != "example" || info.Version != "1.2.3" || info.Revision != "abc123" || info.Commit != info.Revision {
		t.Fatalf("unexpected identity metadata: %+v", info)
	}
	if info.SourceInputsSHA256 != "unknown" || info.Dirty != "unknown" || info.BuildDate != "2026-10-04T00:00:00Z" {
		t.Fatalf("unexpected unknown/build date metadata: %+v", info)
	}
	if info.GOOS != runtime.GOOS || info.GOARCH != runtime.GOARCH || info.Toolchain != runtime.Version() {
		t.Fatalf("runtime metadata not reported accurately: %+v", info)
	}
}

func TestDispatchFormatsAndStrictArguments(t *testing.T) {
	legacy := Legacy{Version: "2.0", Commit: "deadbeef", BuildDate: "unknown"}
	var stdout, stderr bytes.Buffer
	if handled, code := Dispatch("paddock", []string{"--version"}, &stdout, &stderr, legacy); !handled || code != 0 {
		t.Fatalf("Dispatch(--version) = %v, %d; want true, 0 (stderr %q)", handled, code, stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "paddock 2.0\ncommit deadbeef\nbuilt unknown\n") {
		t.Fatalf("legacy text prefix changed: %q", stdout.String())
	}
	stdout.Reset()
	if handled, code := Dispatch("paddock", []string{"version", "--format", "json"}, &stdout, &stderr, legacy); !handled || code != 0 {
		t.Fatalf("Dispatch(json) = %v, %d; stderr %q", handled, code, stderr.String())
	}
	var info Info
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.Schema != Schema || info.Name != "paddock" || info.Version != "2.0" || info.Revision != "deadbeef" || info.Commit != "deadbeef" {
		t.Fatalf("unexpected JSON: %+v", info)
	}
	for _, args := range [][]string{{"version", "--format"}, {"version", "--format", "yaml"}, {"version", "extra"}, {"--version", "--format", "json"}} {
		stdout.Reset()
		stderr.Reset()
		if handled, code := Dispatch("paddock", args, &stdout, &stderr, Legacy{}); !handled || code != 2 {
			t.Errorf("Dispatch(%q) = %v, %d; want true, 2", args, handled, code)
		}
	}
}

func TestDispatchLeavesOtherCommandsAlone(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if handled, code := Dispatch("sorna", []string{"contract", "validate"}, &stdout, &stderr, Legacy{}); handled || code != 0 {
		t.Fatalf("ordinary command was handled: %v, %d", handled, code)
	}
}
