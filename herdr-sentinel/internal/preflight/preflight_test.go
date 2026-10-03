package preflight

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	sentinelproject "ingen/herdr-sentinel/internal/project"
	sornacontract "ingen/sorna/contract"
)

func TestRunReportsFreshProjectAsIncomplete(t *testing.T) {
	root := t.TempDir()
	if _, err := sentinelproject.Initialize(sentinelproject.Options{Root: root, ID: "fresh-project"}); err != nil {
		t.Fatal(err)
	}

	result, err := Run(root, sentinelproject.WorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "incomplete" {
		t.Fatalf("status = %q, want incomplete", result.Status)
	}
	if result.Assurance != "unverified" {
		t.Fatalf("assurance = %q, want unverified", result.Assurance)
	}
	if !hasCheck(result, "contract", "pending") || !hasCheck(result, "host-enforcement", "pending") {
		t.Fatalf("checks = %+v, want pending contract and host-enforcement checks", result.Checks)
	}
}

func TestRunRecognizesVerifiedSealedSnapshot(t *testing.T) {
	root := t.TempDir()
	if _, err := sentinelproject.Initialize(sentinelproject.Options{Root: root, ID: "sealed-contract"}); err != nil {
		t.Fatal(err)
	}
	contractPath := filepath.Join(root, ".ingen", "contract", "contract.json")
	contract := `{"contract":{"schema":"ingen.contract/v1","id":"sealed-contract","version":1,"status":"draft","interface":{"kind":"http-json"},"rules":[],"unspecified":[]}}`
	if err := os.WriteFile(contractPath, []byte(contract), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := sornacontract.SealFile(contractPath, filepath.Dir(contractPath)); err != nil {
		t.Fatal(err)
	}

	result, err := Run(root, sentinelproject.WorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "incomplete" || !hasCheck(result, "contract", "pass") {
		t.Fatalf("status = %q, checks = %+v, want passing sealed contract and incomplete host handoff", result.Status, result.Checks)
	}
}

func TestRunReportsInvalidContractAsBlocked(t *testing.T) {
	root := t.TempDir()
	if _, err := sentinelproject.Initialize(sentinelproject.Options{Root: root, ID: "invalid-contract"}); err != nil {
		t.Fatal(err)
	}
	contractPath := filepath.Join(root, ".ingen", "contract", "contract.json")
	if err := os.WriteFile(contractPath, []byte(`{"contract":{"schema":"ingen.contract/v1"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Run(root, sentinelproject.WorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "blocked" {
		t.Fatalf("status = %q, want blocked", result.Status)
	}
	if !hasCheck(result, "contract", "fail") {
		t.Fatalf("checks = %+v, want failed contract check", result.Checks)
	}
}

func TestRunReportsContractPolicyIDMismatchAsBlocked(t *testing.T) {
	root := t.TempDir()
	if _, err := sentinelproject.Initialize(sentinelproject.Options{Root: root, ID: "policy-project"}); err != nil {
		t.Fatal(err)
	}
	contractPath := filepath.Join(root, ".ingen", "contract", "contract.json")
	contract := `{"contract":{"schema":"ingen.contract/v1","id":"different-contract","version":1,"status":"draft","interface":{"kind":"http-json"},"rules":[],"unspecified":[]}}`
	if err := os.WriteFile(contractPath, []byte(contract), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Run(root, sentinelproject.WorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "blocked" || !hasCheck(result, "policy-binding", "fail") {
		t.Fatalf("status = %q, checks = %+v, want blocked policy binding", result.Status, result.Checks)
	}
}

func TestRunDoesNotPassPolicyBindingWhenPolicyCannotBeLoaded(t *testing.T) {
	root := t.TempDir()
	if _, err := sentinelproject.Initialize(sentinelproject.Options{Root: root, ID: "policy-project"}); err != nil {
		t.Fatal(err)
	}
	contractPath := filepath.Join(root, ".ingen", "contract", "contract.json")
	contract := `{"contract":{"schema":"ingen.contract/v1","id":"policy-project","version":1,"status":"draft","interface":{"kind":"http-json"},"rules":[],"unspecified":[]}}`
	if err := os.WriteFile(contractPath, []byte(contract), 0o644); err != nil {
		t.Fatal(err)
	}
	oraclePolicyPath := filepath.Join(root, ".ingen", "policy", "oracle.yaml")
	policy, err := os.ReadFile(oraclePolicyPath)
	if err != nil {
		t.Fatal(err)
	}
	brokenPolicy := strings.Replace(string(policy), "subject_id: policy-project", "subject_id: ''", 1)
	if err := os.WriteFile(oraclePolicyPath, []byte(brokenPolicy), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Run(root, sentinelproject.WorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "blocked" || hasCheck(result, "policy-binding", "pass") {
		t.Fatalf("status = %q, checks = %+v; invalid oracle policy must not produce a passing binding check", result.Status, result.Checks)
	}
}

func TestWriteTextIncludesOwnersAndPaths(t *testing.T) {
	result := Result{
		WorkspaceID: "example",
		Workspace:   ".ingen/workspace.yaml",
		Status:      "incomplete",
		Assurance:   "unverified",
		Checks:      []Check{{ID: "contract", Owner: "malcolm", Status: "pending", Path: ".ingen/contract/contract.json", Detail: "write the contract"}},
	}
	var output strings.Builder
	if err := result.WriteText(&output); err != nil {
		t.Fatal(err)
	}
	for _, wanted := range []string{"example", "incomplete", "contract", "malcolm", ".ingen/contract/contract.json"} {
		if !strings.Contains(output.String(), wanted) {
			t.Fatalf("text output %q does not contain %q", output.String(), wanted)
		}
	}
}

func hasCheck(result Result, id, status string) bool {
	for _, check := range result.Checks {
		if check.ID == id && check.Status == status {
			return true
		}
	}
	return false
}
