package nativejournal

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCreateAppendReplayAndDuplicateIdentity(t *testing.T) {
	fixture := newFixture(t)
	if err := Create(fixture.root, fixture.journalPath, fixture.record); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(fixture.root, fixture.journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != StatePrepared || len(loaded.Events) != 1 || loaded.Origin != Origin || loaded.Enforcement != Enforcement || loaded.Assurance != Assurance {
		t.Fatalf("loaded record = %+v", loaded)
	}
	createdBytes, err := os.ReadFile(filepath.Join(fixture.root, fixture.journalPath))
	if err != nil {
		t.Fatal(err)
	}
	if err := Create(fixture.root, fixture.journalPath, fixture.record); err == nil || !strings.Contains(err.Error(), "refusing to replace") {
		t.Fatalf("Create over existing journal error = %v", err)
	}
	stillCreatedBytes, err := os.ReadFile(filepath.Join(fixture.root, fixture.journalPath))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(createdBytes, stillCreatedBytes) {
		t.Fatal("refused Create changed the existing journal bytes")
	}

	dispatch := fixture.event("dispatch-event", "dispatch-reserved", StateDispatching, 1)
	updated, changed, err := Append(fixture.root, fixture.journalPath, dispatch)
	if err != nil || !changed || updated.State != StateDispatching {
		t.Fatalf("Append(dispatch) = state %q, changed %v, err %v", updated.State, changed, err)
	}
	rawRunning := fixture.event("raw-running", "wrapper-state", StateRunning, 2)
	if _, _, err := Append(fixture.root, fixture.journalPath, rawRunning); err == nil || !strings.Contains(err.Error(), "invalid transition") {
		t.Fatalf("raw Append(dispatching -> running) error = %v", err)
	}
	if loaded, err := Load(fixture.root, fixture.journalPath); err != nil || loaded.State != StateDispatching {
		t.Fatalf("raw running append changed dispatch state: state=%q err=%v", loaded.State, err)
	}
	before, err := os.ReadFile(filepath.Join(fixture.root, fixture.journalPath))
	if err != nil {
		t.Fatal(err)
	}
	updated, changed, err = Append(fixture.root, fixture.journalPath, dispatch)
	if err != nil || changed || updated.State != StateDispatching {
		t.Fatalf("Append(exact retry) = state %q, changed %v, err %v", updated.State, changed, err)
	}
	after, err := os.ReadFile(filepath.Join(fixture.root, fixture.journalPath))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("exact event retry rewrote journal bytes")
	}

	changedEvent := dispatch
	changedEvent.Reason = "reused identity"
	if _, _, err := Append(fixture.root, fixture.journalPath, changedEvent); err == nil || !strings.Contains(err.Error(), "reused with different content") {
		t.Fatalf("changed event ID reuse error = %v", err)
	}
}

