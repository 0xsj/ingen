package session

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"ingen/herdr-sentinel/internal/project"
	"ingen/herdr-sentinel/internal/run"
)

func TestSpawnRecordsSessionAndHashedOutputs(t *testing.T) {
	root, receiptPath := newProjectReceipt(t)
	record, err := Spawn(Request{
		Root:          root,
		WorkspacePath: ".ingen/workspace.yaml",
		ReceiptPath:   receiptPath,
		RoleID:        "contract-author",
		Command:       []string{"/bin/sh", "-c", "printf '%s:%s' \"$INGEN_SENTINEL_ROLE_ID\" \"$INGEN_SENTINEL_CAPABILITY_ENFORCEMENT\""},
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != "completed" || record.RoleID != "contract-author" || record.Enforcement != "declaration-only" {
		t.Fatalf("record = %+v, want completed declaration-only session", record)
	}
	stdout, err := os.ReadFile(filepath.Join(root, record.Stdout))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(stdout); got != "contract-author:declaration-only" {
		t.Fatalf("session stdout = %q, want identity and enforcement", got)
	}
	receipt, err := run.LoadFile(filepath.Join(root, receiptPath))
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "running" || len(receipt.Events) != 3 || len(receipt.Artifacts) != 3 {
		t.Fatalf("receipt status/events/artifacts = %s/%d/%d, want running/3/3", receipt.Status, len(receipt.Events), len(receipt.Artifacts))
	}
	if receipt.Events[1].SessionID != record.SessionID || receipt.Events[2].SessionID != record.SessionID {
		t.Fatalf("receipt session IDs = %+v, want %s", receipt.Events, record.SessionID)
	}
}

func TestSpawnPreservesFailedSessionAndExitCode(t *testing.T) {
	root, receiptPath := newProjectReceipt(t)
	record, err := Spawn(Request{
		Root:          root,
		WorkspacePath: ".ingen/workspace.yaml",
		ReceiptPath:   receiptPath,
		RoleID:        "implementation",
		Command:       []string{"/bin/sh", "-c", "printf failure >&2; exit 7"},
	})
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("spawn error = %v, want child exit code 7", err)
	}
	if record.Status != "failed" || record.ExitCode != 7 {
		t.Fatalf("failed record = %+v, want exit code 7", record)
	}
	receipt, err := run.LoadFile(filepath.Join(root, receiptPath))
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "failed" || receipt.Events[len(receipt.Events)-1].Status != "failed" {
		t.Fatalf("failed receipt = %+v, want failed terminal state", receipt)
	}
}

func TestSpawnRefusesExistingOutputAndLeavesReceiptUnchanged(t *testing.T) {
	root, receiptPath := newProjectReceipt(t)
	protectedPath := filepath.Join(root, ".ingen", "contract", "spec.malc")
	before, err := os.ReadFile(protectedPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Spawn(Request{
		Root:          root,
		WorkspacePath: ".ingen/workspace.yaml",
		ReceiptPath:   receiptPath,
		RoleID:        "contract-author",
		OutputPath:    ".ingen/contract/spec.malc",
		Command:       []string{"/bin/sh", "-c", "exit 0"},
	})
	if err == nil {
		t.Fatal("spawn accepted an existing session output path")
	}
	after, err := os.ReadFile(protectedPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("existing file changed from %q to %q", before, after)
	}
	receipt, err := run.LoadFile(filepath.Join(root, receiptPath))
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "created" || len(receipt.Events) != 1 {
		t.Fatalf("receipt changed after rejected spawn: status=%s events=%d", receipt.Status, len(receipt.Events))
	}
}

func TestSpawnRejectsSessionArtifactSymlinkEscape(t *testing.T) {
	root, receiptPath := newProjectReceipt(t)
	outside := t.TempDir()
	sessionsPath := filepath.Join(root, ".ingen", "artifacts", "sessions")
	if err := os.RemoveAll(sessionsPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, sessionsPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, err := Spawn(Request{
		Root:          root,
		WorkspacePath: ".ingen/workspace.yaml",
		ReceiptPath:   receiptPath,
		RoleID:        "contract-author",
		Command:       []string{"/bin/sh", "-c", "exit 0"},
	})
	if err == nil {
		t.Fatal("spawn accepted session artifact directory symlink outside project root")
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("outside directory received session artifacts: %+v", entries)
	}
}

func TestSpawnRejectsCollidingOutputPathsBeforeLaunch(t *testing.T) {
	root, receiptPath := newProjectReceipt(t)
	receiptFile := filepath.Join(root, receiptPath)
	beforeReceipt, err := os.ReadFile(receiptFile)
	if err != nil {
		t.Fatal(err)
	}
	sessionsDir := filepath.Join(root, ".ingen", "artifacts", "sessions")
	beforeArtifacts, err := os.ReadDir(sessionsDir)
	if err != nil {
		t.Fatal(err)
	}
	markerPath := filepath.Join(root, ".ingen", "sessions", "contract-author", "child-ran")
	_, err = Spawn(Request{
		Root:          root,
		WorkspacePath: ".ingen/workspace.yaml",
		ReceiptPath:   receiptPath,
		RoleID:        "contract-author",
		OutputPath:    ".ingen/artifacts/sessions/colliding-output",
		StdoutPath:    ".ingen/artifacts/sessions/colliding-output",
		Command:       []string{"/bin/sh", "-c", "touch child-ran"},
	})
	if err == nil {
		t.Fatal("spawn accepted colliding record and stdout paths")
	}
	assertReceiptBytesUnchanged(t, receiptFile, beforeReceipt)
	afterArtifacts, err := os.ReadDir(sessionsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterArtifacts) != len(beforeArtifacts) {
		t.Fatalf("session artifact directory changed from %v to %v", names(beforeArtifacts), names(afterArtifacts))
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("child marker stat error = %v, want child not launched", err)
	}
}

func TestSpawnTerminalReceiptRejectsAndCleansReservations(t *testing.T) {
	root, receiptPath := newProjectReceipt(t)
	receiptFile := filepath.Join(root, receiptPath)
	receipt, err := run.LoadFile(receiptFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := receipt.SetStatus("completed", time.Date(2026, 9, 17, 10, 1, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if err := run.SaveFile(receiptFile, receipt); err != nil {
		t.Fatal(err)
	}
	beforeReceipt, err := os.ReadFile(receiptFile)
	if err != nil {
		t.Fatal(err)
	}
	sessionsDir := filepath.Join(root, ".ingen", "artifacts", "sessions")
	beforeArtifacts, err := os.ReadDir(sessionsDir)
	if err != nil {
		t.Fatal(err)
	}
	markerPath := filepath.Join(root, ".ingen", "sessions", "contract-author", "child-ran")
	_, err = Spawn(Request{
		Root:          root,
		WorkspacePath: ".ingen/workspace.yaml",
		ReceiptPath:   receiptPath,
		RoleID:        "contract-author",
		Command:       []string{"/bin/sh", "-c", "touch child-ran"},
	})
	if err == nil {
		t.Fatal("spawn accepted a terminal receipt")
	}
	assertReceiptBytesUnchanged(t, receiptFile, beforeReceipt)
	afterArtifacts, err := os.ReadDir(sessionsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterArtifacts) != len(beforeArtifacts) {
		t.Fatalf("session artifacts remain after rejected launch: before=%v after=%v", names(beforeArtifacts), names(afterArtifacts))
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("child marker stat error = %v, want child not launched", err)
	}
}

func assertReceiptBytesUnchanged(t *testing.T, path string, before []byte) {
	t.Helper()
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("receipt bytes changed from %q to %q", before, after)
	}
}

func names(entries []os.DirEntry) []string {
	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry.Name())
	}
	return result
}

func newProjectReceipt(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	if _, err := project.Initialize(project.Options{Root: root, ID: "session-test"}); err != nil {
		t.Fatal(err)
	}
	receipt, err := run.NewUnderRoot(root, ".ingen/workspace.yaml", time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	receiptPath := ".ingen/artifacts/sentinel-run.json"
	if err := run.SaveFile(filepath.Join(root, receiptPath), receipt); err != nil {
		t.Fatal(err)
	}
	return root, receiptPath
}
