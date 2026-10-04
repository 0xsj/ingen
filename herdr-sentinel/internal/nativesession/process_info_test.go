package nativesession

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ingen/herdr-sentinel/internal/herdrclient"
)

type processInfoTestHost struct {
	*fakeHost
	processCalls int
	processInfo  herdrclient.ProcessInfo
	closeCalls   int
	closedID     string
}

func (h *processInfoTestHost) ProcessInfo(_ context.Context, paneID string) (herdrclient.ProcessInfo, error) {
	h.processCalls++
	if paneID != h.processInfo.PaneID {
		return herdrclient.ProcessInfo{}, nil
	}
	return h.processInfo, nil
}

func (h *processInfoTestHost) CloseWorkspace(_ context.Context, workspaceID string) error {
	h.closeCalls++
	h.closedID = workspaceID
	return nil
}

func TestObservePaneProcessesChecksStoredIdentityAndDoesNotMutateJournal(t *testing.T) {
	root, receiptPath := nativeFixture(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	base := &fakeHost{}
	host := &processInfoTestHost{fakeHost: base}
	record, journalPath, err := Spawn(context.Background(), Request{
		Root: root, WorkspacePath: ".ingen/workspace.yaml", ReceiptPath: receiptPath,
		RoleID: "contract-author", SocketPath: "/tmp/herdr.sock", SentinelExecutable: executable,
		Command: []string{"/bin/echo", "native process info fixture"},
	}, host)
	if err != nil {
		t.Fatalf("Spawn() = %v", err)
	}
	host.processInfo = herdrclient.ProcessInfo{
		PaneID: "pane-1", ShellPID: uint32Ptr(41), ForegroundProcessGroupID: uint32Ptr(42),
		ForegroundProcesses: []herdrclient.Process{{PID: 43, Name: "fixture", Argv: []string{"fixture"}}},
	}
	journalFile := filepath.Join(root, journalPath)
	before, err := os.ReadFile(journalFile)
	if err != nil {
		t.Fatal(err)
	}

	host.pane.TerminalID = "replaced-terminal"
	if _, err := ObservePaneProcesses(context.Background(), root, journalPath, host); err == nil {
		t.Fatal("process observation accepted a pane whose terminal identity changed")
	}
	if host.processCalls != 0 {
		t.Fatalf("process info called %d times after pane identity mismatch", host.processCalls)
	}
	host.pane.TerminalID = "term-1"
	observation, err := ObservePaneProcesses(context.Background(), root, journalPath, host)
	if err != nil {
		t.Fatalf("ObservePaneProcesses() = %v", err)
	}
	if observation.Schema != "ingen.sentinel-native-process-observation/v1" || observation.SessionID != record.Intent.SessionID || observation.Host.PaneID != "pane-1" || observation.ProcessInfo.ForegroundProcesses[0].PID != 43 || observation.Trust != "local-observation-unverified" {
		t.Fatalf("process observation = %+v", observation)
	}
	if host.processCalls != 1 || base.runCalls != 1 || base.interrupts != 0 || host.closeCalls != 0 {
		t.Fatalf("calls after read-only observation: process=%d run=%d interrupts=%d close=%d", host.processCalls, base.runCalls, base.interrupts, host.closeCalls)
	}
	after, err := os.ReadFile(journalFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("read-only process observation changed the native journal")
	}
}

func uint32Ptr(value uint32) *uint32 { return &value }
