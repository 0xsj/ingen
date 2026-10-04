package workflowgate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/sorna/contract"
	"ingen/sorna/oracle"
	"ingen/sorna/policy"
)

func TestCheckRequiresApprovedContractAndFrozenOracleForStages(t *testing.T) {
	fixture := newGateFixture(t, false)
	result, err := Check(fixture.request(StageOracle))
	if err != nil {
		t.Fatal(err)
	}
	if result.ContractSHA256 != fixture.sealedContract.SHA256 || result.OraclePolicySHA256 != fixture.sealedPolicy.SHA256 || result.Enforcement != "declaration-only" || result.Assurance != "unverified" {
		t.Fatalf("oracle stage result = %+v", result)
	}
	if _, err := Check(fixture.request(StageImplementation)); err == nil || !strings.Contains(err.Error(), "frozen oracle") {
		t.Fatalf("implementation without oracle error = %v", err)
	}
	fixture.writeOracle(t, fixture.sealedContract, fixture.sealedPolicy.SHA256)
	for _, stage := range []Stage{StageImplementation, StageVerification} {
		result, err := Check(fixture.request(stage))
		if err != nil {
			t.Fatalf("Check(%s): %v", stage, err)
		}
		if result.OracleSHA256 == "" {
			t.Fatalf("Check(%s) omitted oracle digest", stage)
		}
	}
}

