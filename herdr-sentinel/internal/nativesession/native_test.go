package nativesession

import (
	"context"
	"errors"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"ingen/herdr-sentinel/internal/herdrclient"
	"ingen/herdr-sentinel/internal/nativejournal"
	"ingen/herdr-sentinel/internal/project"
	"ingen/herdr-sentinel/internal/run"
)

type fakeHost struct {
	created       int
	runCalls      int
	listCalls     int
	runCommand    string
	interrupts    int
	interruptErr  error
	pane          herdrclient.Pane
	binding       herdrclient.Binding
	createErr     error
	createApplied bool
	listErr       error
	listMatches   []herdrclient.Binding
	runErr        error
}

func (h *fakeHost) Ping(context.Context) (herdrclient.ServerInfo, error) {
	return herdrclient.ServerInfo{Version: SupportedHerdrVersion, Protocol: SupportedHerdrProtocol}, nil
}
func (h *fakeHost) CreateWorkspace(_ context.Context, cwd, label string) (herdrclient.Binding, error) {
	h.created++
	h.binding = herdrclient.Binding{WorkspaceID: "host-ws", TabID: "tab-1", PaneID: "pane-1", TerminalID: "term-1", Label: label, CWD: cwd}
	h.pane = herdrclient.Pane{WorkspaceID: "host-ws", PaneID: "pane-1", TerminalID: "term-1", CWD: cwd}
	if h.createErr != nil && !h.createApplied {
		return herdrclient.Binding{}, h.createErr
	}
	if h.createErr != nil {
		return herdrclient.Binding{}, h.createErr
	}
	return h.binding, nil
}
func (h *fakeHost) ListWorkspaces(context.Context, string, string) ([]herdrclient.Binding, error) {
	h.listCalls++
	if h.listErr != nil {
		return nil, h.listErr
	}
	if h.listMatches != nil {
		return append([]herdrclient.Binding(nil), h.listMatches...), nil
	}
	if h.binding.WorkspaceID == "" {
		return nil, nil
	}
	return []herdrclient.Binding{h.binding}, nil
}
func (h *fakeHost) Pane(context.Context, string) (herdrclient.Pane, error) { return h.pane, nil }
func (h *fakeHost) Run(_ context.Context, _, command string) error {
	h.runCalls++
	h.runCommand = command
	return h.runErr
}
func (h *fakeHost) Interrupt(context.Context, string) error {
	h.interrupts++
	return h.interruptErr
}
func (h *fakeHost) CloseWorkspace(context.Context, string) error { return nil }

func nativeFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	if _, err := project.Initialize(project.Options{Root: root, ID: "native-session-test"}); err != nil {
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

func spawnFixture(t *testing.T, command []string) (string, string, Record, *fakeHost) {
	t.Helper()
	root, receiptPath := nativeFixture(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	host := &fakeHost{}
	record, _, err := Spawn(context.Background(), Request{
		Root: root, WorkspacePath: ".ingen/workspace.yaml", ReceiptPath: receiptPath,
		RoleID: "contract-author", SocketPath: "/tmp/herdr.sock", SentinelExecutable: executable,
		Command: command,
	}, host)
	if err != nil {
		t.Fatalf("Spawn() error = %v", err)
	}
	return root, receiptPath, record, host
}

func claimAndStartForTest(t *testing.T, root, path string, record Record) Record {
	t.Helper()
	claimed, ok, err := nativejournal.ClaimExecution(root, path, record.Intent.SessionID, nativejournal.Event{
		ID: "wrapper-claim:" + record.Intent.SessionID, Kind: "wrapper-claim", At: nextTimestamp(record), State: nativejournal.StateSubmitted, Host: lastHost(record),
	})
	if err != nil || !ok {
		t.Fatalf("ClaimExecution() = %v, %v; want claimed", ok, err)
	}
	started, ok, err := nativejournal.StartExecution(root, path, record.Intent.SessionID, nativejournal.Event{
		ID: "wrapper-running:" + record.Intent.SessionID, Kind: "wrapper-started", At: nextTimestamp(claimed), State: nativejournal.StateRunning, Host: lastHost(claimed),
	})
	if err != nil || !ok {
		t.Fatalf("StartExecution() = %v, %v; want started", ok, err)
	}
	return started
}

func TestSpawnAndExecuteClaimChildExactlyOnce(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "marker")
	root, receiptPath, record, host := spawnFixture(t, []string{"/bin/sh", "-c", "printf run >> \"$1\"", "sh", marker})
	if record.State != "dispatching" {
		t.Fatalf("spawn state = %q, want dispatching reservation", record.State)
	}
	if strings.Contains(host.runCommand, marker) || strings.Contains(host.runCommand, "/bin/sh") {
		t.Fatalf("Herdr command contains child argv: %q", host.runCommand)
	}
	if err := Execute(root, filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")); err != nil {
		t.Fatal(err)
	}
	if err := Execute(root, filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")); err == nil {
		t.Fatal("duplicate wrapper execution was accepted")
	}
	markerBytes, err := os.ReadFile(marker)
	if err != nil || string(markerBytes) != "run" {
		t.Fatalf("child marker = %q, %v; want one execution", markerBytes, err)
	}
	final, err := Load(root, filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json"))
	if err != nil || final.State != "completed" {
		t.Fatalf("final record = %s, %v; want completed", final.State, err)
	}
	if _, err := Collect(root, filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json"), receiptPath); err != nil {
		t.Fatal(err)
	}
	if _, err := Collect(root, filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json"), receiptPath); err != nil {
		t.Fatalf("idempotent collection failed: %v", err)
	}
	receipt, err := run.LoadFile(filepath.Join(root, receiptPath))
	if err != nil || receipt.Status != "running" {
		t.Fatalf("successful role collection changed run lifecycle to %q: %v", receipt.Status, err)
	}
}

func TestFailedChildCollectionReplaysAfterTerminalReceipt(t *testing.T) {
	root, receiptPath, record, _ := spawnFixture(t, []string{"/bin/sh", "-c", "printf failed >&2; exit 7"})
	if err := Execute(root, filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")); err == nil {
		t.Fatal("failed child returned success")
	}
	for i := 0; i < 2; i++ {
		if _, err := Collect(root, filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json"), receiptPath); err != nil {
			t.Fatalf("collection attempt %d: %v", i+1, err)
		}
	}
	receipt, err := run.LoadFile(filepath.Join(root, receiptPath))
	if err != nil || receipt.Status != "failed" {
		t.Fatalf("receipt status = %q, %v; want failed", receipt.Status, err)
	}
}

func TestCollectionRejectsOutputDigestDriftWithoutChangingReceipt(t *testing.T) {
	root, receiptPath, record, _ := spawnFixture(t, []string{"/bin/sh", "-c", "printf captured"})
	journalPath := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")
	if err := Execute(root, journalPath); err != nil {
		t.Fatal(err)
	}
	stdoutPath := filepath.Join(root, record.Intent.StdoutPath)
	if err := os.WriteFile(stdoutPath, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	receiptFile := filepath.Join(root, receiptPath)
	before, err := os.ReadFile(receiptFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Collect(root, journalPath, receiptPath); err == nil {
		t.Fatal("collection accepted output whose bytes changed after wrapper completion")
	}
	after, err := os.ReadFile(receiptFile)
	if err != nil || string(before) != string(after) {
		t.Fatalf("receipt changed after digest mismatch: %v", err)
	}
}

func TestExecuteRejectsChangedWorkspaceManifestBeforeChildLaunch(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "must-not-run")
	root, _, record, _ := spawnFixture(t, []string{"/bin/sh", "-c", "touch \"$1\"", "sh", marker})
	manifest := filepath.Join(root, ".ingen", "workspace.yaml")
	data, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, []byte("# altered after intent\n")...)
	if err := os.WriteFile(manifest, data, 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")
	if err := Execute(root, path); err == nil {
		t.Fatal("Execute accepted a workspace manifest changed after session intent")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("child marker stat = %v, want child not launched", err)
	}
	final, err := Load(root, path)
	if err != nil || final.State != "failed" {
		t.Fatalf("journal after rejected child = %q, %v; want terminal failed", final.State, err)
	}
}

func TestChildStartFailureHasNoFabricatedProcessExitEvidence(t *testing.T) {
	root, _, record, _ := spawnFixture(t, []string{filepath.Join(t.TempDir(), "missing-command")})
	path := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")
	if err := Execute(root, path); err == nil {
		t.Fatal("Execute accepted a missing child executable")
	}
	final, err := Load(root, path)
	if err != nil {
		t.Fatal(err)
	}
	terminal := terminalEvent(final)
	if final.State != "failed" || terminal == nil || terminal.Kind != "wrapper-setup-failed" || terminal.ExitCode != nil || terminal.StdoutSHA256 != "" || terminal.StderrSHA256 != "" {
		t.Fatalf("missing-child terminal evidence = %+v; want setup failure without process exit evidence", terminal)
	}
}

func TestSignalTerminatedChildKeepsActualExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix child signal exit semantics")
	}
	for _, sig := range []string{"INT", "TERM"} {
		processErr := osexec.Command("/bin/sh", "-c", "kill -s "+sig+" $$").Run()
		if processErr == nil {
			t.Fatalf("child unexpectedly returned success after SIG%s", sig)
		}
		code, known := nativeChildExitCode(processErr)
		if !known || code != -1 {
			t.Fatalf("SIG%s child exit code = %d known=%v, want actual signal termination -1", sig, code, known)
		}
		reason := terminalReason(processErr, "SIG"+sig)
		if !strings.Contains(reason, "SIG"+sig) || !strings.Contains(reason, processErr.Error()) {
			t.Fatalf("SIG%s reason = %q, want wrapper signal and child wait error", sig, reason)
		}
	}
}

func TestDurableCancellationAllowsGracefulZeroExitAndKeepsCanceledState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix process-group signal semantics")
	}
	marker := filepath.Join(t.TempDir(), "started")
	command := []string{"/bin/sh", "-c", "trap 'exit 0' INT; printf started > \"$1\"; while :; do /bin/sleep 0.05; done", "sh", marker}
	final, executeErr := executeAndCancelForTest(t, command, marker)
	if executeErr != nil {
		t.Fatalf("graceful child cancellation returned error: %v", executeErr)
	}
	terminal := terminalEvent(final)
	if final.State != nativejournal.StateCanceled || terminal == nil || terminal.ExitCode == nil || *terminal.ExitCode != 0 {
		t.Fatalf("graceful cancellation state/terminal = %s/%+v; want canceled with actual zero exit", final.State, terminal)
	}
}

