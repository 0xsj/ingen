package run

import (
	"bytes"
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
	snapshot, snapshotBytes, err := LoadFileSnapshot("receipt.json")
	if err != nil {
		t.Fatal(err)
	}
	savedBytes, err := os.ReadFile("receipt.json")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.RunID != loaded.RunID || !bytes.Equal(snapshotBytes, savedBytes) {
		t.Fatalf("snapshot = %q, %+v; want exact validated receipt bytes", snapshotBytes, snapshot)
	}
}

func TestNewRejectsWorkspaceSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsidePath := filepath.Join(outside, "workspace.yaml")
	if err := os.WriteFile(outsidePath, []byte(workspaceFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsidePath, filepath.Join(root, "workspace.yaml")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	if _, err := New("workspace.yaml", time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)); err == nil || !strings.Contains(err.Error(), "escapes root") {
		t.Fatalf("New() = %v, want workspace root-containment rejection", err)
	}
}

func TestRegisterFileArtifactIsIdempotentButRejectsConflicts(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("workspace.yaml", []byte(workspaceFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("artifact.json", []byte("result"), 0o644); err != nil {
		t.Fatal(err)
	}
	receipt, err := New("workspace.yaml", time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	changed, err := receipt.RegisterFileArtifact("result", "verifier", "sorna-run", "artifact.json")
	if err != nil || !changed {
		t.Fatalf("first RegisterFileArtifact() = %v, %v; want new artifact", changed, err)
	}
	changed, err = receipt.RegisterFileArtifact("result", "verifier", "sorna-run", "artifact.json")
	if err != nil || changed {
		t.Fatalf("same RegisterFileArtifact() = %v, %v; want idempotent no-op", changed, err)
	}
	if len(receipt.Artifacts) != 1 {
		t.Fatalf("artifacts after retry = %d, want one", len(receipt.Artifacts))
	}
	if err := os.WriteFile("artifact.json", []byte("different"), 0o644); err != nil {
		t.Fatal(err)
	}
	if changed, err := receipt.RegisterFileArtifact("result", "verifier", "sorna-run", "artifact.json"); err == nil || changed || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("conflicting RegisterFileArtifact() = %v, %v; want conflict", changed, err)
	}
	if len(receipt.Artifacts) != 1 {
		t.Fatalf("artifacts after conflict = %d, want one", len(receipt.Artifacts))
	}
}

func TestRegisterFileArtifactRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "workspace.yaml"), []byte(workspaceFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	outsidePath := filepath.Join(outside, "artifact.json")
	if err := os.WriteFile(outsidePath, []byte("result"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsidePath, filepath.Join(root, "artifact.json")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	receipt, err := New("workspace.yaml", time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	changed, err := receipt.RegisterFileArtifact("result", "verifier", "sorna-run", "artifact.json")
	if err == nil || changed || !strings.Contains(err.Error(), "escapes root") {
		t.Fatalf("RegisterFileArtifact() = %v, %v; want root-containment rejection", changed, err)
	}
	if len(receipt.Artifacts) != 0 {
		t.Fatalf("artifacts after symlink rejection = %d, want none", len(receipt.Artifacts))
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

func TestSetStatusRejectsTerminalRegression(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("workspace.yaml", []byte(workspaceFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	receipt, err := New("workspace.yaml", time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if err := receipt.SetStatus("completed", time.Date(2026, time.January, 2, 3, 4, 6, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if err := receipt.SetStatus("running", time.Date(2026, time.January, 2, 3, 4, 7, 0, time.UTC)); err == nil || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("terminal regression = %v, want rejection", err)
	}
	if receipt.Status != "completed" || receipt.UpdatedAt != "2026-01-02T03:04:06Z" {
		t.Fatalf("receipt mutated after terminal regression: %+v", receipt)
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
