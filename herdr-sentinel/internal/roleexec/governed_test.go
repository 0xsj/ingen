package roleexec

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"ingen/herdr-sentinel/internal/workflowgate"
	"ingen/sorna/contract"
	"ingen/sorna/oracle"
	"ingen/sorna/policy"
)

func TestCompareGateRequiresFrozenOraclePinAfterFreeze(t *testing.T) {
	result := workflowgate.Result{Stage: workflowgate.StageImplementation,
		WorkspaceManifestSHA256: testDigest("manifest"), ApprovalSHA256: testDigest("approval"),
		ReviewPolicySHA256: testDigest("review"), ContractSHA256: testDigest("contract"),
		ContractSourceSHA256: testDigest("contract-source"), OraclePolicySHA256: testDigest("oracle-policy"),
		OraclePolicyFileSHA256: testDigest("oracle-policy-file"), OracleSHA256: testDigest("oracle")}
	request := Request{ExpectedManifestSHA256: result.WorkspaceManifestSHA256,
		ExpectedApprovalSHA256: result.ApprovalSHA256, ExpectedReviewPolicySHA256: result.ReviewPolicySHA256,
		ExpectedContractSHA256: result.ContractSHA256, ExpectedContractSourceSHA256: result.ContractSourceSHA256,
		ExpectedOraclePolicySHA256: result.OraclePolicySHA256, ExpectedOraclePolicyFileSHA256: result.OraclePolicyFileSHA256}
	if err := compareGate(request, result); err == nil || !strings.Contains(err.Error(), "frozen oracle digest") {
		t.Fatalf("compareGate without oracle pin = %v, want frozen oracle digest error", err)
	}
	request.ExpectedOracleSHA256 = result.OracleSHA256
	if err := compareGate(request, result); err != nil {
		t.Fatalf("compareGate with exact oracle pin: %v", err)
	}
}

func TestGovernedExecutePinsExactApprovedInputsBeforeLaunching(t *testing.T) {
	requireDarwinGovernedExecution(t)
	tests := []struct {
		name   string
		mutate func(*testing.T, *governedFixture)
	}{
		{"contract source drift", func(t *testing.T, f *governedFixture) {
			writeTestFile(t, f.root, f.contractSourcePath, []byte(`{"contract":{"schema":"ingen.contract/v1","id":"demo","version":2,"status":"draft","interface":{"kind":"http"},"rules":[{"id":"health","strength":"must","subject":"GET /healthz","expect":{"status":200}}],"unspecified":[]}}`))
		}},
		{"approval drift", func(t *testing.T, f *governedFixture) {
			writeTestFile(t, f.root, f.approvalPath, []byte(`{"schema":"ingen.hammond-governance/v1","record_id":"tampered","state":"rejected","events":[]}`))
		}},
		{"authority drift", func(t *testing.T, f *governedFixture) {
			writeTestFile(t, f.root, f.authorityPath, []byte(`{"schema":"ingen.hammond-authority/v1","id":"reviewers","version":1,"actors":{"reviewer":["changed-role"]}}`))
		}},
		{"frozen oracle drift", func(t *testing.T, f *governedFixture) {
			contents, err := os.ReadFile(filepath.Join(f.root, f.oraclePath))
			if err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, f.root, f.oraclePath, append(contents, '\n'))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newGovernedFixture(t, governedFixtureOptions{})
			request := fixture.executionRequest("drift-" + strings.ReplaceAll(test.name, " ", "-"))
			fixture.prepareAndPersist(t, &request)
			test.mutate(t, &fixture)
			if _, _, err := Execute(context.Background(), request); err == nil {
				t.Fatal("Execute accepted changed governed input")
			}
			fixture.assertChildNotStarted(t, request.ExecutionID)
		})
	}
}

func TestGovernedExecuteSucceedsWithExactPinnedArtifacts(t *testing.T) {
	requireDarwinGovernedExecution(t)
	fixture := newGovernedFixture(t, governedFixtureOptions{})
	request := fixture.executionRequest("governed-success")
	fixture.prepareAndPersist(t, &request)
	report, reportPath, err := Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if report.Status != "completed" || report.ExitCode == nil || *report.ExitCode != 0 || report.Governance == nil {
		t.Fatalf("execution report = %+v", report)
	}
	wantPath := filepath.ToSlash(filepath.Join(".ingen", "artifacts", "role-executions", request.ExecutionID+".json"))
	if reportPath != wantPath {
		t.Fatalf("report path = %q, want %q", reportPath, wantPath)
	}
}

