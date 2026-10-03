package project

import (
	"os"
	"path/filepath"
	"testing"

	"ingen/herdr-sentinel/internal/capability"
	"ingen/herdr-sentinel/internal/workspace"
	nublarworkflow "ingen/nublar/workflow"
)

func TestInitializeCreatesValidWorkspaceScaffold(t *testing.T) {
	root := t.TempDir()
	result, err := Initialize(Options{Root: root, ID: "fresh-project"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Workspace != WorkspacePath {
		t.Fatalf("workspace = %q, want %q", result.Workspace, WorkspacePath)
	}
	if _, err := workspace.LoadFile(filepath.Join(root, WorkspacePath)); err != nil {
		t.Fatalf("generated workspace is invalid: %v", err)
	}
	if _, err := capability.FromFileUnderRoot(root, WorkspacePath); err != nil {
		t.Fatalf("generated capability plan is invalid: %v", err)
	}
	delivery, err := nublarworkflow.LoadFile(filepath.Join(root, ".ingen/nublar/workflow.yaml"))
	if err != nil {
		t.Fatalf("generated Nublar workflow is invalid: %v", err)
	}
	if len(delivery.Checks) != 2 || delivery.Checks[0].IsRequired() || delivery.Checks[1].Result != ".ingen/artifacts/evidence-ci-result.json" {
		t.Fatalf("generated Nublar checks = %+v, want optional Sentinel and required Sorna CI result", delivery.Checks)
	}
	for _, path := range []string{
		".ingen/brief.md",
		".ingen/contract/spec.malc",
		".ingen/policy/oracle.yaml",
		".ingen/policy/subject.yaml",
		".ingen/nublar/workflow.yaml",
		"src",
		"docs",
		".ingen/sessions/contract-author",
	} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Errorf("scaffold path %s: %v", path, err)
		}
	}
}

func TestInitializeNeverOverwritesWorkspace(t *testing.T) {
	root := t.TempDir()
	if _, err := Initialize(Options{Root: root, ID: "fresh-project"}); err != nil {
		t.Fatal(err)
	}
	workspacePath := filepath.Join(root, WorkspacePath)
	before, err := os.ReadFile(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Initialize(Options{Root: root, ID: "changed-project"}); err == nil {
		t.Fatal("second initialization succeeded; want overwrite refusal")
	}
	after, err := os.ReadFile(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("workspace changed after overwrite refusal")
	}
}

func TestInitializeRejectsScaffoldSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, ".ingen")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Initialize(Options{Root: root, ID: "fresh-project"}); err == nil {
		t.Fatal("initialization followed .ingen symlink outside project root")
	}
	if _, err := os.Stat(filepath.Join(outside, "workspace.yaml")); !os.IsNotExist(err) {
		t.Fatalf("outside workspace stat error = %v, want file absent", err)
	}
}

func TestInitializeRefusesDanglingScaffoldFileSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".ingen"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "missing-target"), filepath.Join(root, ".ingen", "brief.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Initialize(Options{Root: root, ID: "fresh-project"}); err == nil {
		t.Fatal("initialization accepted a dangling symlink at a scaffold file path")
	}
}