func TestDurableCancellationEscalatesAfterGraceForSignalIgnoringChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix process-group signal semantics")
	}
	marker := filepath.Join(t.TempDir(), "started")
	command := []string{"/bin/sh", "-c", "trap '' INT; printf started > \"$1\"; exec /bin/sleep 30", "sh", marker}
	final, executeErr := executeAndCancelForTest(t, command, marker)
	if executeErr == nil {
		t.Fatal("signal-ignoring child unexpectedly returned success")
	}
	terminal := terminalEvent(final)
	if final.State != nativejournal.StateCanceled || terminal == nil || terminal.ExitCode == nil || *terminal.ExitCode != -1 {
		t.Fatalf("escalated cancellation state/terminal = %s/%+v; want canceled with actual signal exit", final.State, terminal)
	}
	if !strings.Contains(terminal.Reason, "SIGKILL sent to child process group") {
		t.Fatalf("escalated cancellation reason = %q", terminal.Reason)
	}
}

func executeAndCancelForTest(t *testing.T, command []string, startedMarker string) (Record, error) {
	t.Helper()
	root, _, record, _ := spawnFixture(t, command)
	path := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")
	executeDone := make(chan error, 1)
	go func() { executeDone <- Execute(root, path) }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(startedMarker); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(startedMarker); err != nil {
		t.Fatal("native child did not start before cancellation test deadline")
	}
	current, err := Load(root, path)
	if err != nil {
		t.Fatal(err)
	}
	if current.State != nativejournal.StateRunning {
		t.Fatalf("native child start state = %s", current.State)
	}
	if _, _, err := appendEvent(root, path, "cancel-requested:"+record.Intent.SessionID, nativejournal.StateCancelRequested, lastHost(current), "operator requested cancellation", nil, "provider-cancel"); err != nil {
		t.Fatal(err)
	}
	select {
	case executeErr := <-executeDone:
		final, loadErr := Load(root, path)
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		return final, executeErr
	case <-time.After(8 * time.Second):
		t.Fatal("native child process group did not stop after bounded cancellation grace")
		return Record{}, nil
	}
}

func TestCancelDeliveryFailureIsNeverReportedAsSuccessOrRetried(t *testing.T) {
	root, _, record, host := spawnFixture(t, []string{"/bin/sh", "-c", "sleep 10"})
	path := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")
	claimAndStartForTest(t, root, path, record)
	host.interruptErr = errors.New("delivery unknown")
	if _, err := Cancel(context.Background(), root, path, host); err == nil {
		t.Fatal("uncertain interrupt was reported as successful cancellation")
	}
	if _, err := Cancel(context.Background(), root, path, host); err == nil || !strings.Contains(err.Error(), "delivery is not confirmed") {
		t.Fatalf("repeat cancellation error = %v, want explicit pending state", err)
	}
	if host.interrupts != 1 {
		t.Fatalf("Herdr received %d interrupts, want exactly one", host.interrupts)
	}
}