func TestClaimExecutionIsProcessSharedCASAndLateAckDoesNotRegress(t *testing.T) {
	fixture := newFixture(t)
	if err := Create(fixture.root, fixture.journalPath, fixture.record); err != nil {
		t.Fatal(err)
	}
	if _, changed, err := Append(fixture.root, fixture.journalPath, fixture.event("dispatch", "dispatch-reserved", StateDispatching, 1)); err != nil || !changed {
		t.Fatalf("reserve dispatch: changed=%v err=%v", changed, err)
	}

	claim := fixture.event("wrapper-claim", "wrapper-claim", StateSubmitted, 3)
	const callers = 16
	var wg sync.WaitGroup
	var mu sync.Mutex
	claimCount := 0
	errorsSeen := make([]error, 0)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, claimed, err := ClaimExecution(fixture.root, fixture.journalPath, fixture.record.Intent.SessionID, claim)
			mu.Lock()
			defer mu.Unlock()
			if claimed {
				claimCount++
			}
			if err != nil {
				errorsSeen = append(errorsSeen, err)
			}
		}()
	}
	wg.Wait()
	if len(errorsSeen) != 0 || claimCount != 1 {
		t.Fatalf("concurrent claims: count=%d errors=%v", claimCount, errorsSeen)
	}

	ack := fixture.event("host-ack", "host-dispatch-ack", StateSubmitted, 4)
	updated, changed, err := Append(fixture.root, fixture.journalPath, ack)
	if err != nil || !changed || updated.State != StateSubmitted {
		t.Fatalf("late dispatch acknowledgement = state %q, changed %v, err %v", updated.State, changed, err)
	}
	running := fixture.event("wrapper-running", KindWrapperStarted, StateRunning, 5)
	if _, _, err := Append(fixture.root, fixture.journalPath, running); err == nil || !strings.Contains(err.Error(), "must use StartExecution") {
		t.Fatalf("raw Append(submitted -> running) error = %v", err)
	}
	updated, changed, err = StartExecution(fixture.root, fixture.journalPath, fixture.record.Intent.SessionID, running)
	if err != nil || !changed || updated.State != StateRunning {
		t.Fatalf("StartExecution(running) = state %q, changed %v, err %v", updated.State, changed, err)
	}
	lateAck := fixture.event("host-ack-late", "host-dispatch-ack", StateSubmitted, 6)
	updated, changed, err = Append(fixture.root, fixture.journalPath, lateAck)
	if err != nil || !changed || updated.State != StateRunning {
		t.Fatalf("late ack after running = state %q, changed %v, err %v", updated.State, changed, err)
	}
	code := 0
	completed := fixture.event("wrapper-done", "wrapper-terminal", StateCompleted, 7)
	completed.ExitCode = &code
	completed.StdoutSHA256 = strings.Repeat("a", 64)
	completed.StderrSHA256 = strings.Repeat("b", 64)
	updated, changed, err = Append(fixture.root, fixture.journalPath, completed)
	if err != nil || !changed || updated.State != StateCompleted {
		t.Fatalf("Append(completed) = state %q, changed %v, err %v", updated.State, changed, err)
	}
	terminalPath := filepath.Join(fixture.root, fixture.journalPath)
	terminalBytes, err := os.ReadFile(terminalPath)
	if err != nil {
		t.Fatal(err)
	}
	terminalAck := fixture.event("host-ack-terminal", "host-dispatch-ack", StateSubmitted, 8)
	updated, changed, err = Append(fixture.root, fixture.journalPath, terminalAck)
	if err != nil || changed || updated.State != StateCompleted {
		t.Fatalf("late ack after terminal = state %q, changed %v, err %v", updated.State, changed, err)
	}
	afterTerminalAck, err := os.ReadFile(terminalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(terminalBytes, afterTerminalAck) {
		t.Fatal("late acknowledgement rewrote a terminal journal")
	}
	if _, _, err := Append(fixture.root, fixture.journalPath, fixture.event("regress", "wrapper-state", StateRunning, 9)); err == nil || !strings.Contains(err.Error(), "terminal state") {
		t.Fatalf("terminal state regression error = %v", err)
	}
	if _, err := Load(fixture.root, fixture.journalPath); err != nil {
		t.Fatalf("replay terminal journal: %v", err)
	}
}

func TestClaimExecutionUsesLockedMonotonicTimeAgainstConcurrentAck(t *testing.T) {
	fixture := newFixture(t)
	if err := Create(fixture.root, fixture.journalPath, fixture.record); err != nil {
		t.Fatal(err)
	}
	if _, changed, err := Append(fixture.root, fixture.journalPath, fixture.event("dispatch", "dispatch-reserved", StateDispatching, 1)); err != nil || !changed {
		t.Fatalf("reserve dispatch: changed=%v err=%v", changed, err)
	}
	claim := fixture.event("claim-race", "wrapper-claim", StateSubmitted, 2)
	ack := fixture.event("ack-race", "host-dispatch-ack", StateSubmitted, 86400)
	start := make(chan struct{})
	var wg sync.WaitGroup
	var claimed bool
	var claimErr, ackErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, claimed, claimErr = ClaimExecution(fixture.root, fixture.journalPath, fixture.record.Intent.SessionID, claim)
	}()
	go func() {
		defer wg.Done()
		<-start
		_, _, ackErr = Append(fixture.root, fixture.journalPath, ack)
	}()
	close(start)
	wg.Wait()
	if claimErr != nil || !claimed {
		t.Fatalf("wrapper claim lost to concurrent acknowledgement: claimed=%v err=%v", claimed, claimErr)
	}
	if ackErr != nil {
		t.Fatalf("host acknowledgement failed regardless of lock order: %v", ackErr)
	}
	loaded, err := Load(fixture.root, fixture.journalPath)
	if err != nil || loaded.State != StateSubmitted {
		t.Fatalf("reloaded state = %q err %v", loaded.State, err)
	}
}

