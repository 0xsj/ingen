package workspace

import (
	"strings"
	"testing"
)

const validWorkspace = `sentinel_workspace:
  schema: ingen.sentinel-workspace/v1
  id: webhook-validation
  version: 1
  project_root: .
  contract:
    path: examples/webhook-validation-lab/contract/contract.yaml
  implementation_roots:
    - examples/webhook-validation-lab/subject
    - examples/webhook-validation-lab/defects
  sorna:
    oracle_policy: examples/webhook-validation-lab/policy/isolation.yaml
    subject_policy: examples/webhook-validation-lab/policy/subject.yaml
  delivery:
    workflow: nublar/workflows/webhook-validation.yaml
  roles:
    - id: contract-author
      kind: contract-author
      workspace: .sentinel/webhook-validation/contract
      read_roots: []
      write_roots:
        - examples/webhook-validation-lab/contract/contract.yaml
      deny_roots: [.git]
    - id: oracle-writer
      kind: oracle-writer
      workspace: .sentinel/webhook-validation/oracle
      read_roots:
        - examples/webhook-validation-lab/contract/contract.yaml
      write_roots:
        - .artifacts/webhook-validation-oracle
      deny_roots:
        - examples/webhook-validation-lab/subject
        - examples/webhook-validation-lab/defects
        - .git
    - id: backend-implementer
      kind: implementation
      workspace: .sentinel/webhook-validation/implementation
      read_roots:
        - examples/webhook-validation-lab/contract/contract.yaml
      write_roots:
        - examples/webhook-validation-lab/subject
      deny_roots:
        - .artifacts/webhook-validation-oracle
        - .git
    - id: verifier
      kind: verifier
      workspace: .sentinel/webhook-validation/verifier
      read_roots:
        - examples/webhook-validation-lab/contract/contract.yaml
        - .artifacts/webhook-validation-oracle
        - examples/webhook-validation-lab/subject
      write_roots:
        - .artifacts/webhook-validation-run
      deny_roots: [.git]
    - id: mutation-runner
      kind: mutation-runner
      workspace: .sentinel/webhook-validation/mutations
      read_roots:
        - .artifacts/webhook-validation-oracle
        - .artifacts/webhook-validation-subject
      write_roots:
        - .artifacts/webhook-validation-go-campaign
      deny_roots:
        - examples/webhook-validation-lab/contract
        - .git
`

func TestLoadBytesAcceptsWebhookWorkspace(t *testing.T) {
	loaded, err := LoadBytes("workspace.yaml", []byte(validWorkspace))
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != "webhook-validation" || len(loaded.Roles) != 5 {
		t.Fatalf("workspace = %+v, want webhook workspace with five roles", loaded)
	}
}

func TestValidateRequiresOracleWriterToDenyImplementations(t *testing.T) {
	contents := strings.Replace(validWorkspace, "        - examples/webhook-validation-lab/defects\n        - .git", "        - .git", 1)
	_, err := LoadBytes("workspace.yaml", []byte(contents))
	if err == nil || !strings.Contains(err.Error(), "oracle-writer must deny implementation root") {
		t.Fatalf("LoadBytes() = %v, want missing implementation deny error", err)
	}
}

func TestValidateRejectsTraversalAndDuplicateRoleWorkspace(t *testing.T) {
	contents := strings.Replace(validWorkspace, "  project_root: .", "  project_root: ../outside", 1)
	contents = strings.Replace(contents, "workspace: .sentinel/webhook-validation/verifier", "workspace: .sentinel/webhook-validation/oracle", 1)
	_, err := LoadBytes("workspace.yaml", []byte(contents))
	if err == nil || !strings.Contains(err.Error(), "must stay inside the project root") || !strings.Contains(err.Error(), "workspace duplicates") {
		t.Fatalf("LoadBytes() = %v, want traversal and duplicate workspace errors", err)
	}
}
