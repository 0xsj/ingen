package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestAgentDiagnoseRejectsInvalidInvocationBeforeRunning(t *testing.T) {
	oldOut, oldErr := os.Stdout, os.Stderr
	outRead, outWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errRead, errWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = outWrite, errWrite
	t.Cleanup(func() {
		os.Stdout, os.Stderr = oldOut, oldErr
		_ = outRead.Close()
		_ = outWrite.Close()
		_ = errRead.Close()
		_ = errWrite.Close()
	})

	if code := run([]string{"agent", "diagnose", "--agent-executable", "relative/codex"}); code != 2 {
		t.Fatalf("relative executable exit = %d, want usage error 2", code)
	}
	if code := run([]string{"agent", "diagnose", "--agent-executable", filepath.Join(t.TempDir(), "missing-codex")}); code != 2 {
		t.Fatalf("missing executable exit = %d, want setup error 2", code)
	}
	_ = outWrite.Close()
	_ = errWrite.Close()
	output := make([]byte, 4096)
	n, _ := outRead.Read(output)
	if bytes.Contains(output[:n], []byte("supported")) {
		t.Fatal("invalid diagnostic invocation emitted a supported report")
	}
}
