package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"ingen/core/cliversion"
)

func TestVersionSurfaceUsesCommonExecutableIdentity(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"version", "--format", "json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("version command exit code = %d, stderr = %q", code, stderr.String())
	}
	var info cliversion.Info
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		t.Fatalf("decode version JSON: %v", err)
	}
	if info.Schema != cliversion.Schema || info.Name != "sorna-malcolm" || info.Version != "dev" || info.Revision != "unknown" || info.Commit != info.Revision {
		t.Fatalf("unexpected version document: %+v", info)
	}
}

func TestVersionSurfaceRejectsUnknownFormatsAndExtraArguments(t *testing.T) {
	for _, args := range [][]string{
		{"version", "--format", "yaml"},
		{"version", "extra"},
		{"--version", "--format", "json"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 2 {
			t.Errorf("run(%q) exit code = %d, want 2 (stdout %q, stderr %q)", args, code, stdout.String(), stderr.String())
		}
	}
}