func TestStartExecutionAndCancellationSerializeWithoutClearingCancellation(t *testing.T) {
	fixture := newFixture(t)
	if err := Create(fixture.root, fixture.journalPath, fixture.record); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Append(fixture.root, fixture.journalPath, fixture.event("dispatch", "dispatch-reserved", StateDispatching, 1)); err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := ClaimExecution(fixture.root, fixture.journalPath, fixture.record.Intent.SessionID, fixture.event("wrapper-claim", KindWrapperClaim, StateSubmitted, 2)); err != nil || !claimed {
		t.Fatalf("wrapper claim: claimed=%v err=%v", claimed, err)
	}
	startEvent := fixture.event("wrapper-start", KindWrapperStarted, StateRunning, 3)
	cancelEvent := fixture.event("cancel-race", "provider-cancel", StateCancelRequested, 86400)
	cancelEvent.Reason = "operator requested cancellation"
	start := make(chan struct{})
	var wg sync.WaitGroup
	var started bool
	var startErr, cancelErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, started, startErr = StartExecution(fixture.root, fixture.journalPath, fixture.record.Intent.SessionID, startEvent)
	}()
	go func() {
		defer wg.Done()
		<-start
		_, _, cancelErr = Append(fixture.root, fixture.journalPath, cancelEvent)
	}()
	close(start)
	wg.Wait()
	if startErr != nil || cancelErr != nil {
		t.Fatalf("start/cancel race: started=%v startErr=%v cancelErr=%v", started, startErr, cancelErr)
	}
	loaded, err := Load(fixture.root, fixture.journalPath)
	if err != nil || loaded.State != StateCancelRequested {
		t.Fatalf("reloaded state = %q err %v, want cancellation preserved", loaded.State, err)
	}
}

func TestCancellationAndHostBindingPreventUnsafeLaunchDrift(t *testing.T) {
	fixture := newFixture(t)
	if err := Create(fixture.root, fixture.journalPath, fixture.record); err != nil {
		t.Fatal(err)
	}
	host := &Host{WorkspaceID: "herdr-workspace", SentinelWorkspaceID: fixture.record.Intent.WorkspaceID, PaneID: "pane-1"}
	dispatch := fixture.event("dispatch", "dispatch-reserved", StateDispatching, 1)
	dispatch.Host = host
	if _, _, err := Append(fixture.root, fixture.journalPath, dispatch); err != nil {
		t.Fatal(err)
	}
	cancel := fixture.event("cancel", "cancel-request", StateCancelRequested, 2)
	cancel.Host = host
	if _, _, err := Append(fixture.root, fixture.journalPath, cancel); err != nil {
		t.Fatal(err)
	}
	_, claimed, err := ClaimExecution(fixture.root, fixture.journalPath, fixture.record.Intent.SessionID, fixture.event("wrapper-claim", "wrapper-claim", StateSubmitted, 3))
	if err != nil || claimed {
		t.Fatalf("claim after cancellation = %v, %v; want no launch", claimed, err)
	}
	startedEvent := fixture.event("wrapper-start-after-cancel", "wrapper-started", StateRunning, 3)
	updated, started, err := StartExecution(fixture.root, fixture.journalPath, fixture.record.Intent.SessionID, startedEvent)
	if err != nil || started || updated.State != StateCancelRequested {
		t.Fatalf("child start after cancellation = state %q started %v err %v", updated.State, started, err)
	}
	observed, changed, err := Append(fixture.root, fixture.journalPath, startedEvent)
	if err != nil || !changed || observed.State != StateCancelRequested {
		t.Fatalf("late child start observation = state %q changed %v err %v", observed.State, changed, err)
	}

	changedHost := fixture.event("bad-host-update", "host-observation", StateCancelRequested, 4)
	changedHost.Host = &Host{WorkspaceID: "herdr-workspace", SentinelWorkspaceID: fixture.record.Intent.WorkspaceID, PaneID: "pane-2"}
	if _, _, err := Append(fixture.root, fixture.journalPath, changedHost); err == nil || !strings.Contains(err.Error(), "pane binding is immutable") {
		t.Fatalf("host pane swap error = %v", err)
	}
}

