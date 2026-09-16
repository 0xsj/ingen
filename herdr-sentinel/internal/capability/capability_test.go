package capability

import (
	"os"
	"strings"
	"testing"
)

const workspaceFixture = `sentinel_workspace:
  schema: ingen.sentinel-workspace/v1
  id: webhook-validation
  version: 1
  project_root: .
  contract:
    path: contract.yaml
  implementation_roots:
    - subject
    - defects
  sorna:
    oracle_policy: oracle-policy.yaml
    subject_policy: subject-policy.yaml
  delivery:
    workflow: workflow.yaml
  roles:
    - id: contract-author
      kind: contract-author
      workspace: .sentinel/contract
      read_roots: []
      write_roots: [contract.yaml]
      deny_roots: [.git]
    - id: oracle-writer
      kind: oracle-writer
      workspace: .sentinel/oracle
      read_roots: [contract.yaml]
      write_roots: [.artifacts/oracle]
      deny_roots: [subject, defects, .git]
    - id: backend-implementer
      kind: implementation
      workspace: .sentinel/implementation
      read_roots: [contract.yaml]
      write_roots: [subject]
      deny_roots: [.artifacts/oracle, .git]
    - id: verifier
      kind: verifier
      workspace: .sentinel/verifier
      read_roots: [contract.yaml, .artifacts/oracle, subject]
      write_roots: [.artifacts/run]
      deny_roots: [.git]
    - id: mutation-runner
      kind: mutation-runner
      workspace: .sentinel/mutations
      read_roots: [.artifacts/oracle, subject]
      write_roots: [.artifacts/mutations]
      deny_roots: [contract.yaml, .git]
`

func TestFromFilePreservesManifestAndRoleCapabilities(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkspaceFixture(t)

	plan, err := FromFile("workspace.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Workspace.ID != "webhook-validation" || len(plan.Roles) != 5 || len(plan.ImplementationRoots) != 2 {
		t.Fatalf("plan = %+v, want five roles and two implementation roots", plan)
	}
	if plan.Enforcement != "declaration-only" || plan.Assurance != "unverified" {
		t.Fatalf("plan enforcement = %q/%q, want declaration-only/unverified", plan.Enforcement, plan.Assurance)
	}
	if plan.Workspace.Manifest.SHA256 == "" {
		t.Fatal("plan did not preserve a manifest hash")
	}
}

func TestValidateRejectsAllowedImplementationRootForOracle(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkspaceFixture(t)
	plan, err := FromFile("workspace.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for index := range plan.Roles {
		if plan.Roles[index].Kind == "oracle-writer" {
			plan.Roles[index].ReadRoots = append(plan.Roles[index].ReadRoots, "subject")
		}
	}
	if err := plan.Validate(); err == nil || !strings.Contains(err.Error(), "allows root") {
		t.Fatalf("Validate() = %v, want oracle implementation access error", err)
	}
}

func TestValidateRequiresOracleDenyForEveryImplementationRoot(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkspaceFixture(t)
	plan, err := FromFile("workspace.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for index := range plan.Roles {
		if plan.Roles[index].Kind == "oracle-writer" {
			plan.Roles[index].DenyRoots = []string{"subject", ".git"}
		}
	}
	if err := plan.Validate(); err == nil || !strings.Contains(err.Error(), "must deny implementation root") {
		t.Fatalf("Validate() = %v, want missing implementation deny error", err)
	}
}

func TestSaveAndLoadPlan(t *testing.T) {
	t.Chdir(t.TempDir())
	writeWorkspaceFixture(t)
	plan, err := FromFile("workspace.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveFile("plan.json", plan); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadFile("plan.json")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Workspace.Manifest != plan.Workspace.Manifest || loaded.Roles[1].Kind != "oracle-writer" {
		t.Fatalf("loaded = %+v, want persisted capability plan", loaded)
	}
}

func writeWorkspaceFixture(t *testing.T) {
	t.Helper()
	for path, contents := range map[string]string{
		"workspace.yaml":      workspaceFixture,
		"oracle-policy.yaml":  "policy: oracle\n",
		"subject-policy.yaml": "policy: subject\n",
	} {
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
