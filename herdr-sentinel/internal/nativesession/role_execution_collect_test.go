package nativesession

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"ingen/herdr-sentinel/internal/nativejournal"
	"ingen/herdr-sentinel/internal/roleexec"
	"ingen/herdr-sentinel/internal/run"
)

func TestCollectAttachesAndReplaysContainedRoleEvidence(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("role execution requires the Darwin host sandbox")
	}
	root, receiptPath, record, report := prepareAndRunNativeRole(t)
	journalPath := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")
	started := claimAndStartForTest(t, root, journalPath, record)
	markRoleLaunchedForTest(t, root, receiptPath, started)
	zero := 0
	terminal, err := appendWrapperTerminalForTest(t, root, journalPath, started, nativejournal.StateCompleted, &zero)
	if err != nil {
		t.Fatal(err)
	}
	legacyBase := seedLegacyNativeCollectionForTest(t, root, receiptPath, terminal, terminal)
	for i := 0; i < 2; i++ {
		if _, err := Collect(root, journalPath, receiptPath); err != nil {
			t.Fatalf("Collect attempt %d: %v", i+1, err)
		}
	}
	receipt, err := run.LoadFile(filepath.Join(root, receiptPath))
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "running" {
		t.Fatalf("inner successful role changed lifecycle status to %q", receipt.Status)
	}
	baseEvent, roleEvent := false, false
	for _, event := range receipt.Events {
		baseEvent = baseEvent || event.SourceID == "native-session-collected:"+record.Intent.SessionID
		roleEvent = roleEvent || event.SourceID == "role-execution-collected:"+report.ExecutionID
	}
	if !baseEvent || !roleEvent {
		t.Fatalf("receipt lacks stable base and contained evidence events: %+v", receipt.Events)
	}
	for _, event := range receipt.Events {
		if event.SourceID == legacyBase.SourceID && !reflect.DeepEqual(event, legacyBase) {
			t.Fatalf("upgrade rewrote the legacy base collection event: before=%+v after=%+v", legacyBase, event)
		}
	}
	if terminal.State != nativejournal.StateCompleted {
		t.Fatalf("terminal state = %q", terminal.State)
	}
}

func seedLegacyNativeCollectionForTest(t *testing.T, root, receiptPath string, record, terminal Record) run.Event {
	t.Helper()
	receiptFile := filepath.Join(root, receiptPath)
	var legacyEvent run.Event
	_, err := run.UpdateFile(receiptFile, func(receipt *run.Receipt) (bool, error) {
		journalPath := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")
		paths := []string{journalPath, record.Intent.StdoutPath, record.Intent.StderrPath}
		ids := []string{"native-session-" + record.Intent.SessionID, "native-session-" + record.Intent.SessionID + "-stdout", "native-session-" + record.Intent.SessionID + "-stderr"}
		kinds := []string{"sentinel-native-session", "session-stdout", "session-stderr"}
		for i := range paths {
			if _, err := receipt.RegisterFileArtifactUnderRoot(ids[i], record.Intent.RoleID, kinds[i], root, paths[i]); err != nil {
				return false, err
			}
		}
		legacyEvent = run.Event{
			SourceID:    "native-session-collected:" + record.Intent.SessionID,
			Type:        "role-completed",
			At:          time.Now().UTC().Format(time.RFC3339Nano),
			Role:        record.Intent.RoleID,
			Workspace:   record.Intent.Workdir,
			SessionID:   record.Intent.SessionID,
			Status:      "completed",
			ArtifactIDs: ids,
			Outcome:     nativeOutcome(record),
			Reason:      terminalEvent(terminal).Reason,
		}
		if err := receipt.AppendEvent(legacyEvent); err != nil {
			return false, err
		}
		legacyEvent = receipt.Events[len(receipt.Events)-1]
		return true, nil
	})
	if err != nil {
		t.Fatalf("seed prior native collection: %v", err)
	}
	return legacyEvent
}

func TestCollectRequiresReportForStartedCanceledRoleWrapper(t *testing.T) {
	root, receiptPath, record, _ := spawnMissingReportRole(t)
	journalPath := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")
	started := claimAndStartForTest(t, root, journalPath, record)
	markRoleLaunchedForTest(t, root, receiptPath, started)
	cancelCode := 130
	canceled, err := appendWrapperTerminalForTest(t, root, journalPath, started, nativejournal.StateCanceled, &cancelCode)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Collect(root, journalPath, receiptPath); err == nil || !strings.Contains(err.Error(), "role-execution evidence") {
		t.Fatalf("Collect without the started wrapper's report = %v", err)
	}
	if canceled.State != nativejournal.StateCanceled {
		t.Fatalf("state = %q", canceled.State)
	}
	if _, err := os.Stat(filepath.Join(root, receiptPath)); err != nil {
		t.Fatal(err)
	}
}