func TestWrapperFinishedRequiresConsistentExitCode(t *testing.T) {
	fixture := newFixture(t)
	if err := Create(fixture.root, fixture.journalPath, fixture.record); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Append(fixture.root, fixture.journalPath, fixture.event("failed", "wrapper-terminal", StateCompleted, 1)); err == nil || !strings.Contains(err.Error(), "exit code") {
		t.Fatalf("completed wrapper without zero exit code error = %v", err)
	}
	if _, _, err := Append(fixture.root, fixture.journalPath, fixture.event("dispatch", "dispatch-reserved", StateDispatching, 1)); err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := ClaimExecution(fixture.root, fixture.journalPath, fixture.record.Intent.SessionID, fixture.event("claim", "wrapper-claim", StateSubmitted, 2)); err != nil || !claimed {
		t.Fatalf("claim before wrapper exit: claimed=%v err=%v", claimed, err)
	}
	if _, started, err := StartExecution(fixture.root, fixture.journalPath, fixture.record.Intent.SessionID, fixture.event("running", KindWrapperStarted, StateRunning, 3)); err != nil || !started {
		t.Fatalf("start before wrapper exit: started=%v err=%v", started, err)
	}
	code := 1
	failed := fixture.event("failed", "wrapper-terminal", StateFailed, 4)
	failed.ExitCode = &code
	failed.StdoutSHA256 = strings.Repeat("a", 64)
	failed.StderrSHA256 = strings.Repeat("b", 64)
	if _, changed, err := Append(fixture.root, fixture.journalPath, failed); err != nil || !changed {
		t.Fatalf("failed wrapper with nonzero exit: changed=%v err=%v", changed, err)
	}
}

func TestRunningCanCompleteCanceledOnlyWithWrapperEvidence(t *testing.T) {
	fixture := newFixture(t)
	if err := Create(fixture.root, fixture.journalPath, fixture.record); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Append(fixture.root, fixture.journalPath, fixture.event("dispatch", "dispatch-reserved", StateDispatching, 1)); err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := ClaimExecution(fixture.root, fixture.journalPath, fixture.record.Intent.SessionID, fixture.event("claim", KindWrapperClaim, StateSubmitted, 2)); err != nil || !claimed {
		t.Fatalf("claim before cancellation terminal: claimed=%v err=%v", claimed, err)
	}
	if _, started, err := StartExecution(fixture.root, fixture.journalPath, fixture.record.Intent.SessionID, fixture.event("started", KindWrapperStarted, StateRunning, 3)); err != nil || !started {
		t.Fatalf("start before cancellation terminal: started=%v err=%v", started, err)
	}
	if _, _, err := Append(fixture.root, fixture.journalPath, fixture.event("reasonless-canceled", "wrapper-state", StateCanceled, 4)); err == nil || !strings.Contains(err.Error(), "requires a reason") {
		t.Fatalf("generic reasonless canceled event error = %v", err)
	}
	code := 130
	canceled := fixture.event("wrapper-canceled", KindWrapperTerminal, StateCanceled, 4)
	canceled.Reason = "wrapper received cancellation signal"
	canceled.ExitCode = &code
	canceled.StdoutSHA256 = strings.Repeat("a", 64)
	canceled.StderrSHA256 = strings.Repeat("b", 64)
	updated, changed, err := Append(fixture.root, fixture.journalPath, canceled)
	if err != nil || !changed || updated.State != StateCanceled {
		t.Fatalf("wrapper cancellation terminal = state %q changed %v err %v", updated.State, changed, err)
	}
	if _, err := Load(fixture.root, fixture.journalPath); err != nil {
		t.Fatalf("replay canceled terminal: %v", err)
	}
}