func TestPreclaimCancelDurablyBlocksWrapperWithoutInterruptingPane(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "should-not-run")
	root, _, record, host := spawnFixture(t, []string{"/bin/sh", "-c", "touch \"$1\"", "sh", marker})
	path := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")
	canceled, err := Cancel(context.Background(), root, path, host)
	if err != nil || canceled.State != nativejournal.StateCanceled {
		t.Fatalf("preclaim Cancel() = state %q, err %v; want terminal canceled outcome", canceled.State, err)
	}
	if host.interrupts != 0 {
		t.Fatalf("preclaim cancellation state/interrupts = %s/%d", canceled.State, host.interrupts)
	}
	if err := Execute(root, path); err == nil {
		t.Fatal("wrapper executed after durable preclaim cancellation")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("child marker stat = %v, want child not launched", err)
	}
	terminal := terminalEvent(canceled)
	if terminal == nil || terminal.Kind != nativejournal.KindProviderCanceledBeforeClaim || terminal.ExitCode != nil || terminal.StdoutSHA256 != "" || terminal.StderrSHA256 != "" {
		t.Fatalf("preclaim terminal event = %+v; want cancellation without process evidence", terminal)
	}
	if _, err := Collect(root, path, ".ingen/artifacts/sentinel-run.json"); err != nil {
		t.Fatalf("collect preclaim canceled session: %v", err)
	}
}