func TestPrepareProtectsSelectedGovernancePathsEvenWhenCustom(t *testing.T) {
	requireDarwinGovernedExecution(t)
	tests := []struct {
		name    string
		options governedFixtureOptions
		want    string
	}{
		{"approval record", governedFixtureOptions{approvalPath: ".ingen/custom/approval.json", writeRoot: ".ingen/custom"}, "protected input"},
		{"approved canonical contract", governedFixtureOptions{contractArtifactPath: ".ingen/custom/approved-contract.json", writeRoot: ".ingen/custom"}, "protected input"},
		{"nested authority snapshot", governedFixtureOptions{authorityPath: ".ingen/custom/authority.json", writeRoot: ".ingen/custom"}, "protected input"},
		{"active review policy", governedFixtureOptions{reviewPolicyPath: ".ingen/custom/review-policy.json", writeRoot: ".ingen/custom"}, "protected input"},
		{"frozen oracle", governedFixtureOptions{oraclePath: ".ingen/custom/frozen-oracle.json", writeRoot: ".ingen/custom"}, "frozen oracle input"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newGovernedFixture(t, test.options)
			request := fixture.executionRequest("protected-" + strings.ReplaceAll(test.name, " ", "-"))
			_, err := Prepare(request)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Prepare error = %v, want %q", err, test.want)
			}
		})
	}
}

type governedFixtureOptions struct {
	contractArtifactPath string
	approvalPath         string
	authorityPath        string
	reviewPolicyPath     string
	oraclePath           string
	writeRoot            string
}

type governedFixture struct {
	root, workspacePath, contractSourcePath                                         string
	contractArtifactPath, approvalPath, authorityPath, reviewPolicyPath, oraclePath string
	gate                                                                            workflowgate.Result
}