func TestProviderCanSettlePreclaimCancellationWithoutProcessEvidence(t *testing.T) {
	fixture := newFixture(t)
	if err := Create(fixture.root, fixture.journalPath, fixture.record); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Append(fixture.root, fixture.journalPath, fixture.event("dispatch", "dispatch-reserved", StateDispatching, 1)); err != nil {
		t.Fatal(err)
	}
	cancelRequested := fixture.event("cancel-requested", "provider-cancel", StateCancelRequested, 2)
	cancelRequested.Reason = "operator canceled before wrapper claim"
	if _, _, err := Append(fixture.root, fixture.journalPath, cancelRequested); err != nil {
		t.Fatal(err)
	}
	settled := fixture.event("preclaim-canceled", KindProviderCanceledBeforeClaim, StateCanceled, 3)
	settled.Reason = "cancellation blocked wrapper claim; no child process existed"
	updated, changed, err := Append(fixture.root, fixture.journalPath, settled)
	if err != nil || !changed || updated.State != StateCanceled {
		t.Fatalf("preclaim cancellation settlement = state %q changed %v err %v", updated.State, changed, err)
	}
	if terminal := updated.Events[len(updated.Events)-1]; terminal.ExitCode != nil || terminal.StdoutSHA256 != "" || terminal.StderrSHA256 != "" {
		t.Fatalf("preclaim cancellation fabricated child evidence: %+v", terminal)
	}
	if _, err := Load(fixture.root, fixture.journalPath); err != nil {
		t.Fatalf("replay preclaim cancellation settlement: %v", err)
	}
}

func TestProviderCannotSettleCancellationAfterWrapperClaim(t *testing.T) {
	fixture := newFixture(t)
	if err := Create(fixture.root, fixture.journalPath, fixture.record); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Append(fixture.root, fixture.journalPath, fixture.event("dispatch", "dispatch-reserved", StateDispatching, 1)); err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := ClaimExecution(fixture.root, fixture.journalPath, fixture.record.Intent.SessionID, fixture.event("claim", KindWrapperClaim, StateSubmitted, 2)); err != nil || !claimed {
		t.Fatalf("claim before cancellation: claimed=%v err=%v", claimed, err)
	}
	cancelRequested := fixture.event("cancel-requested", "provider-cancel", StateCancelRequested, 3)
	cancelRequested.Reason = "operator requested cancellation"
	if _, _, err := Append(fixture.root, fixture.journalPath, cancelRequested); err != nil {
		t.Fatal(err)
	}
	settled := fixture.event("preclaim-canceled", KindProviderCanceledBeforeClaim, StateCanceled, 4)
	settled.Reason = "no child process existed"
	if _, _, err := Append(fixture.root, fixture.journalPath, settled); err == nil || !strings.Contains(err.Error(), "invalid after wrapper claim") {
		t.Fatalf("postclaim preclaim-settlement error = %v", err)
	}
}