func TestSpawnRejectsTerminalReceiptBeforeHostWorkspaceCreation(t *testing.T) {
	root, receiptPath := nativeFixture(t)
	receiptFile := filepath.Join(root, receiptPath)
	receipt, err := run.LoadFile(receiptFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := receipt.SetStatus("completed", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := run.SaveFile(receiptFile, receipt); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(receiptFile)
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	host := &fakeHost{}
	_, recordPath, err := Spawn(context.Background(), Request{
		Root: root, WorkspacePath: ".ingen/workspace.yaml", ReceiptPath: receiptPath,
		RoleID: "contract-author", SocketPath: "/tmp/herdr.sock", SentinelExecutable: executable,
		Command: []string{"/bin/sh", "-c", "exit 0"},
	}, host)
	if err == nil || recordPath != "" {
		t.Fatalf("Spawn() = path %q, err %v; want reject before journaling", recordPath, err)
	}
	if host.created != 0 {
		t.Fatalf("created %d Herdr workspaces for terminal receipt", host.created)
	}
	after, err := os.ReadFile(receiptFile)
	if err != nil || string(after) != string(before) {
		t.Fatalf("terminal receipt changed: %v", err)
	}
}

func TestLostWorkspaceCreateResponseBindsUniqueSnapshotWithoutDispatch(t *testing.T) {
	root, receiptPath := nativeFixture(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	host := &fakeHost{
		createErr:     &herdrclient.DeliveryError{Method: "workspace.create", MayHaveApplied: true, Err: errors.New("response lost")},
		createApplied: true,
	}
	record, path, err := Spawn(context.Background(), Request{
		Root: root, WorkspacePath: ".ingen/workspace.yaml", ReceiptPath: receiptPath,
		RoleID: "contract-author", SocketPath: "/tmp/herdr.sock", SentinelExecutable: executable,
		Command: []string{"/bin/true"},
	}, host)
	if err == nil || !strings.Contains(err.Error(), "may have succeeded") {
		t.Fatalf("Spawn after lost create response = %v, want explicit uncertainty", err)
	}
	if path == "" || record.State != "indeterminate" || lastHost(record) == nil {
		t.Fatalf("reconciled create record = state %q, host %+v, path %q", record.State, lastHost(record), path)
	}
	if host.created != 1 || host.listCalls != 1 || host.runCalls != 0 {
		t.Fatalf("host calls create/list/run = %d/%d/%d, want 1/1/0", host.created, host.listCalls, host.runCalls)
	}
}

func TestRecoverBindsPreviouslyUnboundIndeterminateWithoutCreateOrDispatch(t *testing.T) {
	root, receiptPath := nativeFixture(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	host := &fakeHost{
		createErr:     &herdrclient.DeliveryError{Method: "workspace.create", MayHaveApplied: true, Err: errors.New("response lost")},
		createApplied: true,
		listErr:       errors.New("initial snapshot unavailable"),
	}
	initial, path, err := Spawn(context.Background(), Request{
		Root: root, WorkspacePath: ".ingen/workspace.yaml", ReceiptPath: receiptPath,
		RoleID: "contract-author", SocketPath: "/tmp/herdr.sock", SentinelExecutable: executable,
		Command: []string{"/bin/true"},
	}, host)
	if err == nil || initial.State != "indeterminate" || lastHost(initial) != nil {
		t.Fatalf("initial uncertain record = state %q host %+v, err %v; want unbound indeterminate", initial.State, lastHost(initial), err)
	}
	createCalls := host.created
	host.listErr = nil
	recovered, err := Recover(context.Background(), root, path, host)
	if err == nil || !strings.Contains(err.Error(), "no command was sent") {
		t.Fatalf("Recover() = %v, want explicit no-redispatch result", err)
	}
	if recovered.State != "indeterminate" || lastHost(recovered) == nil {
		t.Fatalf("recovered record = state %q host %+v, want bound indeterminate", recovered.State, lastHost(recovered))
	}
	if host.created != createCalls || host.runCalls != 0 || host.listCalls != 2 {
		t.Fatalf("after recovery create/list/run = %d/%d/%d; want unchanged create, second snapshot, no dispatch", host.created, host.listCalls, host.runCalls)
	}
	// A second recovery observes the now-bound identity and remains read-only.
	again, err := Recover(context.Background(), root, path, host)
	if err != nil || again.State != "indeterminate" || host.created != createCalls || host.runCalls != 0 {
		t.Fatalf("repeat Recover() = state %q, err %v; create/run=%d/%d", again.State, err, host.created, host.runCalls)
	}
}

func TestUncertainPaneSendIsNotRetriedAndTerminalWrapperCanRecover(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran-once")
	root, receiptPath := nativeFixture(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	host := &fakeHost{runErr: &herdrclient.DeliveryError{Method: "pane.send_input", MayHaveApplied: true, Err: errors.New("response lost")}}
	record, path, err := Spawn(context.Background(), Request{
		Root: root, WorkspacePath: ".ingen/workspace.yaml", ReceiptPath: receiptPath,
		RoleID: "contract-author", SocketPath: "/tmp/herdr.sock", SentinelExecutable: executable,
		Command: []string{"/bin/sh", "-c", "printf x >> \"$1\"", "sh", marker},
	}, host)
	if err == nil || !strings.Contains(err.Error(), "instead of resending") {
		t.Fatalf("Spawn after uncertain pane send = %v, want no-resend guidance", err)
	}
	if record.State != "dispatching" || host.runCalls != 1 {
		t.Fatalf("uncertain dispatch state/calls = %s/%d, want dispatching/1", record.State, host.runCalls)
	}
	if _, err := Recover(context.Background(), root, path, host); err != nil {
		t.Fatalf("recover exact still-owned pane before wrapper completion: %v", err)
	}
	if host.runCalls != 1 {
		t.Fatalf("Recover retried pane.send_input %d times total", host.runCalls)
	}
	if err := Execute(root, path); err != nil {
		t.Fatalf("simulate wrapper accepted by Herdr despite lost response: %v", err)
	}
	recovered, err := Recover(context.Background(), root, path, host)
	if err != nil || recovered.State != "completed" {
		t.Fatalf("recover completed wrapper = state %q, err %v", recovered.State, err)
	}
	if host.runCalls != 1 {
		t.Fatalf("terminal Recover retried pane.send_input %d times total", host.runCalls)
	}
	bytes, err := os.ReadFile(marker)
	if err != nil || string(bytes) != "x" {
		t.Fatalf("child marker = %q, %v; want one execution", bytes, err)
	}
}

func TestRecoverWithoutActiveWrapperLeaseMarksClaimedOutcomeIndeterminate(t *testing.T) {
	root, _, record, host := spawnFixture(t, []string{"/bin/true"})
	path := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")
	if _, claimed, err := nativejournal.ClaimExecution(root, path, record.Intent.SessionID, nativejournal.Event{
		ID: "wrapper-claim:" + record.Intent.SessionID, Kind: nativejournal.KindWrapperClaim, At: nextTimestamp(record), State: nativejournal.StateSubmitted, Host: lastHost(record),
	}); err != nil || !claimed {
		t.Fatalf("ClaimExecution() = %v, %v", claimed, err)
	}
	recovered, err := Recover(context.Background(), root, path, host)
	if err == nil || !strings.Contains(err.Error(), "lease was lost") {
		t.Fatalf("Recover() error = %v; want explicit lease-loss diagnostic", err)
	}
	firstJournal, readErr := os.ReadFile(filepath.Join(root, path))
	if readErr != nil {
		t.Fatal(readErr)
	}
	recoveredAgain, againErr := Recover(context.Background(), root, path, host)
	if againErr == nil || againErr.Error() != err.Error() {
		t.Fatalf("repeated Recover() error = %v, first = %v", againErr, err)
	}
	secondJournal, readErr := os.ReadFile(filepath.Join(root, path))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(firstJournal) != string(secondJournal) || recoveredAgain.State != recovered.State {
		t.Fatal("repeated recovery changed the indeterminate journal")
	}
	terminal := terminalEvent(recovered)
	if recovered.State != nativejournal.StateIndeterminate || terminal != nil || host.interrupts != 0 || host.runCalls != 1 {
		t.Fatalf("lease-lost recovery state/terminal/interrupts/runs = %s/%v/%d/%d", recovered.State, terminal, host.interrupts, host.runCalls)
	}
}

func TestRecoverAfterWrapperCrashReleasesExecutionLease(t *testing.T) {
	const helperEnv = "INGEN_NATIVE_WRAPPER_CRASH_HELPER"
	if os.Getenv(helperEnv) == "1" {
		if err := Execute(os.Getenv("INGEN_NATIVE_WRAPPER_CRASH_ROOT"), os.Getenv("INGEN_NATIVE_WRAPPER_CRASH_PATH")); err != nil {
			t.Fatalf("helper Execute() = %v", err)
		}
		return
	}

	markerDir := t.TempDir()
	startedPath := filepath.Join(markerDir, "child-started")
	finishedPath := filepath.Join(markerDir, "child-finished")
	command := []string{"/bin/sh", "-c", "printf started > \"$1\"; /bin/sleep 0.5; printf finished > \"$2\"; printf child-output", "sh", startedPath, finishedPath}
	root, _, record, host := spawnFixture(t, command)
	path := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")
	helper := osexec.Command(os.Args[0], "-test.run=^TestRecoverAfterWrapperCrashReleasesExecutionLease$")
	helper.Env = append(os.Environ(), helperEnv+"=1", "INGEN_NATIVE_WRAPPER_CRASH_ROOT="+root, "INGEN_NATIVE_WRAPPER_CRASH_PATH="+path)
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if helper.Process != nil {
			_ = helper.Process.Kill()
			_ = helper.Wait()
		}
	}()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(startedPath); err == nil {
			lease, acquired, leaseErr := nativejournal.TryExecutionLease(root, record.Intent.ExecutionLeasePath)
			if leaseErr != nil {
				t.Fatal(leaseErr)
			}
			if acquired {
				_ = lease.Close()
			} else {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(startedPath); err != nil {
		t.Fatalf("native child never reached readiness: %v", err)
	}
	lease, acquired, err := nativejournal.TryExecutionLease(root, record.Intent.ExecutionLeasePath)
	if err != nil || acquired {
		if acquired {
			_ = lease.Close()
		}
		t.Fatalf("wrapper did not hold its execution lease at child readiness: acquired=%v err=%v", acquired, err)
	}
	// Kill only the owned wrapper process. The child has a bounded natural exit
	// and a completion marker; the test never guesses or signals its PID.
	if err := helper.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := helper.Wait(); err == nil {
		t.Fatal("wrapper helper unexpectedly exited without the requested kill")
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(finishedPath); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(finishedPath); err != nil {
		t.Fatalf("child did not finish naturally after wrapper crash: %v", err)
	}

	recovered, recoverErr := Recover(context.Background(), root, path, host)
	if recoverErr == nil || !strings.Contains(recoverErr.Error(), "lease was lost") {
		t.Fatalf("Recover() = %v; want lease-loss diagnostic", recoverErr)
	}
	if recovered.State != nativejournal.StateIndeterminate || terminalEvent(recovered) != nil || host.runCalls != 1 {
		t.Fatalf("crash recovery state/terminal/host dispatches = %s/%v/%d", recovered.State, terminalEvent(recovered), host.runCalls)
	}
	for _, event := range recovered.Events {
		if event.State == nativejournal.StateIndeterminate && (event.ExitCode != nil || event.StdoutSHA256 != "" || event.StderrSHA256 != "") {
			t.Fatalf("recovery fabricated process outcome: %+v", event)
		}
	}
	journalPath := filepath.Join(root, path)
	first, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	again, againErr := Recover(context.Background(), root, path, host)
	second, readErr := os.ReadFile(journalPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if againErr == nil || againErr.Error() != recoverErr.Error() || string(first) != string(second) || again.State != recovered.State {
		t.Fatalf("repeated crash recovery changed error/state/journal: %v / %s / byte-equal=%v", againErr, again.State, string(first) == string(second))
	}
}

func TestCommandInterruptGraceUsesContainedChildPinWithoutConfusingSentinelPin(t *testing.T) {
	root, err := filepath.Abs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manifestSHA := testRoleDigest("manifest")
	policySHA := testRoleDigest("policy")
	sentinelSHA := testRoleDigest("sentinel executable")
	childSHA := testRoleDigest("contained child executable")
	if sentinelSHA == childSHA {
		t.Fatal("test requires distinct Sentinel and contained child executable pins")
	}
	intent := nativejournal.Intent{
		SentinelExecutable:       "/opt/ingen/sentinel",
		SentinelExecutableSHA256: sentinelSHA,
		ReceiptPath:              ".ingen/artifacts/run.json",
		RoleID:                   "implementation",
		WorkspaceManifestSHA256:  manifestSHA,
		Argv: []string{
			"/opt/ingen/sentinel", "role", "execute",
			"--root", root,
			"--workspace", ".ingen/workspace.yaml",
			"--receipt", ".ingen/artifacts/run.json",
			"--role", "implementation",
			"--execution-id", "exec-child-pin-test",
			"--policy-path", ".ingen/artifacts/role-executions/exec-child-pin-test.policy.json",
			"--expected-manifest-sha256", manifestSHA,
			"--expected-policy-sha256", policySHA,
			"--expected-executable-sha256", childSHA,
			"--", "/bin/true",
		},
	}
	binding, recognized, parseErr := parseRoleExecutionIntent(intent)
	if parseErr != nil || !recognized {
		t.Fatalf("parseRoleExecutionIntent() recognized=%v err=%v", recognized, parseErr)
	}
	if binding.ExecutableSHA256 != childSHA || intent.SentinelExecutableSHA256 != sentinelSHA {
		t.Fatal("test fixture did not keep contained-child and Sentinel executable pins distinct")
	}
	if grace := commandInterruptGrace(root, intent); grace != containedRoleExecutionInterruptGrace {
		t.Fatalf("commandInterruptGrace() = %s, want contained role grace %s", grace, containedRoleExecutionInterruptGrace)
	}
	intent.RoleID = "different-role"
	if grace := commandInterruptGrace(root, intent); grace != processInterruptGrace {
		t.Fatalf("mismatched role got grace %s, want generic grace %s", grace, processInterruptGrace)
	}
}

func TestRecoverBusyExecutionLeaseLeavesJournalBytesUnchanged(t *testing.T) {
	root, _, record, host := spawnFixture(t, []string{"/bin/true"})
	path := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")
	lease, acquired, err := nativejournal.TryExecutionLease(root, record.Intent.ExecutionLeasePath)
	if err != nil || !acquired {
		t.Fatalf("acquire wrapper execution lease = %v, %v", acquired, err)
	}
	defer lease.Close()
	if _, claimed, err := nativejournal.ClaimExecution(root, path, record.Intent.SessionID, nativejournal.Event{
		ID: "wrapper-claim:" + record.Intent.SessionID, Kind: nativejournal.KindWrapperClaim, At: nextTimestamp(record), State: nativejournal.StateSubmitted, Host: lastHost(record),
	}); err != nil || !claimed {
		t.Fatalf("ClaimExecution() = %v, %v", claimed, err)
	}
	before, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Recover(context.Background(), root, path, host); err == nil || !strings.Contains(err.Error(), "holds the execution lease") {
		t.Fatalf("Recover() error = %v, want active lease", err)
	}
	after, err := os.ReadFile(filepath.Join(root, path))
	if err != nil || string(before) != string(after) {
		t.Fatalf("busy recovery changed journal bytes: %v", err)
	}
}

func TestRecoverSettlesCancellationAfterClaimButBeforeChildStart(t *testing.T) {
	root, receiptPath, record, host := spawnFixture(t, []string{"/bin/true"})
	path := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")
	claimed, ok, err := nativejournal.ClaimExecution(root, path, record.Intent.SessionID, nativejournal.Event{
		ID: "wrapper-claim:" + record.Intent.SessionID, Kind: nativejournal.KindWrapperClaim, At: nextTimestamp(record), State: nativejournal.StateSubmitted, Host: lastHost(record),
	})
	if err != nil || !ok {
		t.Fatalf("ClaimExecution() = %v, %v", ok, err)
	}
	cancel, _, err := appendEvent(root, path, "cancel-requested:"+record.Intent.SessionID, nativejournal.StateCancelRequested, lastHost(claimed), "operator requested cancellation before child start", nil, "provider-cancel")
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := Recover(context.Background(), root, path, host)
	if err != nil || recovered.State != nativejournal.StateCanceled {
		t.Fatalf("Recover() = state %q, err %v; want truthful prestart cancellation", recovered.State, err)
	}
	terminal := terminalEvent(recovered)
	if terminal == nil || terminal.Kind != nativejournal.KindProviderCanceledBeforeStart || terminal.ExitCode != nil || terminal.StdoutSHA256 != "" || terminal.StderrSHA256 != "" {
		t.Fatalf("settlement event = %+v; want no child process evidence", terminal)
	}
	if hasWrapperStarted(recovered) || host.interrupts != 0 {
		t.Fatalf("recovery fabricated child start or signaled host: started=%v interrupts=%d", hasWrapperStarted(recovered), host.interrupts)
	}
	if _, started, err := nativejournal.StartExecution(root, path, record.Intent.SessionID, nativejournal.Event{
		ID: "wrapper-running:" + record.Intent.SessionID, Kind: nativejournal.KindWrapperStarted, At: nextTimestamp(cancel), State: nativejournal.StateRunning, Host: lastHost(recovered),
	}); err != nil || started {
		t.Fatalf("StartExecution after settled cancel = started %v err %v; want denied", started, err)
	}
	if _, err := Collect(root, path, receiptPath); err != nil {
		t.Fatalf("collect canceled-before-start session: %v", err)
	}
}

func TestCancelDefersPreclaimSettlementWhileWrapperLeaseIsBusy(t *testing.T) {
	root, _, record, host := spawnFixture(t, []string{"/bin/true"})
	path := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")
	lease, acquired, err := nativejournal.TryExecutionLease(root, record.Intent.ExecutionLeasePath)
	if err != nil || !acquired {
		t.Fatalf("acquire wrapper lease = %v, %v", acquired, err)
	}
	updated, err := Cancel(context.Background(), root, path, host)
	if err == nil || !strings.Contains(err.Error(), "settlement is deferred") || updated.State != nativejournal.StateCancelRequested {
		t.Fatalf("Cancel() = state %q, err %v; want pending cancellation while wrapper active", updated.State, err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	recovered, err := Recover(context.Background(), root, path, host)
	if err != nil || recovered.State != nativejournal.StateCanceled {
		t.Fatalf("Recover() after lease release = state %q err %v; want canceled-before-claim", recovered.State, err)
	}
}

func TestRecoveryIndeterminateDoesNotEraseDurableCancellationForWrapper(t *testing.T) {
	startedFile := filepath.Join(t.TempDir(), "child-started")
	root, _, record, host := spawnFixture(t, []string{"/bin/sh", "-c", "printf ready > \"$1\"; exec /bin/sleep 30", "sh", startedFile})
	path := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")
	executeDone := make(chan error, 1)
	go func() { executeDone <- Execute(root, path) }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(startedFile); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(startedFile); err != nil {
		t.Fatal("native child did not start before deadline")
	}
	host.interruptErr = errors.New("interrupt response lost")
	if _, err := Cancel(context.Background(), root, path, host); err == nil {
		t.Fatal("uncertain Herdr interrupt was reported as delivered")
	}
	beforeRecovery, err := Load(root, path)
	if err != nil || beforeRecovery.State != nativejournal.StateCancelRequested {
		t.Fatalf("state before active recovery = %q, err %v", beforeRecovery.State, err)
	}
	beforeBytes, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	if recovered, err := Recover(context.Background(), root, path, host); err == nil || recovered.State != nativejournal.StateCancelRequested {
		t.Fatalf("Recover() = state %q, err %v; want active wrapper and unchanged cancellation", recovered.State, err)
	}
	afterBytes, err := os.ReadFile(filepath.Join(root, path))
	if err != nil || string(beforeBytes) != string(afterBytes) {
		t.Fatalf("active recovery changed cancellation journal: %v", err)
	}
	select {
	case err := <-executeDone:
		if err == nil {
			t.Fatal("canceled child unexpectedly returned success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("wrapper did not honor durable cancellation after recovery marked state indeterminate")
	}
	final, err := Load(root, path)
	if err != nil || final.State != "canceled" || !hasCancellationRequest(final) {
		t.Fatalf("final journal state/cancel history = %s/%v, %v", final.State, hasCancellationRequest(final), err)
	}
}