func TestCheckRejectsStaleContractPolicyOracleAndSidecar(t *testing.T) {
	t.Run("source changed after approval", func(t *testing.T) {
		fixture := newGateFixture(t, false)
		path := filepath.Join(fixture.root, ".ingen/contract/contract.json")
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		contents = []byte(strings.Replace(string(contents), `"version":1`, `"version":2`, 1))
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Check(fixture.request(StageOracle)); err == nil || !strings.Contains(err.Error(), "approval") {
			t.Fatalf("contract source drift error = %v", err)
		}
	})
	t.Run("oracle policy changed after freeze", func(t *testing.T) {
		fixture := newGateFixture(t, false)
		fixture.writeOracle(t, fixture.sealedContract, fixture.sealedPolicy.SHA256)
		path := filepath.Join(fixture.root, ".ingen/policy/oracle.json")
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		contents = []byte(strings.Replace(string(contents), `"purpose":"oracle gate test"`, `"purpose":"changed policy"`, 1))
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Check(fixture.request(StageImplementation)); err == nil || !strings.Contains(err.Error(), "current oracle policy digest") {
			t.Fatalf("oracle policy drift error = %v", err)
		}
	})
	t.Run("wrong oracle contract version", func(t *testing.T) {
		fixture := newGateFixture(t, false)
		frozen := fixture.makeOracle(fixture.sealedContract, fixture.sealedPolicy.SHA256)
		frozen.Contract.Version++
		fixture.writeOracleArtifact(t, frozen)
		if _, err := Check(fixture.request(StageVerification)); err == nil || !strings.Contains(err.Error(), "does not match the currently approved contract") {
			t.Fatalf("oracle version mismatch error = %v", err)
		}
	})
	t.Run("mismatched sidecar", func(t *testing.T) {
		fixture := newGateFixture(t, false)
		fixture.writeOracle(t, fixture.sealedContract, fixture.sealedPolicy.SHA256)
		if err := os.WriteFile(filepath.Join(fixture.root, ".ingen/artifacts/oracle/hash.txt"), []byte(strings.Repeat("0", 64)+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Check(fixture.request(StageImplementation)); err == nil || !strings.Contains(err.Error(), "hash sidecar") {
			t.Fatalf("oracle sidecar mismatch error = %v", err)
		}
	})
}

func TestCheckResolvesFixtureRelativeToContractUnderRoot(t *testing.T) {
	fixture := newGateFixture(t, true)
	if _, err := Check(fixture.request(StageOracle)); err != nil {
		t.Fatalf("valid parent-relative project fixture: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "secret-fixture.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	fixturePath := filepath.Join(fixture.root, ".ingen/fixtures/fixture.txt")
	if err := os.Remove(fixturePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, fixturePath); err != nil {
		t.Fatal(err)
	}
	if _, err := Check(fixture.request(StageOracle)); err == nil || !strings.Contains(err.Error(), "fixture") {
		t.Fatalf("fixture symlink escape error = %v", err)
	}
}

func TestCheckAcceptsAlreadySealedContractSource(t *testing.T) {
	fixture := newGateFixture(t, false)
	contractPath := filepath.Join(fixture.root, ".ingen/contract/contract.json")
	if err := os.WriteFile(contractPath, fixture.sealedContract.CanonicalJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Check(fixture.request(StageOracle))
	if err != nil {
		t.Fatalf("Check with sealed contract source: %v", err)
	}
	if result.ContractSHA256 != fixture.sealedContract.SHA256 {
		t.Fatalf("contract digest = %q, want %q", result.ContractSHA256, fixture.sealedContract.SHA256)
	}
}

type gateFixture struct {
	root           string
	sealedContract contract.Sealed
	sealedPolicy   policy.Sealed
	contractRef    string
	policyRef      string
	authorityRef   string
	approvalRef    string
	oracleRef      string
}

func newGateFixture(t *testing.T, withFixture bool) gateFixture {
	t.Helper()
	root := t.TempDir()
	for _, path := range []string{
		".ingen/contract", ".ingen/governance", ".ingen/policy", ".ingen/fixtures", ".ingen/artifacts/oracle",
	} {
		if err := os.MkdirAll(filepath.Join(root, path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	contractSource := map[string]any{
		"schema": contract.Schema, "id": "demo", "version": 1, "status": "draft",
		"interface":   map[string]any{"kind": "http"},
		"rules":       []any{map[string]any{"id": "health", "strength": "must", "subject": "GET /healthz", "expect": map[string]any{"status": 200}}},
		"unspecified": []any{},
	}
	if withFixture {
		contractSource["fixtures"] = []any{map[string]any{"path": "../fixtures/fixture.txt", "purpose": "public test fixture"}}
		if err := os.WriteFile(filepath.Join(root, ".ingen/fixtures/fixture.txt"), []byte("fixture-v1"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	contractDocumentBytes, err := json.Marshal(map[string]any{"contract": contractSource})
	if err != nil {
		t.Fatal(err)
	}
	contractSourcePath := filepath.Join(root, ".ingen/contract/contract.json")
	if err := os.WriteFile(contractSourcePath, contractDocumentBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	loadedContract, err := contract.LoadFile(contractSourcePath)
	if err != nil {
		t.Fatal(err)
	}
	sealedContract, err := contract.SealAt(loadedContract, filepath.Dir(contractSourcePath))
	if err != nil {
		t.Fatal(err)
	}
	contractRef := ".ingen/contract/canonical.json"
	if err := os.WriteFile(filepath.Join(root, contractRef), sealedContract.CanonicalJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	contractDigest := fixtureHash(sealedContract.CanonicalJSON)

	policyDoc := map[string]any{
		"schema": policy.Schema, "id": "demo-oracle", "version": 1, "status": "draft", "purpose": "oracle gate test", "enforcement": "declared-only",
		"filesystem": map[string]any{
			"read":  []any{map[string]any{"path": ".ingen/contract/contract.json", "reason": "contract input"}},
			"write": []any{map[string]any{"path": ".ingen/artifacts/oracle", "reason": "oracle output"}},
			"deny":  []any{map[string]any{"path": "src", "reason": "hide implementation"}},
		},
		"network": map[string]any{"mode": "disabled"},
		"process": map[string]any{"subject_id": "demo", "can_invoke_subject": false},
	}
	policyBytes, err := json.Marshal(map[string]any{"policy": policyDoc})
	if err != nil {
		t.Fatal(err)
	}
	policyPath := filepath.Join(root, ".ingen/policy/oracle.json")
	if err := os.WriteFile(policyPath, policyBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".ingen/policy/subject.json"), policyBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	policyDocument, err := policy.LoadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	sealedPolicy, err := policy.Seal(policyDocument)
	if err != nil {
		t.Fatal(err)
	}

	workspaceYaml := `sentinel_workspace:
  schema: ingen.sentinel-workspace/v1
  id: demo
  version: 1
  project_root: .
  contract:
    path: .ingen/contract/contract.json
  implementation_roots: [src]
  sorna:
    oracle_policy: .ingen/policy/oracle.json
    subject_policy: .ingen/policy/subject.json
  delivery:
    workflow: .ingen/nublar/workflow.yaml
  roles:
    - id: contract-author
      kind: contract-author
      workspace: .ingen/sessions/contract-author
      read_roots: [.ingen/brief.md, docs]
      write_roots: [.ingen/contract]
      deny_roots: [src, .git]
    - id: governance-reviewer
      kind: contract-author
      workspace: .ingen/sessions/governance-reviewer
      read_roots: [.ingen/contract]
      write_roots: [.ingen/governance]
      deny_roots: [src, .git]
    - id: oracle-writer
      kind: oracle-writer
      workspace: .ingen/sessions/oracle-writer
      read_roots: [.ingen/contract/contract.json, .ingen/fixtures]
      write_roots: [.ingen/artifacts/oracle]
      deny_roots: [src, .git]
    - id: implementation
      kind: implementation
      workspace: .ingen/sessions/implementation
      read_roots: [.ingen/contract/contract.json, docs]
      write_roots: [src]
      deny_roots: [.ingen/artifacts/oracle, .git]
    - id: verifier
      kind: verifier
      workspace: .ingen/sessions/verifier
      read_roots: [.ingen/contract/contract.json, .ingen/artifacts/oracle, src]
      write_roots: [.ingen/artifacts/evidence]
      deny_roots: [.git]
    - id: mutation-runner
      kind: mutation-runner
      workspace: .ingen/sessions/mutation-runner
      read_roots: [.ingen/contract/contract.json, .ingen/artifacts/oracle, .ingen/artifacts/evidence]
      write_roots: [.ingen/artifacts/mutations]
      deny_roots: [.git]
`
	workspacePath := ".ingen/workspace.yaml"
	if err := os.WriteFile(filepath.Join(root, workspacePath), []byte(workspaceYaml), 0o600); err != nil {
		t.Fatal(err)
	}

	authorityPath := ".ingen/governance/authority.json"
	authorityBytes, _ := json.Marshal(map[string]any{"schema": "ingen.hammond-authority/v1", "id": "reviewers", "version": 1, "actors": map[string][]string{"reviewer": {"product-reviewer"}}})
	if err := os.WriteFile(filepath.Join(root, authorityPath), authorityBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	authorityDigest := fixtureHash(authorityBytes)
	policyRef := ".ingen/governance/review-policy.json"
	governancePolicyBytes, _ := json.Marshal(map[string]any{
		"schema": "ingen.hammond-review-policy/v1", "id": "review", "version": 1, "minimum_approvals": 1,
		"authority": map[string]any{"id": "reviewers", "version": 1, "schema": "ingen.hammond-authority/v1", "artifact": map[string]string{"uri": authorityPath, "sha256": authorityDigest}},
	})
	if err := os.WriteFile(filepath.Join(root, policyRef), governancePolicyBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	governancePolicyDigest := fixtureHash(governancePolicyBytes)
	approvalPath := ".ingen/governance/approval.json"
	approval := map[string]any{
		"schema": "ingen.hammond-governance/v1", "record_id": "approved-demo",
		"contract": map[string]any{"project_id": "demo", "id": "demo", "version": 1, "schema": contract.Schema, "artifact": map[string]string{"uri": contractRef, "sha256": contractDigest}},
		"policy":   map[string]any{"id": "review", "version": 1, "schema": "ingen.hammond-review-policy/v1", "artifact": map[string]string{"uri": policyRef, "sha256": governancePolicyDigest}},
		"state":    "approved", "events": []any{
			map[string]any{"id": "registered", "type": "registered", "actor": "owner", "at": "2026-10-01T00:00:00Z"},
			map[string]any{"id": "review-opened", "type": "review-opened", "actor": "owner", "review_cycle_id": "cycle-1", "at": "2026-10-01T00:01:00Z"},
			map[string]any{"id": "approval", "type": "approval-recorded", "actor": "reviewer", "role": "product-reviewer", "review_cycle_id": "cycle-1", "decision": "approve", "artifact_sha256": contractDigest, "at": "2026-10-01T00:02:00Z"},
		},
	}
	approvalBytes, _ := json.Marshal(approval)
	if err := os.WriteFile(filepath.Join(root, approvalPath), approvalBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	return gateFixture{root: root, sealedContract: sealedContract, sealedPolicy: sealedPolicy, contractRef: contractRef, policyRef: policyRef, authorityRef: authorityPath, approvalRef: approvalPath, oracleRef: ".ingen/artifacts/oracle/oracle.json"}
}

func (fixture gateFixture) request(stage Stage) Request {
	return Request{Root: fixture.root, WorkspacePath: ".ingen/workspace.yaml", ApprovalPath: fixture.approvalRef, ReviewPolicyPath: fixture.policyRef, Stage: stage, OraclePath: fixture.oracleRef}
}

func (fixture gateFixture) makeOracle(sealed contract.Sealed, policyHash string) oracle.Artifact {
	return oracle.Artifact{
		Schema: oracle.Schema,
		Status: "frozen",
		Contract: oracle.ContractReference{
			ID: "demo", Version: 1, SHA256: sealed.SHA256,
		},
		PolicySHA256: policyHash,
		Cases:        []oracle.Case{{CaseID: "case-0001", RuleID: "health", Strength: "must", Subject: "GET /healthz"}},
	}
}

func (fixture gateFixture) writeOracle(t *testing.T, sealed contract.Sealed, policyHash string) {
	t.Helper()
	fixture.writeOracleArtifact(t, fixture.makeOracle(sealed, policyHash))
}

func (fixture gateFixture) writeOracleArtifact(t *testing.T, artifact oracle.Artifact) {
	t.Helper()
	data, err := oracle.CanonicalJSON(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture.root, fixture.oracleRef), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func fixtureHash(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