func TestCollectRejectsTamperedInnerCaptureAndKeepsReceipt(t *testing.T) {
	root, receiptPath, record, report := prepareAndRunNativeRole(t)
	journalPath := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")
	started := claimAndStartForTest(t, root, journalPath, record)
	markRoleLaunchedForTest(t, root, receiptPath, started)
	zero := 0
	if _, err := appendWrapperTerminalForTest(t, root, journalPath, started, nativejournal.StateCompleted, &zero); err != nil {
		t.Fatal(err)
	}
	capture := filepath.Join(root, filepath.FromSlash(report.StdoutPath))
	if err := os.WriteFile(capture, []byte("changed capture"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(root, receiptPath))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Collect(root, journalPath, receiptPath); err == nil {
		t.Fatal("Collect accepted tampered inner capture")
	}
	after, err := os.ReadFile(filepath.Join(root, receiptPath))
	if err != nil || string(before) != string(after) {
		t.Fatalf("receipt changed after inner capture tamper: %v", err)
	}
}

func appendWrapperTerminalForTest(t *testing.T, root, journalPath string, started Record, state string, exitCode *int) (Record, error) {
	t.Helper()
	stdout, stderr := []byte("wrapper stdout"), []byte("wrapper stderr")
	stdoutPath, stderrPath := filepath.Join(root, started.Intent.StdoutPath), filepath.Join(root, started.Intent.StderrPath)
	if err := os.WriteFile(stdoutPath, stdout, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stderrPath, stderr, 0o600); err != nil {
		t.Fatal(err)
	}
	record, _, err := appendEvent(root, journalPath, "wrapper-terminal:"+started.Intent.SessionID, state, lastHost(started), "wrapper terminal", exitCode, nativejournal.KindWrapperTerminal, roleDigest(stdout), roleDigest(stderr))
	return record, err
}

func markRoleLaunchedForTest(t *testing.T, root, receiptPath string, started Record) {
	t.Helper()
	if err := recordRoleLaunched(root, filepath.Join(root, receiptPath), started); err != nil {
		t.Fatal(err)
	}
}

func roleExitCode(exitCode *int) any {
	if exitCode == nil {
		return nil
	}
	return *exitCode
}

func TestCollectRejectsMismatchedNativePolicyPin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("role execution requires the Darwin host sandbox")
	}
	root, receiptPath, record, _ := prepareAndRunNativeRole(t, testRoleDigest("different-policy"))
	journalPath := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")
	started := claimAndStartForTest(t, root, journalPath, record)
	markRoleLaunchedForTest(t, root, receiptPath, started)
	zero := 0
	if _, err := appendWrapperTerminalForTest(t, root, journalPath, started, nativejournal.StateCompleted, &zero); err != nil {
		t.Fatal(err)
	}
	if _, err := Collect(root, journalPath, receiptPath); err == nil || !strings.Contains(err.Error(), "exact manifest, policy, or executable pins") {
		t.Fatalf("Collect accepted a role report with a mismatched argv policy pin: %v", err)
	}
}

func prepareAndRunNativeRole(t *testing.T, nativePolicyPin ...string) (string, string, Record, roleexec.Report) {
	return prepareAndRunNativeRoleCommand(t, context.Background(), []string{"/bin/echo", "inner role"}, nativePolicyPin...)
}

func TestFirstFailedRoleCollectionCommitsFailedReceipt(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("role execution requires the Darwin host sandbox")
	}
	root, receiptPath, record, report := prepareAndRunNativeRoleCommand(t, context.Background(), []string{"/usr/bin/false"})
	if report.Status != "failed" || report.ExitCode == nil || *report.ExitCode == 0 {
		t.Fatalf("inner report = status %q exit %v, want a nonzero failed outcome", report.Status, roleExitCode(report.ExitCode))
	}
	collectFirstRoleOutcome(t, root, receiptPath, record, nativejournal.StateFailed, report.ExitCode)
	receipt, err := run.LoadFile(filepath.Join(root, receiptPath))
	if err != nil || receipt.Status != "failed" {
		t.Fatalf("failed role receipt status = %q, %v", receipt.Status, err)
	}
}

func TestFirstCanceledRoleCollectionCommitsNonpassingReceipt(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("role execution requires the Darwin host sandbox")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	root, receiptPath, record, report := prepareAndRunNativeRoleCommand(t, ctx, []string{"/bin/sleep", "10"})
	if report.Status != "canceled" && report.Status != "indeterminate" {
		t.Fatalf("inner report status = %q, want canceled or indeterminate", report.Status)
	}
	cancelCode := 130
	collectFirstRoleOutcome(t, root, receiptPath, record, nativejournal.StateCanceled, &cancelCode)
	receipt, err := run.LoadFile(filepath.Join(root, receiptPath))
	if err != nil || receipt.Status != "failed" {
		t.Fatalf("canceled role receipt status = %q, %v", receipt.Status, err)
	}
}

