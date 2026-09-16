package adaptertest_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"ingen/paddock/internal/adaptertest"
)

func TestLoadRejectsInvalidManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "adapter-tests.yaml")
	contents := `schema: paddock.adapter-tests/v1
adapter:
  executable: python3
cases:
  - name: broken
    root: workspace
    language: rust
    expect: maybe
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := adaptertest.Load(path)
	if err == nil || !strings.Contains(err.Error(), "expect must be pass or error") {
		t.Fatalf("manifest error = %v", err)
	}
}

func TestRunRecordsMultipleAdapterModes(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "../../.."))
	adapter := filepath.Join(repoRoot, "paddock", "examples", "adapter", "conformance-adapter.py")
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "workspace"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(directory, "adapter-tests.yaml")
	contents := fmt.Sprintf(`schema: paddock.adapter-tests/v1
adapter:
  executable: python3
  args:
    - %s
    - --workspace
    - "{{root}}"
cases:
  - name: rust-file
    root: workspace
    language: rust
    source_unit: file
    required_edge_kinds: [import]
    expect: pass
    package_count: 2
    edge_count: 1
  - name: unsupported-language
    root: workspace
    language: go
    source_unit: file
    expect: error
    error_contains: expects language rust
`, adapter)
	if err := os.WriteFile(manifestPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	document, err := adaptertest.Run(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if document.Status != "PASS" || document.Passed != 2 || document.Failed != 0 || len(document.Cases) != 2 {
		t.Fatalf("unexpected adapter test result: %#v", document)
	}
	if document.Cases[0].Actual != "pass" || document.Cases[0].PackageCount != 2 || document.Cases[0].EdgeCount != 1 {
		t.Fatalf("unexpected passing adapter case: %#v", document.Cases[0])
	}
	if document.Cases[1].Actual != "error" || document.Cases[1].ErrorCode != "language-mismatch" || document.Cases[1].Error == "" {
		t.Fatalf("unexpected expected-error adapter case: %#v", document.Cases[1])
	}
	if err := document.Validate(); err != nil {
		t.Fatalf("valid adapter test result rejected: %v", err)
	}
	resultPath := filepath.Join(directory, "adapter-test-result.json")
	if err := adaptertest.Save(resultPath, document); err != nil {
		t.Fatal(err)
	}
	loaded, err := adaptertest.LoadResult(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := adaptertest.VerifyFiles(loaded); err != nil {
		t.Fatal(err)
	}
}

func TestRunUsesAdapterProfile(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "../../.."))
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "workspace"), 0o755); err != nil {
		t.Fatal(err)
	}
	profilePath := filepath.Join(directory, "profile.yaml")
	adapter := filepath.Join(repoRoot, "paddock", "examples", "adapter", "conformance-adapter.py")
	profile := fmt.Sprintf(`schema: paddock.adapter-profile/v1
name: conformance
executable: python3
args:
  - %s
  - --workspace
  - "{{root}}"
`, adapter)
	if err := os.WriteFile(profilePath, []byte(profile), 0o600); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(directory, "adapter-tests.yaml")
	manifest := `schema: paddock.adapter-tests/v1
adapter:
  profile: profile.yaml
cases:
  - name: rust-file
    root: workspace
    language: rust
    source_unit: file
    required_edge_kinds: [import]
    expect: pass
    package_count: 2
    edge_count: 1
`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	document, err := adaptertest.Run(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if document.Status != "PASS" || document.Adapter.Profile != "profile.yaml" || document.Adapter.Executable != "python3" || document.Profile == nil || document.Profile.Path != profilePath || document.Cases[0].Status != "PASS" {
		t.Fatalf("profile-backed adapter test result is incomplete: %#v", document)
	}
}

func TestDocumentValidateRejectsInconsistentEvidence(t *testing.T) {
	document := adaptertest.Document{
		Schema:   adaptertest.DocumentSchema,
		Manifest: adaptertest.FileRef{Path: "tests.yaml", SHA256: strings.Repeat("a", 64)},
		Adapter:  adaptertest.Adapter{Executable: "python3"},
		Status:   "PASS",
		Passed:   0,
		Cases: []adaptertest.CaseResult{{
			Name:             "broken",
			Root:             "/workspace",
			Language:         "rust",
			Expected:         "pass",
			Actual:           "error",
			Status:           "PASS",
			AssertionsPassed: true,
			Error:            "adapter failed",
		}},
	}
	if err := document.Validate(); err == nil || !strings.Contains(err.Error(), "counts do not match") {
		t.Fatalf("inconsistent adapter test result was accepted: %v", err)
	}
}