func TestLoadRejectsUnknownTrailingAndInconsistentReplayButSurvivesExecutableRemoval(t *testing.T) {
	fixture := newFixture(t)
	if err := Create(fixture.root, fixture.journalPath, fixture.record); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(fixture.record.Intent.SentinelExecutable); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(fixture.root, fixture.journalPath); err != nil {
		t.Fatalf("load should not depend on the historical Sentinel executable: %v", err)
	}
	journal := filepath.Join(fixture.root, fixture.journalPath)
	contents, err := os.ReadFile(journal)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(contents, &value); err != nil {
		t.Fatal(err)
	}
	value["unexpected"] = true
	unknown, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(journal, unknown, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(fixture.root, fixture.journalPath); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown field error = %v", err)
	}
	duplicate := bytes.Replace(contents, []byte(`"run_id": "run-test",`), []byte(`"run_id": "other", "run_id": "run-test",`), 1)
	if err := os.WriteFile(journal, duplicate, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(fixture.root, fixture.journalPath); err == nil || !strings.Contains(err.Error(), "duplicate JSON object key") {
		t.Fatalf("duplicate key error = %v", err)
	}
	if err := os.WriteFile(journal, append(contents, []byte("{}\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(fixture.root, fixture.journalPath); err == nil || !strings.Contains(err.Error(), "trailing JSON value") {
		t.Fatalf("trailing value error = %v", err)
	}
	corrupt := bytes.Replace(contents, []byte(`"state": "prepared"`), []byte(`"state": "completed"`), 1)
	if err := os.WriteFile(journal, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(fixture.root, fixture.journalPath); err == nil || !strings.Contains(err.Error(), "does not match replayed state") {
		t.Fatalf("replay mismatch error = %v", err)
	}
}

func TestCreateAndLoadRejectEscapingAndSymlinkPaths(t *testing.T) {
	fixture := newFixture(t)
	if _, err := Load(fixture.root, "../outside.json"); err == nil {
		t.Fatal("Load accepted a parent traversal")
	}
	if err := os.Symlink(fixture.root, filepath.Join(fixture.root, "linked-workdir")); err != nil {
		t.Fatal(err)
	}
	fixture.record.Intent.Workdir = "linked-workdir"
	if err := Create(fixture.root, fixture.journalPath, fixture.record); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("Create through symlinked workdir error = %v", err)
	}
	fixture.record.Intent.Workdir = "work"
	if err := Create(fixture.root, fixture.journalPath, fixture.record); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(fixture.root, fixture.journalPath), filepath.Join(fixture.root, "saved.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(fixture.root, "saved.json"), filepath.Join(fixture.root, fixture.journalPath)); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(fixture.root, fixture.journalPath); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("Load through symlinked journal error = %v", err)
	}
}

func TestWriteJSONValidatesRecord(t *testing.T) {
	fixture := newFixture(t)
	var output bytes.Buffer
	if err := WriteJSON(&output, fixture.record); err != nil {
		t.Fatal(err)
	}
	loaded, err := decode("writer output", output.Bytes())
	if err != nil || loaded.State != StatePrepared {
		t.Fatalf("decode WriteJSON result = state %q err %v", loaded.State, err)
	}
}

type fixture struct {
	root        string
	journalPath string
	record      Record
	baseTime    time.Time
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	root := t.TempDir()
	var err error
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{"work", "bin", ".ingen", filepath.Join(".ingen", "artifacts"), filepath.Join(".ingen", "artifacts", "sessions")} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "receipt.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(root, "bin", "sentinel")
	executableBytes := []byte("test sentinel executable bytes")
	if err := os.WriteFile(executable, executableBytes, 0o700); err != nil {
		t.Fatal(err)
	}
	executableSHA := sha256.Sum256(executableBytes)
	manifestSHA := sha256.Sum256([]byte("workspace manifest"))
	intent := Intent{
		SessionID:                "session-test",
		RunID:                    "run-test",
		WorkspaceID:              "sentinel-workspace",
		WorkspaceVersion:         4,
		WorkspaceManifestSHA256:  hex.EncodeToString(manifestSHA[:]),
		RoleID:                   "implementation",
		RoleKind:                 "builder",
		Workdir:                  "work",
		ReceiptPath:              "receipt.json",
		Argv:                     []string{"/bin/echo", "hello"},
		StdoutPath:               filepath.ToSlash(filepath.Join(".ingen", "artifacts", "sessions", "session.stdout")),
		StderrPath:               filepath.ToSlash(filepath.Join(".ingen", "artifacts", "sessions", "session.stderr")),
		HerdrSocket:              "unix:///tmp/herdr.sock",
		SentinelExecutable:       executable,
		SentinelExecutableSHA256: hex.EncodeToString(executableSHA[:]),
	}
	baseTime := time.Now().UTC().Add(time.Minute).Truncate(time.Second)
	record, err := New(intent, baseTime)
	if err != nil {
		t.Fatal(err)
	}
	return fixture{
		root:        root,
		journalPath: filepath.ToSlash(filepath.Join(".ingen", "artifacts", "sessions", "native.json")),
		record:      record,
		baseTime:    baseTime,
	}
}

func (f fixture) event(id, kind, state string, seconds int) Event {
	return Event{
		ID:    id,
		Kind:  kind,
		At:    f.baseTime.Add(time.Duration(seconds) * time.Second),
		State: state,
	}
}
