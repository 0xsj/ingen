package policytest_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"ingen/paddock/internal/policy"
	"ingen/paddock/internal/policytest"
)

func TestLoadRejectsInvalidManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tests.yaml")
	if err := os.WriteFile(path, []byte("schema: paddock.policy-tests/v1\ncases:\n  - name: broken\n    root: service\n    expect: maybe\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := policytest.Load(path)
	if err == nil || !strings.Contains(err.Error(), "expect must be pass, fail, or error") {
		t.Fatalf("manifest error = %v", err)
	}
}

func TestRunRecordsExpectedFixtureOutcomes(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "../../.."))
	policyPath := filepath.Join(repoRoot, "paddock", "examples", "hexagonal.yaml")
	goodRoot := filepath.Join(repoRoot, "paddock", "examples", "services", "hexagonal-go", "good")
	violatingRoot := filepath.Join(repoRoot, "paddock", "examples", "services", "hexagonal-go", "violating")
	manifestPath := filepath.Join(t.TempDir(), "tests.yaml")
	contents := "schema: paddock.policy-tests/v1\ncases:\n" +
		"  - name: good\n    root: " + goodRoot + "\n    expect: pass\n" +
		"  - name: violating\n    root: " + violatingRoot + "\n    expect: fail\n    require_rules: [domain-is-pure]\n"
	if err := os.WriteFile(manifestPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	config, err := policy.Load(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	document, err := policytest.Run(manifestPath, policyPath, config)
	if err != nil {
		t.Fatal(err)
	}
	if document.Schema != policytest.DocumentSchema || document.Status != "PASS" || document.Passed != 2 || document.Failed != 0 || len(document.Cases) != 2 {
		t.Fatalf("unexpected policy test document: %#v", document)
	}
	if document.Cases[0].Actual != "pass" || document.Cases[1].Actual != "fail" {
		t.Fatalf("unexpected case outcomes: %#v", document.Cases)
	}
	if len(document.Cases[1].MissingRules) != 0 {
		t.Fatalf("required rule was not found: %#v", document.Cases[1])
	}
	if len(document.Cases[0].FindingRules) != 0 || len(document.Cases[1].FindingRules) != 1 || document.Cases[1].FindingRules[0] != "domain-is-pure" {
		t.Fatalf("unexpected finding rule IDs: %#v", document.Cases)
	}
}