func collectFirstRoleOutcome(t *testing.T, root, receiptPath string, record Record, state string, code *int) {
	t.Helper()
	journalPath := filepath.Join(ArtifactDirectory, record.Intent.SessionID+".json")
	started := claimAndStartForTest(t, root, journalPath, record)
	markRoleLaunchedForTest(t, root, receiptPath, started)
	if _, err := appendWrapperTerminalForTest(t, root, journalPath, started, state, code); err != nil {
		t.Fatal(err)
	}
	if _, err := Collect(root, journalPath, receiptPath); err != nil {
		t.Fatalf("first collection of %s role: %v", state, err)
	}
}

func prepareAndRunNativeRoleCommand(t *testing.T, ctx context.Context, command []string, nativePolicyPin ...string) (string, string, Record, roleexec.Report) {
	t.Helper()
	root, receiptPath := nativeFixture(t)
	root, err := absoluteRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	request := roleexec.Request{
		Root: root, WorkspacePath: ".ingen/workspace.yaml", ReceiptPath: receiptPath,
		RoleID: "contract-author", ExecutionID: "native-role-execution-1", Command: command,
		ProtectedPaths: []string{receiptPath},
		PolicyPath:     ".ingen/artifacts/role-executions/native-role-execution-1.policy.json",
	}
	prepared, err := roleexec.Prepare(request)
	if err != nil {
		t.Fatal(err)
	}
	request.ExpectedManifestSHA256 = prepared.ManifestSHA256
	request.ExpectedPolicySHA256 = prepared.Policy.SHA256
	request.ExpectedExecutableSHA256 = prepared.ExecutableSHA256
	argvPolicyPin := request.ExpectedPolicySHA256
	if len(nativePolicyPin) > 0 {
		argvPolicyPin = nativePolicyPin[0]
	}
	if err := roleexec.PersistPolicy(root, request.PolicyPath, prepared.Policy); err != nil {
		t.Fatal(err)
	}
	argv := []string{
		executable, "role", "execute", "--root", root, "--workspace", request.WorkspacePath,
		"--receipt", receiptPath, "--role", request.RoleID, "--execution-id", request.ExecutionID,
		"--policy-path", request.PolicyPath, "--expected-manifest-sha256", request.ExpectedManifestSHA256,
		"--expected-policy-sha256", argvPolicyPin, "--expected-executable-sha256", request.ExpectedExecutableSHA256,
		"--protect-path", receiptPath, "--",
	}
	argv = append(argv, command...)
	host := &fakeHost{}
	record, _, err := Spawn(context.Background(), Request{
		Root: root, WorkspacePath: request.WorkspacePath, ReceiptPath: receiptPath,
		RoleID: request.RoleID, SocketPath: "/tmp/herdr.sock", SentinelExecutable: executable, Command: argv,
	}, host)
	if err != nil {
		t.Fatal(err)
	}
	report, _, err := roleexec.Execute(ctx, request)
	if err != nil {
		if strings.Contains(err.Error(), "Operation not permitted") || strings.Contains(err.Error(), "status 71") {
			t.Skipf("nested Darwin sandbox unavailable in this executor: %v", err)
		}
		if report.Status != "failed" && report.Status != "canceled" && report.Status != "indeterminate" {
			t.Fatalf("roleexec.Execute() error: %v", err)
		}
	}
	return root, receiptPath, record, report
}

func spawnMissingReportRole(t *testing.T) (string, string, Record, *fakeHost) {
	t.Helper()
	root, receiptPath := nativeFixture(t)
	root, err := absoluteRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	manifestSHA, err := hashUnderRoot(root, ".ingen/workspace.yaml")
	if err != nil {
		t.Fatal(err)
	}
	argv := []string{executable, "role", "execute", "--root", root, "--workspace", ".ingen/workspace.yaml", "--receipt", receiptPath,
		"--role", "contract-author", "--execution-id", "missing-report", "--policy-path", ".ingen/artifacts/role-executions/missing-report.policy.json",
		"--expected-manifest-sha256", manifestSHA, "--expected-policy-sha256", testRoleDigest("policy"),
		"--expected-executable-sha256", testRoleDigest("executable"), "--", "/bin/true"}
	host := &fakeHost{}
	record, _, err := Spawn(context.Background(), Request{Root: root, WorkspacePath: ".ingen/workspace.yaml", ReceiptPath: receiptPath, RoleID: "contract-author", SocketPath: "/tmp/herdr.sock", SentinelExecutable: executable, Command: argv}, host)
	if err != nil {
		t.Fatal(err)
	}
	return root, receiptPath, record, host
}
