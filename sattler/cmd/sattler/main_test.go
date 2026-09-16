package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"ingen/sattler"
)

func TestBundleCompareCommandReportsJSONManifestErrors(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "comparison.json")
	if err := os.WriteFile(manifestPath, []byte(`{"schema":"ingen.sattler-comparison-input/v0","before":{"ci_result":"before.json"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	stderr := captureStderr(t, func() int {
		return bundleCompareCommand([]string{"compare", "--format", "json", manifestPath})
	})
	var document sattler.ErrorDocument
	if err := json.Unmarshal([]byte(stderr), &document); err != nil {
		t.Fatalf("stderr = %q, decode error = %v", stderr, err)
	}
	if document.Schema != sattler.ErrorSchema || document.Operation != "bundle compare" {
		t.Fatalf("error document = %+v, want Sattler error envelope", document)
	}
	if len(document.Errors) != 1 || document.Errors[0].Code != "incomplete-pair" {
		t.Fatalf("error issues = %+v, want incomplete-pair", document.Errors)
	}
}

func captureStderr(t *testing.T, run func() int) string {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stderr
	os.Stderr = write
	code := run()
	if closeErr := write.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	os.Stderr = previous
	contents, readErr := io.ReadAll(read)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if closeErr := read.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if code != 1 {
		t.Fatalf("command exit code = %d, want 1", code)
	}
	return string(contents)
}
