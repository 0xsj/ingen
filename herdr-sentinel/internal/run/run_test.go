package run

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
      deny_roots: [subject, .git]
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

func TestNewAppendAndLoadReceipt(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("workspace.yaml", []byte(workspaceFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("sorna-result.json", []byte(`{"status":"passed"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	created := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	receipt, err := New("workspace.yaml", created)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "created" || len(receipt.Events) != 1 || receipt.Workspace.File.SHA256 == "" {
		t.Fatalf("receipt = %+v, want created receipt with workspace hash", receipt)
	}

	if err := receipt.AddFileArtifact("sorna-result", "verifier", "sorna-ci-result", "sorna-result.json"); err != nil {
		t.Fatal(err)
	}
	if err := receipt.AppendEvent(Event{
		Type:        "artifact-produced",
		At:          created.Add(time.Second).Format(time.RFC3339Nano),
		Role:        "verifier",
		Workspace:   ".sentinel/verifier",
		ArtifactIDs: []string{"sorna-result"},
		Outcome:     "produced",
	}); err != nil {
		t.Fatal(err)
	}
	if err := receipt.SetStatus("completed", created.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := SaveFile("receipt.json", receipt); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadFile("receipt.json")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RunID != receipt.RunID || loaded.Status != "completed" || len(loaded.Events) != 2 || len(loaded.Artifacts) != 1 {
		t.Fatalf("loaded = %+v, want persisted lifecycle receipt", loaded)
	}
}

func TestAppendEventRejectsUnknownArtifactWithoutMutation(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("workspace.yaml", []byte(workspaceFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	receipt, err := New("workspace.yaml", time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	err = receipt.AppendEvent(Event{
		Type:        "artifact-produced",
		At:          "2026-01-02T03:04:06Z",
		ArtifactIDs: []string{"missing"},
	})
	if err == nil || !strings.Contains(err.Error(), "unknown artifact") {
		t.Fatalf("AppendEvent() = %v, want unknown artifact error", err)
	}
	if len(receipt.Events) != 1 || receipt.UpdatedAt != receipt.CreatedAt {
		t.Fatalf("receipt mutated after rejected event: %+v", receipt)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("workspace.yaml", []byte(workspaceFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	receipt, err := New("workspace.yaml", time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("receipt.json")
	if err := SaveFile(path, receipt); err != nil {
		t.Fatal(err)
	}
	receiptData, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	modified := strings.Replace(string(receiptData), `"status": "created",`, "\"status\": \"created\",\n  \"future\": true,", 1)
	if err := os.WriteFile(path, []byte(modified), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("LoadFile() = %v, want unknown field error", err)
	}
}
