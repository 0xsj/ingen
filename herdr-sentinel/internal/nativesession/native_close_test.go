package nativesession

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/herdr-sentinel/internal/nativejournal"
)

func TestCloseOwnedWorkspaceRequiresKnownTerminalAndExactIdentity(t *testing.T) {
	root, _, record, base := spawnFixture(t, []string{"/bin/echo", "close fixture"})
	host := &processInfoTestHost{fakeHost: base}
	recordPath := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")

	if _, err := CloseOwnedWorkspace(context.Background(), root, recordPath, host); err == nil || !strings.Contains(err.Error(), "known process outcome") {
		t.Fatalf("close of submitted session = %v; want unknown-outcome rejection", err)
	}
	if host.closeCalls != 0 {
		t.Fatal("unknown session was closed")
	}

	started := claimAndStartForTest(t, root, recordPath, record)
	code := 0
	if _, _, err := appendEvent(root, recordPath, "wrapper-terminal:"+record.Intent.SessionID, nativejournal.StateCompleted, lastHost(started), "", &code, nativejournal.KindWrapperTerminal, strings.Repeat("a", 64), strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	journalBefore, err := os.ReadFile(filepath.Join(root, recordPath))
	if err != nil {
		t.Fatal(err)
	}

	host.pane.TerminalID = "wrong-terminal"
	if _, err := CloseOwnedWorkspace(context.Background(), root, recordPath, host); err == nil {
		t.Fatal("close accepted a changed terminal identity")
	}
	if host.closeCalls != 0 {
		t.Fatal("workspace was closed after terminal identity mismatch")
	}
	host.pane.TerminalID = "term-1"
	closed, err := CloseOwnedWorkspace(context.Background(), root, recordPath, host)
	if err != nil {
		t.Fatalf("CloseOwnedWorkspace() = %v", err)
	}
	if closed.Action != "closed-exact-workspace" || closed.Host.WorkspaceID != "host-ws" || host.closeCalls != 1 || host.closedID != "host-ws" {
		t.Fatalf("close result=%+v calls=%d id=%q", closed, host.closeCalls, host.closedID)
	}
	journalAfter, err := os.ReadFile(filepath.Join(root, recordPath))
	if err != nil {
		t.Fatal(err)
	}
	if string(journalBefore) != string(journalAfter) {
		t.Fatal("workspace close changed native journal bytes")
	}
}

func TestCloseOwnedWorkspaceAllowsOnlyExplicitNoStartCancellation(t *testing.T) {
	root, _, record, base := spawnFixture(t, []string{"/bin/echo", "never started"})
	recordPath := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")
	requested, _, err := appendEvent(root, recordPath, "cancel-requested:"+record.Intent.SessionID, nativejournal.StateCancelRequested, lastHost(record), "canceled before claim", nil, "provider-cancel")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := appendEvent(root, recordPath, "canceled-before-claim:"+record.Intent.SessionID, nativejournal.StateCanceled, lastHost(requested), "operator canceled before wrapper claim; no child process ran", nil, nativejournal.KindProviderCanceledBeforeClaim); err != nil {
		t.Fatal(err)
	}
	host := &processInfoTestHost{fakeHost: base}
	result, err := CloseOwnedWorkspace(context.Background(), root, recordPath, host)
	if err != nil {
		t.Fatalf("close proven no-start canceled session: %v", err)
	}
	if result.JournalState != nativejournal.StateCanceled || host.closeCalls != 1 || host.closedID != "host-ws" {
		t.Fatalf("result=%+v calls=%d id=%q", result, host.closeCalls, host.closedID)
	}
}

func TestCloseOwnedWorkspaceNeedsLiveExactPaneObservation(t *testing.T) {
	root, _, record, base := spawnFixture(t, []string{"/bin/echo", "close fixture"})
	recordPath := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")
	started := claimAndStartForTest(t, root, recordPath, record)
	code := 0
	if _, _, err := appendEvent(root, recordPath, "wrapper-terminal:"+record.Intent.SessionID, nativejournal.StateCompleted, lastHost(started), "", &code, nativejournal.KindWrapperTerminal, strings.Repeat("a", 64), strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	host := &processInfoTestHost{fakeHost: base}
	host.pane.CWD = filepath.Join(root, "other")
	if _, err := CloseOwnedWorkspace(context.Background(), root, recordPath, host); err == nil {
		t.Fatal("close accepted mismatched live pane CWD")
	}
	if host.closeCalls != 0 {
		t.Fatal("workspace was closed after live pane mismatch")
	}
	if _, err := CloseOwnedWorkspace(nil, root, recordPath, host); err == nil {
		t.Fatal("close accepted nil context")
	}
}

func TestCloseOwnedWorkspaceRejectsUnknownChildOutcomes(t *testing.T) {
	for _, state := range []string{"active", "indeterminate", "setup-failed"} {
		t.Run(state, func(t *testing.T) {
			root, _, record, base := spawnFixture(t, []string{"/bin/echo", "outcome unknown"})
			recordPath := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")
			switch state {
			case "active":
				claimAndStartForTest(t, root, recordPath, record)
			case "indeterminate":
				started := claimAndStartForTest(t, root, recordPath, record)
				if _, _, err := appendEvent(root, recordPath, "unknown:"+record.Intent.SessionID, nativejournal.StateIndeterminate, lastHost(started), "wrapper disappeared before child outcome was recorded", nil, "provider-recovery"); err != nil {
					t.Fatal(err)
				}
			case "setup-failed":
				if _, _, err := appendEvent(root, recordPath, "setup-failed:"+record.Intent.SessionID, nativejournal.StateFailed, lastHost(record), "wrapper setup failed before child process outcome was known", nil, "wrapper-setup-failed"); err != nil {
					t.Fatal(err)
				}
			}
			host := &processInfoTestHost{fakeHost: base}
			if _, err := CloseOwnedWorkspace(context.Background(), root, recordPath, host); err == nil {
				t.Fatalf("CloseOwnedWorkspace accepted %s child outcome", state)
			}
			if host.closeCalls != 0 {
				t.Fatalf("CloseWorkspace called %d times for %s child outcome", host.closeCalls, state)
			}
		})
	}
}

func TestCloseOwnedWorkspaceRequiresActualExecuteTerminalAndPreservesJournal(t *testing.T) {
	root, _, record, base := spawnFixture(t, []string{"/bin/echo", "known completed child"})
	recordPath := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")
	if err := Execute(root, recordPath); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	final, err := Load(root, recordPath)
	if err != nil {
		t.Fatal(err)
	}
	terminal := terminalEvent(final)
	if final.State != nativejournal.StateCompleted || terminal == nil || terminal.Kind != nativejournal.KindWrapperTerminal || terminal.ExitCode == nil || *terminal.ExitCode != 0 {
		t.Fatalf("Execute terminal evidence = state %q, event %+v", final.State, terminal)
	}
	journalPath := filepath.Join(root, recordPath)
	before, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	host := &processInfoTestHost{fakeHost: base}
	result, err := CloseOwnedWorkspace(context.Background(), root, recordPath, host)
	if err != nil {
		t.Fatalf("CloseOwnedWorkspace() = %v", err)
	}
	if result.Action != "closed-exact-workspace" || result.Host.WorkspaceID != lastHost(final).WorkspaceID || host.closeCalls != 1 || host.closedID != lastHost(final).WorkspaceID {
		t.Fatalf("close result=%+v calls=%d id=%q", result, host.closeCalls, host.closedID)
	}
	after, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("closing completed workspace changed native journal bytes")
	}
}