func newGovernedFixture(t *testing.T, options governedFixtureOptions) governedFixture {
	t.Helper()
	if options.contractArtifactPath == "" {
		options.contractArtifactPath = ".ingen/contract/canonical.json"
	}
	if options.authorityPath == "" {
		options.authorityPath = ".ingen/governance/authority.json"
	}
	if options.reviewPolicyPath == "" {
		options.reviewPolicyPath = ".ingen/governance/review-policy.json"
	}
	if options.oraclePath == "" {
		options.oraclePath = ".ingen/artifacts/oracle/oracle.json"
	}
	if options.approvalPath == "" {
		options.approvalPath = ".ingen/governance/approval.json"
	}
	if options.writeRoot == "" {
		options.writeRoot = "src"
	}
	root := t.TempDir()
	for _, dir := range []string{".ingen/contract", ".ingen/governance", ".ingen/policy", ".ingen/artifacts/oracle", ".ingen/sessions/implementation", ".ingen/sessions/contract-author", ".ingen/sessions/oracle-writer", ".ingen/sessions/verifier", ".ingen/sessions/mutation-runner", "src", "docs"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{options.contractArtifactPath, options.authorityPath, options.reviewPolicyPath, options.oraclePath} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o700); err != nil {
			t.Fatal(err)
		}
	}

	contractSourcePath := ".ingen/contract/contract.json"
	contractDoc := contract.Document{Contract: map[string]any{
		"schema": contract.Schema, "id": "demo", "version": 1, "status": "draft",
		"interface":   map[string]any{"kind": "http"},
		"rules":       []any{map[string]any{"id": "health", "strength": "must", "subject": "GET /healthz", "expect": map[string]any{"status": 200}}},
		"unspecified": []any{},
	}}
	source, err := json.Marshal(map[string]any{"contract": contractDoc.Contract})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, contractSourcePath, source)
	sealedContract, err := contract.Seal(contractDoc)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, options.contractArtifactPath, sealedContract.CanonicalJSON)

	oraclePolicyDoc := policy.Document{Policy: map[string]any{
		"schema": policy.Schema, "id": "demo-oracle", "version": 1, "status": "draft", "purpose": "governed roleexec test", "enforcement": "declared-only",
		"filesystem": map[string]any{
			"read":  []any{map[string]any{"path": contractSourcePath, "reason": "contract input"}},
			"write": []any{map[string]any{"path": filepath.ToSlash(filepath.Dir(options.oraclePath)), "reason": "oracle output"}},
			"deny":  []any{map[string]any{"path": "src", "reason": "hide implementation"}},
		},
		"network": map[string]any{"mode": "disabled"}, "process": map[string]any{"subject_id": "demo", "can_invoke_subject": false},
	}}
	policyBytes, err := json.Marshal(map[string]any{"policy": oraclePolicyDoc.Policy})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, ".ingen/policy/oracle.json", policyBytes)
	writeTestFile(t, root, ".ingen/policy/subject.json", policyBytes)
	policyDoc, err := policy.LoadBytes(".ingen/policy/oracle.json", policyBytes)
	if err != nil {
		t.Fatal(err)
	}
	sealedPolicy, err := policy.Seal(policyDoc)
	if err != nil {
		t.Fatal(err)
	}

	authorityBytes, err := json.Marshal(map[string]any{"schema": "ingen.hammond-authority/v1", "id": "reviewers", "version": 1, "actors": map[string][]string{"reviewer": {"product-reviewer"}}})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, options.authorityPath, authorityBytes)
	governancePolicyBytes, err := json.Marshal(map[string]any{
		"schema": "ingen.hammond-review-policy/v1", "id": "review", "version": 1, "minimum_approvals": 1,
		"authority": map[string]any{"id": "reviewers", "version": 1, "schema": "ingen.hammond-authority/v1", "artifact": map[string]string{"uri": options.authorityPath, "sha256": testHash(authorityBytes)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, options.reviewPolicyPath, governancePolicyBytes)
	approvalPath := options.approvalPath
	approvalBytes, err := json.Marshal(map[string]any{
		"schema": "ingen.hammond-governance/v1", "record_id": "approved-demo",
		"contract": map[string]any{"project_id": "demo", "id": "demo", "version": 1, "schema": contract.Schema, "artifact": map[string]string{"uri": options.contractArtifactPath, "sha256": sealedContract.SHA256}},
		"policy":   map[string]any{"id": "review", "version": 1, "schema": "ingen.hammond-review-policy/v1", "artifact": map[string]string{"uri": options.reviewPolicyPath, "sha256": testHash(governancePolicyBytes)}},
		"state":    "approved", "events": []any{
			map[string]any{"id": "registered", "type": "registered", "actor": "owner", "at": "2026-10-01T00:00:00Z"},
			map[string]any{"id": "opened", "type": "review-opened", "actor": "owner", "review_cycle_id": "cycle-1", "at": "2026-10-01T00:01:00Z"},
			map[string]any{"id": "approved", "type": "approval-recorded", "actor": "reviewer", "role": "product-reviewer", "review_cycle_id": "cycle-1", "decision": "approve", "artifact_sha256": sealedContract.SHA256, "at": "2026-10-01T00:02:00Z"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, approvalPath, approvalBytes)

	workspacePath := ".ingen/workspace.yaml"
	workspaceYAML := fmt.Sprintf("sentinel_workspace:\n  schema: ingen.sentinel-workspace/v1\n  id: demo\n  version: 1\n  project_root: .\n  contract:\n    path: %s\n  implementation_roots: [src]\n  sorna:\n    oracle_policy: .ingen/policy/oracle.json\n    subject_policy: .ingen/policy/subject.json\n  delivery:\n    workflow: .ingen/nublar/workflow.yaml\n  roles:\n    - id: contract-author\n      kind: contract-author\n      workspace: .ingen/sessions/contract-author\n      read_roots: [.ingen/contract]\n      write_roots: [.ingen/contract]\n      deny_roots: [src]\n    - id: oracle-writer\n      kind: oracle-writer\n      workspace: .ingen/sessions/oracle-writer\n      read_roots: [%s]\n      write_roots: [.ingen/artifacts/oracle]\n      deny_roots: [src]\n    - id: implementation\n      kind: implementation\n      workspace: .ingen/sessions/implementation\n      read_roots: [%s, docs]\n      write_roots: [%s]\n      deny_roots: [.git, .ingen/artifacts/oracle]\n    - id: verifier\n      kind: verifier\n      workspace: .ingen/sessions/verifier\n      read_roots: [%s, src]\n      write_roots: [.ingen/artifacts/evidence]\n      deny_roots: [.git]\n    - id: mutation-runner\n      kind: mutation-runner\n      workspace: .ingen/sessions/mutation-runner\n      read_roots: [%s, .ingen/artifacts/evidence]\n      write_roots: [.ingen/artifacts/mutations]\n      deny_roots: [.git]\n", contractSourcePath, contractSourcePath, contractSourcePath, options.writeRoot, contractSourcePath, contractSourcePath)
	writeTestFile(t, root, workspacePath, []byte(workspaceYAML))

	frozenOracle, err := oracle.Generate(sealedContract, sealedPolicy.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	oracleBytes, err := oracle.CanonicalJSON(frozenOracle)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, options.oraclePath, oracleBytes)

	gate, err := workflowgate.Check(workflowgate.Request{Root: root, WorkspacePath: workspacePath, ApprovalPath: approvalPath, ReviewPolicyPath: options.reviewPolicyPath, Stage: workflowgate.StageImplementation, OraclePath: options.oraclePath})
	if err != nil {
		t.Fatalf("initial workflow gate: %v", err)
	}
	return governedFixture{root: root, workspacePath: workspacePath, contractSourcePath: contractSourcePath,
		contractArtifactPath: options.contractArtifactPath, approvalPath: approvalPath, authorityPath: options.authorityPath,
		reviewPolicyPath: options.reviewPolicyPath, oraclePath: options.oraclePath, gate: gate}
}

func (f governedFixture) executionRequest(executionID string) Request {
	protected := make([]string, 0, len(f.gate.ApprovalArtifacts))
	for _, artifact := range f.gate.ApprovalArtifacts {
		protected = append(protected, artifact.Path)
	}
	return Request{Root: f.root, WorkspacePath: f.workspacePath, RoleID: "implementation", ExecutionID: executionID,
		Command: []string{"/bin/echo", "governed-test"}, Governed: true, ApprovalPath: f.approvalPath,
		ReviewPolicyPath: f.reviewPolicyPath, OraclePath: f.oraclePath, ProtectedPaths: protected,
		ExpectedManifestSHA256: f.gate.WorkspaceManifestSHA256, ExpectedApprovalSHA256: f.gate.ApprovalSHA256,
		ExpectedReviewPolicySHA256: f.gate.ReviewPolicySHA256, ExpectedContractSHA256: f.gate.ContractSHA256,
		ExpectedContractSourceSHA256: f.gate.ContractSourceSHA256, ExpectedOraclePolicySHA256: f.gate.OraclePolicySHA256,
		ExpectedOraclePolicyFileSHA256: f.gate.OraclePolicyFileSHA256, ExpectedOracleSHA256: f.gate.OracleSHA256}
}

func (f governedFixture) prepareAndPersist(t *testing.T, request *Request) {
	t.Helper()
	prepared, err := Prepare(*request)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	request.ExpectedPolicySHA256 = prepared.Policy.SHA256
	request.ExpectedExecutableSHA256 = prepared.ExecutableSHA256
	request.PolicyPath = filepath.ToSlash(filepath.Join(".ingen", "artifacts", "role-executions", request.ExecutionID+".policy.json"))
	if err := PersistPolicy(request.Root, request.PolicyPath, prepared.Policy); err != nil {
		t.Fatalf("PersistPolicy: %v", err)
	}
}

func (f governedFixture) assertChildNotStarted(t *testing.T, executionID string) {
	t.Helper()
	for _, suffix := range []string{".claim", ".stdout", ".stderr", ".json"} {
		path := filepath.Join(f.root, ".ingen", "artifacts", "role-executions", executionID+suffix)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("role artifact %s exists after prelaunch rejection (stat error %v)", suffix, err)
		}
	}
}

func requireDarwinGovernedExecution(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("roleexec requires Darwin Seatbelt host enforcement")
	}
}

func writeTestFile(t *testing.T, root, relative string, contents []byte) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
}

func testHash(contents []byte) string {
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:])
}
func testDigest(contents string) string { return testHash([]byte(contents)) }
