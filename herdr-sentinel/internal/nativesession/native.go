// Package nativesession coordinates Sentinel-owned sessions in a running
// Herdr host. It records intent and lifecycle locally; it does not enforce
// declared filesystem capabilities or treat host UI status as a verdict.
package nativesession

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"ingen/core/ciresult"
	"ingen/herdr-sentinel/internal/capability"
	"ingen/herdr-sentinel/internal/herdrclient"
	"ingen/herdr-sentinel/internal/nativejournal"
	"ingen/herdr-sentinel/internal/run"
)

const ArtifactDirectory = ".ingen/artifacts/native-sessions"

const (
	SupportedHerdrVersion  = "0.9.3"
	SupportedHerdrProtocol = 22
)

type Record = nativejournal.Record

// HostClient is the narrow Herdr surface needed by native sessions. It permits
// fake clients in tests without weakening the production socket binding.
type HostClient interface {
	Ping(context.Context) (herdrclient.ServerInfo, error)
	CreateWorkspace(context.Context, string, string) (herdrclient.Binding, error)
	ListWorkspaces(context.Context, string, string) ([]herdrclient.Binding, error)
	Pane(context.Context, string) (herdrclient.Pane, error)
	Run(context.Context, string, string) error
	Interrupt(context.Context, string) error
	CloseWorkspace(context.Context, string) error
}

type Request struct {
	Root               string
	WorkspacePath      string
	ReceiptPath        string
	RoleID             string
	SocketPath         string
	SentinelExecutable string
	Command            []string
	Now                time.Time
}

// Spawn commits the launch intent before making any Herdr workspace. It then
// persists the exact host binding and dispatch reservation before sending a
// shell-quoted Sentinel wrapper command. The original argv remains in the
// journal and never enters shell text.
func Spawn(ctx context.Context, request Request, host HostClient) (Record, string, error) {
	if ctx == nil {
		return Record{}, "", fmt.Errorf("native Sentinel session context is required")
	}
	if host == nil {
		return Record{}, "", fmt.Errorf("native Sentinel session Herdr client is required")
	}
	if len(request.Command) == 0 || strings.TrimSpace(request.Command[0]) == "" {
		return Record{}, "", fmt.Errorf("native Sentinel session command is required")
	}
	for _, arg := range request.Command {
		if strings.ContainsRune(arg, '\x00') {
			return Record{}, "", fmt.Errorf("native Sentinel session command arguments must not contain NUL")
		}
	}
	root, err := absoluteRoot(request.Root)
	if err != nil {
		return Record{}, "", err
	}
	workspacePath, err := relativePath("workspace", request.WorkspacePath)
	if err != nil {
		return Record{}, "", err
	}
	receiptPath, err := relativePath("receipt", request.ReceiptPath)
	if err != nil {
		return Record{}, "", err
	}
	if strings.TrimSpace(request.SocketPath) == "" || !filepath.IsAbs(request.SocketPath) || filepath.Clean(request.SocketPath) != request.SocketPath {
		return Record{}, "", fmt.Errorf("native Sentinel session requires an explicit clean absolute Herdr socket path")
	}
	if strings.TrimSpace(request.SentinelExecutable) == "" {
		return Record{}, "", fmt.Errorf("native Sentinel session requires the Sentinel executable path")
	}
	server, err := host.Ping(ctx)
	if err != nil {
		return Record{}, "", fmt.Errorf("connect to Herdr: %w", err)
	}
	if server.Version != SupportedHerdrVersion || server.Protocol != SupportedHerdrProtocol {
		return Record{}, "", fmt.Errorf("unsupported Herdr host %q protocol %d; Sentinel native sessions require Herdr %s protocol %d", server.Version, server.Protocol, SupportedHerdrVersion, SupportedHerdrProtocol)
	}

	plan, err := capability.FromFileUnderRoot(root, workspacePath)
	if err != nil {
		return Record{}, "", err
	}
	role, err := findRole(plan, request.RoleID)
	if err != nil {
		return Record{}, "", err
	}
	receiptFile, err := run.ResolveFileRefUnderRoot(root, ciresult.FileRef{Path: receiptPath})
	if err != nil {
		return Record{}, "", fmt.Errorf("resolve Sentinel receipt: %w", err)
	}
	receipt, err := run.LoadFile(receiptFile)
	if err != nil {
		return Record{}, "", fmt.Errorf("load Sentinel receipt: %w", err)
	}
	if receipt.Workspace.ID != plan.Workspace.ID || receipt.Workspace.Version != plan.Workspace.Version || receipt.Workspace.File != plan.Workspace.Manifest {
		return Record{}, "", fmt.Errorf("Sentinel receipt does not match the current capability plan workspace identity")
	}
	if receipt.Status == "completed" || receipt.Status == "failed" || receipt.Status == "blocked" || receipt.Status == "cleaned" {
		return Record{}, "", fmt.Errorf("cannot spawn a native session from terminal receipt status %q", receipt.Status)
	}

	roleWorkdir, err := filepath.Abs(filepath.Join(root, role.Workspace))
	if err != nil {
		return Record{}, "", fmt.Errorf("resolve role workspace: %w", err)
	}
	if err := run.ValidateDirectoryPathUnderRoot(root, role.Workspace); err != nil {
		return Record{}, "", err
	}
	executable, executableHash, err := canonicalExecutable(request.SentinelExecutable)
	if err != nil {
		return Record{}, "", err
	}

	sessionID, err := newSessionID()
	if err != nil {
		return Record{}, "", err
	}
	recordPath := filepath.Join(ArtifactDirectory, sessionID+".json")
	stdoutPath := filepath.Join(ArtifactDirectory, sessionID+".stdout.log")
	stderrPath := filepath.Join(ArtifactDirectory, sessionID+".stderr.log")
	for _, outputPath := range []string{recordPath, stdoutPath, stderrPath} {
		if err := run.ValidatePathUnderRoot(root, outputPath); err != nil {
			return Record{}, "", err
		}
		if _, err := os.Lstat(filepath.Join(root, outputPath)); err == nil {
			return Record{}, "", fmt.Errorf("native Sentinel session output %s already exists", outputPath)
		} else if !os.IsNotExist(err) {
			return Record{}, "", fmt.Errorf("inspect native session output %s: %w", outputPath, err)
		}
	}
	if err := run.ValidateDirectoryPathUnderRoot(root, ArtifactDirectory); err != nil {
		return Record{}, "", err
	}
	if err := os.MkdirAll(filepath.Join(root, ArtifactDirectory), 0o755); err != nil {
		return Record{}, "", fmt.Errorf("create native session artifact directory: %w", err)
	}

	now := request.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	intent := nativejournal.Intent{
		SessionID:                sessionID,
		RunID:                    receipt.RunID,
		WorkspaceID:              plan.Workspace.ID,
		WorkspaceVersion:         plan.Workspace.Version,
		WorkspaceManifestSHA256:  plan.Workspace.Manifest.SHA256,
		RoleID:                   role.ID,
		RoleKind:                 role.Kind,
		Workdir:                  filepath.Clean(role.Workspace),
		ReceiptPath:              receiptPath,
		Argv:                     append([]string(nil), request.Command...),
		StdoutPath:               stdoutPath,
		StderrPath:               stderrPath,
		HerdrSocket:              request.SocketPath,
		SentinelExecutable:       executable,
		SentinelExecutableSHA256: executableHash,
	}
	journal, err := nativejournal.New(intent, now)
	if err != nil {
		return Record{}, "", err
	}
	if err := nativejournal.Create(root, recordPath, journal); err != nil {
		return Record{}, "", fmt.Errorf("commit native session intent: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(root, role.Workspace), 0o755); err != nil {
		journal, _, markErr := appendEvent(root, recordPath, "role-workspace-failed:"+sessionID, nativejournal.StateFailed, nil, "create native role workspace: "+safeReason(err.Error()), nil, "provider-observation")
		if markErr != nil {
			return Record{}, recordPath, fmt.Errorf("create native role workspace: %v; record failure: %w", err, markErr)
		}
		return journal, recordPath, fmt.Errorf("create native role workspace: %w", err)
	}

	binding, err := host.CreateWorkspace(ctx, roleWorkdir, sessionID)
	if err != nil {
		var delivery *herdrclient.DeliveryError
		if errors.As(err, &delivery) && delivery.MayHaveApplied {
			matches, listErr := host.ListWorkspaces(ctx, roleWorkdir, sessionID)
			if listErr == nil && len(matches) == 1 && validBinding(matches[0], roleWorkdir, sessionID) {
				binding = matches[0]
				journal, _, err = appendEvent(root, recordPath, "host-create-reconciled", "indeterminate", hostIdentity(binding, plan.Workspace.ID), "workspace.create response was lost; recovery will not dispatch automatically", nil, "provider-observation")
				if err != nil {
					return Record{}, recordPath, fmt.Errorf("record uncertain Herdr workspace creation: %w", err)
				}
				return journal, recordPath, fmt.Errorf("Herdr workspace creation may have succeeded; native session is indeterminate and was not dispatched")
			}
			journal, _, markErr := appendEvent(root, recordPath, "host-create-indeterminate", "indeterminate", nil, "workspace.create response was lost and exact workspace identity could not be reconciled", nil, "provider-observation")
			if markErr != nil {
				return Record{}, recordPath, fmt.Errorf("workspace creation was uncertain (%v); journal recovery failed: %w", err, markErr)
			}
			return journal, recordPath, fmt.Errorf("Herdr workspace creation may have succeeded; exact identity could not be reconciled; run native-recover (create error: %v; list error: %v)", err, listErr)
		}
		journal, _, markErr := appendEvent(root, recordPath, "host-create-failed", "failed", nil, "Herdr workspace creation failed: "+safeReason(err.Error()), nil, "provider-observation")
		if markErr != nil {
			return Record{}, recordPath, fmt.Errorf("Herdr workspace creation failed (%v); journal update failed: %w", err, markErr)
		}
		return journal, recordPath, fmt.Errorf("create Herdr workspace: %w", err)
	}
	if !validBinding(binding, roleWorkdir, sessionID) {
		journal, _, markErr := appendEvent(root, recordPath, "host-create-invalid", "indeterminate", nil, "Herdr workspace response omitted or mismatched exact session identity", nil, "provider-observation")
		if markErr != nil {
			return Record{}, recordPath, fmt.Errorf("invalid Herdr workspace identity; journal update failed: %w", markErr)
		}
		return journal, recordPath, fmt.Errorf("Herdr workspace response has incomplete or mismatched session identity; run native-recover")
	}
	identity := hostIdentity(binding, plan.Workspace.ID)
	journal, _, err = appendEvent(root, recordPath, "dispatch-reserved", nativejournal.StateDispatching, identity, "", nil, "provider-dispatch")
	if err != nil {
		return Record{}, recordPath, fmt.Errorf("persist native dispatch reservation: %w", err)
	}
	commandText := wrapperCommand(executable, root, recordPath)
	if err := host.Run(ctx, binding.PaneID, commandText); err != nil {
		var delivery *herdrclient.DeliveryError
		if errors.As(err, &delivery) && delivery.MayHaveApplied {
			return journal, recordPath, fmt.Errorf("Herdr may have accepted the native wrapper; state remains dispatching; run native-recover instead of resending: %w", err)
		}
		journal, _, markErr := appendEvent(root, recordPath, "dispatch-rejected", nativejournal.StateFailed, identity, "Herdr rejected wrapper dispatch: "+safeReason(err.Error()), nil, "provider-dispatch")
		if markErr != nil {
			return Record{}, recordPath, fmt.Errorf("wrapper dispatch failed (%v); journal update failed: %w", err, markErr)
		}
		return journal, recordPath, fmt.Errorf("Herdr rejected wrapper dispatch: %w", err)
	}
	// Record the provider's acknowledgement as a separate observation. The
	// journal recognizes this kind without regressing a wrapper that already
	// advanced to running or a terminal state.
	journal, _, err = appendEvent(root, recordPath, "dispatch-ack", nativejournal.StateSubmitted, identity, "", nil, "host-dispatch-ack")
	if err != nil {
		return Record{}, recordPath, fmt.Errorf("wrapper sent but dispatch acknowledgement could not be journaled; run native-recover: %w", err)
	}
	return journal, recordPath, nil
}

func Load(root, path string) (Record, error) {
	root, err := absoluteRoot(root)
	if err != nil {
		return Record{}, err
	}
	recordPath, err := nativeRecordPath(path)
	if err != nil {
		return Record{}, err
	}
	record, err := nativejournal.Load(root, recordPath)
	if err != nil {
		return Record{}, err
	}
	if err := validateRecordLocation(recordPath, record); err != nil {
		return Record{}, err
	}
	return record, nil
}

// Execute runs the immutable argv recorded in the journal. The journal's
// locked claim is the only authorization to launch; duplicate wrappers exit
// without touching the process or output paths.
func Execute(root, path string) error {
	root, err := absoluteRoot(root)
	if err != nil {
		return err
	}
	recordPath, err := nativeRecordPath(path)
	if err != nil {
		return err
	}
	record, err := nativejournal.Load(root, recordPath)
	if err != nil {
		return err
	}
	if err := validateRecordLocation(recordPath, record); err != nil {
		return err
	}
	if record.State != nativejournal.StateDispatching {
		return fmt.Errorf("native wrapper is not authorized in state %q; duplicate execution is refused", record.State)
	}
	if err := validateRecordLocation(recordPath, record); err != nil {
		return err
	}
	if err := validateExecutable(record.Intent); err != nil {
		markTerminalFailure(root, recordPath, record.Intent.SessionID, err.Error())
		return err
	}
	receiptFile, err := run.ResolveFileRefUnderRoot(root, ciresult.FileRef{Path: record.Intent.ReceiptPath})
	if err != nil {
		markTerminalFailure(root, recordPath, record.Intent.SessionID, err.Error())
		return err
	}
	if _, err := receiptMatchesIntent(root, receiptFile, record); err != nil {
		markTerminalFailure(root, recordPath, record.Intent.SessionID, err.Error())
		return fmt.Errorf("validate Sentinel receipt/workspace identity before native execution: %w", err)
	}
	workdir, err := run.ResolveDirectoryUnderRoot(root, record.Intent.Workdir)
	if err != nil {
		markTerminalFailure(root, recordPath, record.Intent.SessionID, err.Error())
		return fmt.Errorf("validate native role workdir: %w", err)
	}
	runChild, stopSignals := prepareCommandRunner()
	defer stopSignals()
	claimed, ok, err := nativejournal.ClaimExecution(root, recordPath, record.Intent.SessionID, nativejournal.Event{
		ID:    "wrapper-claim:" + record.Intent.SessionID,
		Kind:  "wrapper-claim",
		At:    nextTimestamp(record),
		State: nativejournal.StateSubmitted,
		Host:  lastHost(record),
	})
	if err != nil {
		if latest, loadErr := nativejournal.Load(root, recordPath); loadErr == nil && latest.State == nativejournal.StateSubmitted {
			_, _, _ = appendEvent(root, recordPath, "wrapper-claim-uncertain:"+record.Intent.SessionID, nativejournal.StateIndeterminate, lastHost(latest), "execution claim publication outcome was uncertain; wrapper did not start child", nil, "wrapper-recovery")
		}
		return fmt.Errorf("claim native wrapper execution: %w", err)
	}
	if !ok {
		return fmt.Errorf("native wrapper execution was already claimed; refusing duplicate execution")
	}
	stdout, err := createExclusiveUnderRoot(root, record.Intent.StdoutPath)
	if err != nil {
		markTerminalFailure(root, recordPath, record.Intent.SessionID, "create native stdout exclusively: "+err.Error())
		return fmt.Errorf("create native stdout exclusively: %w", err)
	}
	stderr, err := createExclusiveUnderRoot(root, record.Intent.StderrPath)
	if err != nil {
		_ = stdout.Close()
		_ = removeUnderRoot(root, record.Intent.StdoutPath)
		markTerminalFailure(root, recordPath, record.Intent.SessionID, "create native stderr exclusively: "+err.Error())
		return fmt.Errorf("create native stderr exclusively: %w", err)
	}
	startRecord, startOK, err := nativejournal.StartExecution(root, recordPath, record.Intent.SessionID, nativejournal.Event{
		ID: "wrapper-running:" + record.Intent.SessionID, Kind: "wrapper-started", At: nextTimestamp(claimed), State: nativejournal.StateRunning, Host: lastHost(claimed),
	})
	if err != nil || !startOK {
		_ = stdout.Close()
		_ = stderr.Close()
		_ = removeUnderRoot(root, record.Intent.StdoutPath)
		_ = removeUnderRoot(root, record.Intent.StderrPath)
		if err != nil {
			_, _, _ = appendEvent(root, recordPath, "wrapper-start-uncertain:"+record.Intent.SessionID, nativejournal.StateIndeterminate, lastHost(claimed), "could not durably record wrapper start; child was not started", nil, "wrapper-recovery")
			return fmt.Errorf("record native wrapper start: %w", err)
		}
		return fmt.Errorf("native wrapper start was not authorized in state %q; child was not started", startRecord.State)
	}
	if err := recordRoleLaunched(root, receiptFile, startRecord); err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		_ = removeUnderRoot(root, record.Intent.StdoutPath)
		_ = removeUnderRoot(root, record.Intent.StderrPath)
		_, _, _ = appendEvent(root, recordPath, "wrapper-receipt-rejected:"+record.Intent.SessionID, nativejournal.StateFailed, lastHost(startRecord), "Sentinel receipt rejected wrapper launch: "+safeReason(err.Error()), nil, "wrapper-setup-failed")
		return fmt.Errorf("record native role launch: %w", err)
	}
	command := osexec.Command(record.Intent.Argv[0], record.Intent.Argv[1:]...)
	command.Dir = workdir
	command.Stdin = nil
	command.Stdout = stdout
	command.Stderr = stderr
	command.Env = append(os.Environ(),
		"INGEN_SENTINEL_RUN_ID="+record.Intent.RunID,
		"INGEN_SENTINEL_WORKSPACE_ID="+record.Intent.WorkspaceID,
		"INGEN_SENTINEL_SESSION_ID="+record.Intent.SessionID,
		"INGEN_SENTINEL_ROLE_ID="+record.Intent.RoleID,
		"INGEN_SENTINEL_ROLE_KIND="+record.Intent.RoleKind,
		"INGEN_SENTINEL_CAPABILITY_ENFORCEMENT="+nativejournal.Enforcement,
	)
	processErr, interrupted, childStarted, interruptReason := runChild(command, func() bool {
		latest, loadErr := nativejournal.Load(root, recordPath)
		return loadErr == nil && hasCancellationRequest(latest)
	})
	syncOutErr := stdout.Sync()
	syncErr := stderr.Sync()
	closeOutErr := stdout.Close()
	closeErr := stderr.Close()
	captureErr := errors.Join(syncOutErr, syncErr, closeOutErr, closeErr)
	if !childStarted {
		_ = removeUnderRoot(root, record.Intent.StdoutPath)
		_ = removeUnderRoot(root, record.Intent.StderrPath)
		if interrupted {
			finished, loadErr := nativejournal.Load(root, recordPath)
			if loadErr != nil {
				return fmt.Errorf("native child was canceled before start; reload journal: %w", loadErr)
			}
			reason := "cancellation was received before native child process start"
			if interruptReason != "" {
				reason += ": " + interruptReason
			}
			_, _, appendErr := appendEvent(root, recordPath, "wrapper-canceled-before-start:"+record.Intent.SessionID, nativejournal.StateCanceled, lastHost(finished), reason, nil, "wrapper-canceled-before-start")
			if appendErr != nil {
				return fmt.Errorf("native child was canceled before start; record cancellation: %w", appendErr)
			}
			return fmt.Errorf("native child canceled before start")
		}
		setupErr := errors.Join(processErr, captureErr)
		markTerminalFailure(root, recordPath, record.Intent.SessionID, "start native child process: "+safeReason(fmt.Sprint(setupErr)))
		return fmt.Errorf("start native child process: %w", setupErr)
	}
	if captureErr != nil {
		reason := fmt.Sprintf("native child output capture could not be synced or closed: %v", captureErr)
		if code, known := nativeChildExitCode(processErr); known {
			reason += fmt.Sprintf("; child exit code %d", code)
		}
		if interruptReason != "" {
			reason += "; interruption: " + interruptReason
		}
		markIndeterminate(root, recordPath, record.Intent.SessionID, reason)
		return fmt.Errorf("native output capture failed; session outcome is indeterminate: %w", errors.Join(processErr, captureErr))
	}
	stdoutHash, hashErr := hashUnderRoot(root, record.Intent.StdoutPath)
	if hashErr != nil {
		markIndeterminate(root, recordPath, record.Intent.SessionID, fmt.Sprintf("could not hash native stdout: %v", hashErr))
		return fmt.Errorf("hash native stdout: %w", hashErr)
	}
	stderrHash, hashErr := hashUnderRoot(root, record.Intent.StderrPath)
	if hashErr != nil {
		markIndeterminate(root, recordPath, record.Intent.SessionID, fmt.Sprintf("could not hash native stderr: %v", hashErr))
		return fmt.Errorf("hash native stderr: %w", hashErr)
	}
	finished, err := nativejournal.Load(root, recordPath)
	if err != nil {
		return err
	}
	exitCode, knownExit := nativeChildExitCode(processErr)
	if !knownExit || (!interrupted && processErr != nil && exitCode == 0) {
		reason := "native child process outcome could not be determined"
		if processErr != nil {
			reason += ": " + processErr.Error()
		}
		markIndeterminate(root, recordPath, record.Intent.SessionID, reason)
		return fmt.Errorf("native child outcome is indeterminate: %v", processErr)
	}
	state := nativejournal.StateCompleted
	reason := ""
	if interrupted {
		state = nativejournal.StateCanceled
		reason = terminalReason(processErr, interruptReason)
	} else if exitCode != 0 {
		state = nativejournal.StateFailed
		reason = terminalReason(processErr, "")
	}
	_, _, err = appendEvent(root, recordPath, "wrapper-terminal:"+record.Intent.SessionID, state, lastHost(finished), reason, &exitCode, "wrapper-terminal", stdoutHash, stderrHash)
	if err != nil {
		return fmt.Errorf("record native wrapper terminal outcome: %w", err)
	}
	if processErr != nil {
		return processErr
	}
	return nil
}

func nativeChildExitCode(processErr error) (int, bool) {
	if processErr == nil {
		return 0, true
	}
	var exitErr *osexec.ExitError
	if errors.As(processErr, &exitErr) {
		return exitErr.ExitCode(), true
	}
	return 0, false
}

func terminalReason(processErr error, interruption string) string {
	parts := make([]string, 0, 2)
	if interruption != "" {
		parts = append(parts, "wrapper interruption: "+interruption)
	}
	if processErr != nil {
		parts = append(parts, "child process: "+processErr.Error())
	}
	return safeReason(strings.Join(parts, "; "))
}

func Collect(root, path, receiptPath string) (Record, error) {
	record, err := Load(root, path)
	if err != nil {
		return Record{}, err
	}
	if record.State != nativejournal.StateCompleted && record.State != nativejournal.StateFailed && record.State != nativejournal.StateCanceled {
		return record, fmt.Errorf("native session is not terminal (state %s); collect is unavailable", record.State)
	}
	root, err = absoluteRoot(root)
	if err != nil {
		return Record{}, err
	}
	requestedReceipt, err := relativePath("receipt", receiptPath)
	if err != nil {
		return Record{}, err
	}
	if requestedReceipt != record.Intent.ReceiptPath {
		return Record{}, fmt.Errorf("collection receipt path %q does not match native intent %q", requestedReceipt, record.Intent.ReceiptPath)
	}
	terminal := terminalEvent(record)
	if terminal == nil {
		return Record{}, fmt.Errorf("native session has no terminal journal event")
	}
	recordPath, err := nativeRecordPath(path)
	if err != nil {
		return Record{}, err
	}
	paths := []string{recordPath}
	kinds := []string{"sentinel-native-session"}
	artifactIDs := []string{"native-session-" + record.Intent.SessionID}
	recordHash, err := hashUnderRoot(root, recordPath)
	if err != nil {
		return Record{}, err
	}
	confirmed, err := Load(root, recordPath)
	if err != nil || !reflect.DeepEqual(record, confirmed) {
		return Record{}, fmt.Errorf("native journal changed during collection")
	}
	expectedHashes := []string{recordHash}
	if terminal.StdoutSHA256 != "" || terminal.StderrSHA256 != "" {
		if terminal.StdoutSHA256 == "" || terminal.StderrSHA256 == "" {
			return Record{}, fmt.Errorf("native session terminal event has incomplete output digests")
		}
		stdoutHash, hashErr := hashUnderRoot(root, record.Intent.StdoutPath)
		if hashErr != nil {
			return Record{}, hashErr
		}
		stderrHash, hashErr := hashUnderRoot(root, record.Intent.StderrPath)
		if hashErr != nil {
			return Record{}, hashErr
		}
		if stdoutHash != terminal.StdoutSHA256 || stderrHash != terminal.StderrSHA256 {
			return Record{}, fmt.Errorf("native session output digest mismatch; refusing collection")
		}
		paths = append(paths, record.Intent.StdoutPath, record.Intent.StderrPath)
		kinds = append(kinds, "session-stdout", "session-stderr")
		artifactIDs = append(artifactIDs, "native-session-"+record.Intent.SessionID+"-stdout", "native-session-"+record.Intent.SessionID+"-stderr")
		expectedHashes = append(expectedHashes, terminal.StdoutSHA256, terminal.StderrSHA256)
	}
	receiptFile, err := run.ResolveFileRefUnderRoot(root, ciresult.FileRef{Path: requestedReceipt})
	if err != nil {
		return Record{}, err
	}
	identity, err := receiptMatchesIntent(root, receiptFile, record)
	if err != nil {
		return Record{}, err
	}
	_, err = run.UpdateFile(receiptFile, func(loaded *run.Receipt) (bool, error) {
		if !sameReceiptIdentity(loaded, identity) {
			return false, fmt.Errorf("Sentinel receipt identity changed during native collection")
		}
		changed := false
		for i := range paths {
			added, addErr := loaded.RegisterFileArtifactUnderRoot(artifactIDs[i], record.Intent.RoleID, kinds[i], root, paths[i])
			if addErr != nil {
				return false, addErr
			}
			for _, artifact := range loaded.Artifacts {
				if artifact.ID == artifactIDs[i] {
					if artifact.Ref.SHA256 != expectedHashes[i] {
						return false, fmt.Errorf("native session artifact %s changed during collection", paths[i])
					}
					break
				}
			}
			changed = changed || added
		}
		sourceID := "native-session-collected:" + record.Intent.SessionID
		for _, event := range loaded.Events {
			if event.SourceID == sourceID {
				if event.Role != record.Intent.RoleID || event.SessionID != record.Intent.SessionID || event.Outcome != nativeOutcome(record) || !sameIDs(event.ArtifactIDs, artifactIDs) {
					return false, fmt.Errorf("native session collection source ID conflicts with existing receipt event")
				}
				return changed, nil
			}
		}
		if loaded.Status == "completed" || loaded.Status == "failed" || loaded.Status == "blocked" || loaded.Status == "cleaned" {
			return false, fmt.Errorf("cannot collect native session into terminal receipt status %q without an existing identical collection event", loaded.Status)
		}
		status := "completed"
		if record.State == nativejournal.StateFailed || record.State == nativejournal.StateCanceled {
			status = "failed"
		}
		at := time.Now().UTC().Format(time.RFC3339Nano)
		if err := loaded.AppendEvent(run.Event{
			SourceID:    sourceID,
			Type:        "role-completed",
			At:          at,
			Role:        record.Intent.RoleID,
			Workspace:   record.Intent.Workdir,
			SessionID:   record.Intent.SessionID,
			Status:      status,
			ArtifactIDs: artifactIDs,
			Outcome:     nativeOutcome(record),
			Reason:      terminal.Reason,
		}); err != nil {
			return false, err
		}
		if status == "failed" {
			if err := loaded.SetStatus("failed", time.Now().UTC()); err != nil {
				return false, err
			}
		}
		return true, nil
	})
	if err != nil {
		return Record{}, fmt.Errorf("collect native session artifacts into receipt: %w", err)
	}
	return record, nil
}

func Cancel(ctx context.Context, root, path string, host HostClient) (Record, error) {
	root, err := absoluteRoot(root)
	if err != nil {
		return Record{}, err
	}
	record, err := Load(root, path)
	if err != nil {
		return Record{}, err
	}
	if terminalState(record.State) {
		return record, nil
	}
	if record.State == nativejournal.StateCancelRequested {
		return record, fmt.Errorf("native cancellation is already requested; delivery is not confirmed; inspect native-status or recover; no second interrupt was sent")
	}
	if record.State == nativejournal.StateDispatching {
		recordPath, pathErr := nativeRecordPath(path)
		if pathErr != nil {
			return record, pathErr
		}
		identity := lastHost(record)
		updated, _, appendErr := appendEvent(root, recordPath, "cancel-requested:"+record.Intent.SessionID, nativejournal.StateCancelRequested, identity, "operator canceled before wrapper claim; child launch is blocked", nil, "provider-cancel")
		if appendErr != nil {
			return record, appendErr
		}
		latest, loadErr := nativejournal.Load(root, recordPath)
		if loadErr != nil {
			return updated, fmt.Errorf("prelaunch cancellation is recorded; reload journal before settlement: %w", loadErr)
		}
		if !hasWrapperClaim(latest) {
			settled, _, settleErr := appendEvent(root, recordPath, "canceled-before-claim:"+record.Intent.SessionID, nativejournal.StateCanceled, identity, "operator canceled before wrapper claim; no child process ran", nil, nativejournal.KindProviderCanceledBeforeClaim)
			if settleErr != nil {
				return latest, fmt.Errorf("prelaunch cancellation is recorded but could not be settled: %w", settleErr)
			}
			return settled, nil
		}
		// ClaimExecution won the journal lock just before the cancellation
		// reservation. The durable request blocks StartExecution if it has not
		// started yet; for an already claimed pane, verify exact identity before
		// sending its one interrupt.
		if host == nil {
			return latest, fmt.Errorf("native cancellation is recorded after wrapper claim; Herdr client is required to interrupt the owned pane")
		}
		if err := checkSupportedHost(ctx, host); err != nil {
			return latest, err
		}
		if _, err := verifiedPane(ctx, host, root, latest, identity); err != nil {
			return latest, err
		}
		if err := host.Interrupt(ctx, identity.PaneID); err != nil {
			return latest, fmt.Errorf("cancellation intent is recorded, but Herdr interrupt failed; recover before retrying: %w", err)
		}
		return nativejournal.Load(root, recordPath)
	}
	if record.State == nativejournal.StatePrepared || record.State == nativejournal.StateIndeterminate {
		return record, fmt.Errorf("native wrapper execution is not confirmed in state %q; no interrupt was sent to the unclaimed pane", record.State)
	}
	identity := lastHost(record)
	if identity == nil || identity.WorkspaceID == "" || identity.PaneID == "" || identity.TerminalID == "" {
		return record, fmt.Errorf("native session has no complete owned Herdr workspace/pane/terminal identity")
	}
	if host == nil {
		return record, fmt.Errorf("native session Herdr client is required")
	}
	if err := checkSupportedHost(ctx, host); err != nil {
		return record, err
	}
	if _, err := verifiedPane(ctx, host, root, record, identity); err != nil {
		return record, err
	}
	recordPath, err := nativeRecordPath(path)
	if err != nil {
		return record, err
	}
	updated, _, err := appendEvent(root, recordPath, "cancel-requested:"+record.Intent.SessionID, nativejournal.StateCancelRequested, identity, "operator requested cancellation", nil, "provider-cancel")
	if err != nil {
		return record, err
	}
	latest, err := nativejournal.Load(root, recordPath)
	if err != nil {
		return updated, fmt.Errorf("cancellation intent is recorded; read state before interrupt: %w", err)
	}
	if terminalState(latest.State) {
		return latest, nil
	}
	if err := host.Interrupt(ctx, identity.PaneID); err != nil {
		return updated, fmt.Errorf("cancellation intent is recorded, but Herdr interrupt failed; recover before retrying: %w", err)
	}
	return nativejournal.Load(root, recordPath)
}

func Recover(ctx context.Context, root, path string, host HostClient) (Record, error) {
	root, err := absoluteRoot(root)
	if err != nil {
		return Record{}, err
	}
	record, err := Load(root, path)
	if err != nil {
		return Record{}, err
	}
	if terminalState(record.State) {
		return record, nil
	}
	if host == nil {
		return record, fmt.Errorf("native session Herdr client is required for recovery")
	}
	if err := checkSupportedHost(ctx, host); err != nil {
		return record, err
	}
	recordPath, err := nativeRecordPath(path)
	if err != nil {
		return record, err
	}
	identity := lastHost(record)
	if identity == nil || identity.WorkspaceID == "" || identity.PaneID == "" || identity.TerminalID == "" {
		if record.State != nativejournal.StatePrepared && record.State != nativejournal.StateIndeterminate {
			return record, fmt.Errorf("native session host identity is missing in state %s; recovery will not create or dispatch a workspace", record.State)
		}
		cwd := filepath.Join(root, record.Intent.Workdir)
		matches, listErr := host.ListWorkspaces(ctx, cwd, record.Intent.SessionID)
		if listErr != nil {
			return record, fmt.Errorf("reconcile native workspace identity: %w", listErr)
		}
		if len(matches) != 1 || !validBinding(matches[0], cwd, record.Intent.SessionID) {
			updated, _, markErr := appendEvent(root, recordPath, "recovery-unbound:"+record.Intent.SessionID, nativejournal.StateIndeterminate, nil, "prepared session has no unique matching Herdr workspace; recovery did not dispatch", nil, "provider-recovery")
			if markErr != nil {
				return record, markErr
			}
			return updated, fmt.Errorf("prepared native session has %d exact Herdr workspace matches; no command was sent", len(matches))
		}
		identity = hostIdentity(matches[0], record.Intent.WorkspaceID)
		updated, _, appendErr := appendEvent(root, recordPath, "recovery-bound:"+record.Intent.SessionID, nativejournal.StateIndeterminate, identity, "reconciled an existing workspace after interruption; recovery will not dispatch", nil, "provider-recovery")
		if appendErr != nil {
			return record, appendErr
		}
		return updated, fmt.Errorf("native session was created before dispatch reservation; workspace identity was restored but no command was sent")
	}
	if _, err := verifiedPane(ctx, host, root, record, identity); err != nil {
		if record.State == nativejournal.StateDispatching || record.State == nativejournal.StateSubmitted || record.State == nativejournal.StateRunning || record.State == nativejournal.StateCancelRequested {
			updated, _, markErr := appendEvent(root, recordPath, "recovery-host-uncertain:"+record.Intent.SessionID, nativejournal.StateIndeterminate, identity, "Herdr no longer confirms the stored owned pane and terminal identity", nil, "provider-recovery")
			if markErr == nil {
				return updated, fmt.Errorf("native session identity could not be confirmed; state marked indeterminate: %w", err)
			}
		}
		return record, err
	}
	// Pane visibility or agent status does not prove child completion. Wrapper
	// journal events are authoritative for process start and terminal outcome.
	return record, nil
}

func WriteJSON(writer io.Writer, record Record) error { return nativejournal.WriteJSON(writer, record) }

func appendEvent(root, recordPath, id, state string, host *nativejournal.Host, reason string, exitCode *int, kind string, digests ...string) (Record, bool, error) {
	stdoutHash, stderrHash := "", ""
	if len(digests) > 0 {
		stdoutHash = digests[0]
	}
	if len(digests) > 1 {
		stderrHash = digests[1]
	}
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		current, err := nativejournal.Load(root, recordPath)
		if err != nil {
			return Record{}, false, err
		}
		event := nativejournal.Event{
			ID: id, Kind: kind, At: nextTimestamp(current), State: state, Host: host,
			Reason: reason, ExitCode: exitCode, StdoutSHA256: stdoutHash, StderrSHA256: stderrHash,
		}
		updated, changed, appendErr := nativejournal.Append(root, recordPath, event)
		if appendErr == nil {
			return updated, changed, nil
		}
		lastErr = appendErr
		latest, loadErr := nativejournal.Load(root, recordPath)
		if loadErr != nil {
			return Record{}, false, errors.Join(appendErr, loadErr)
		}
		for _, previous := range latest.Events {
			if previous.ID == id {
				if previous.Kind == kind && previous.State == state && sameHost(previous.Host, host) && previous.Reason == reason && equalIntPtr(previous.ExitCode, exitCode) && previous.StdoutSHA256 == stdoutHash && previous.StderrSHA256 == stderrHash {
					return latest, false, nil
				}
				return Record{}, false, fmt.Errorf("native journal event ID %q conflicts with concurrent content", id)
			}
		}
		if !strings.Contains(appendErr.Error(), "timestamp") {
			return Record{}, false, appendErr
		}
	}
	return Record{}, false, fmt.Errorf("append native journal event %q after concurrent updates: %w", id, lastErr)
}

func receiptMatchesIntent(root, receiptFile string, record Record) (*run.Receipt, error) {
	receipt, err := run.LoadFile(receiptFile)
	if err != nil {
		return nil, err
	}
	if err := validateReceipt(root, &receipt, record); err != nil {
		return nil, err
	}
	return &receipt, nil
}

func validateReceipt(root string, receipt *run.Receipt, record Record) error {
	if receipt == nil {
		return fmt.Errorf("Sentinel receipt is required")
	}
	if receipt.RunID != record.Intent.RunID || receipt.Workspace.ID != record.Intent.WorkspaceID || receipt.Workspace.Version != record.Intent.WorkspaceVersion || receipt.Workspace.File.SHA256 != record.Intent.WorkspaceManifestSHA256 {
		return fmt.Errorf("native session does not match the Sentinel receipt run/workspace identity")
	}
	manifestPath, err := run.ResolveFileRefUnderRoot(root, receipt.Workspace.File)
	if err != nil {
		return fmt.Errorf("resolve receipt workspace manifest: %w", err)
	}
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(manifest)
	if hex.EncodeToString(digest[:]) != record.Intent.WorkspaceManifestSHA256 {
		return fmt.Errorf("workspace manifest changed after native session intent was committed")
	}
	plan, err := capability.FromFileUnderRoot(root, receipt.Workspace.File.Path)
	if err != nil {
		return fmt.Errorf("validate capability plan for native session: %w", err)
	}
	if plan.Workspace.ID != record.Intent.WorkspaceID || plan.Workspace.Version != record.Intent.WorkspaceVersion || plan.Workspace.Manifest.SHA256 != record.Intent.WorkspaceManifestSHA256 {
		return fmt.Errorf("native session capability plan identity changed")
	}
	role, err := findRole(plan, record.Intent.RoleID)
	if err != nil || role.Kind != record.Intent.RoleKind || filepath.Clean(role.Workspace) != filepath.Clean(record.Intent.Workdir) {
		return fmt.Errorf("native session role identity no longer matches the workspace manifest")
	}
	return nil
}

func recordRoleLaunched(root, receiptFile string, record Record) error {
	_, err := run.UpdateFile(receiptFile, func(loaded *run.Receipt) (bool, error) {
		if err := validateReceipt(root, loaded, record); err != nil {
			return false, err
		}
		if loaded.Status == "completed" || loaded.Status == "failed" || loaded.Status == "blocked" || loaded.Status == "cleaned" {
			return false, fmt.Errorf("receipt reached terminal state %q before wrapper execution", loaded.Status)
		}
		sourceID := "native-session-launched:" + record.Intent.SessionID
		for _, event := range loaded.Events {
			if event.SourceID == sourceID {
				if event.Role != record.Intent.RoleID || event.SessionID != record.Intent.SessionID || event.Status != "running" {
					return false, fmt.Errorf("native role launch source ID conflicts with existing receipt event")
				}
				return false, nil
			}
		}
		at := time.Now().UTC()
		if loaded.Status == "created" {
			if err := loaded.SetStatus("running", at); err != nil {
				return false, err
			}
		}
		if err := loaded.AppendEvent(run.Event{
			SourceID:  sourceID,
			Type:      "role-launched",
			At:        at.Format(time.RFC3339Nano),
			Role:      record.Intent.RoleID,
			Workspace: record.Intent.Workdir,
			SessionID: record.Intent.SessionID,
			Status:    "running",
			Outcome:   "native-wrapper-claimed",
		}); err != nil {
			return false, err
		}
		return true, nil
	})
	return err
}

func sameReceiptIdentity(left, right *run.Receipt) bool {
	return left != nil && right != nil && left.RunID == right.RunID && left.Workspace == right.Workspace
}

func verifiedPane(ctx context.Context, host HostClient, root string, record Record, expected *nativejournal.Host) (herdrclient.Pane, error) {
	pane, err := host.Pane(ctx, expected.PaneID)
	if err != nil {
		return herdrclient.Pane{}, fmt.Errorf("read owned Herdr pane %q: %w", expected.PaneID, err)
	}
	if pane.WorkspaceID != expected.WorkspaceID || pane.PaneID != expected.PaneID || pane.TerminalID != expected.TerminalID || expected.SentinelWorkspaceID != record.Intent.WorkspaceID {
		return herdrclient.Pane{}, fmt.Errorf("Herdr pane identity does not match the native session's stored workspace/pane/terminal binding")
	}
	if pane.CWD == "" || filepath.Clean(pane.CWD) != filepath.Clean(filepath.Join(root, record.Intent.Workdir)) {
		return herdrclient.Pane{}, fmt.Errorf("Herdr pane working directory does not match the native role workspace")
	}
	return pane, nil
}

func checkSupportedHost(ctx context.Context, host HostClient) error {
	server, err := host.Ping(ctx)
	if err != nil {
		return fmt.Errorf("connect to Herdr: %w", err)
	}
	if server.Version != SupportedHerdrVersion || server.Protocol != SupportedHerdrProtocol {
		return fmt.Errorf("unsupported Herdr host %q protocol %d; Sentinel native sessions require Herdr %s protocol %d", server.Version, server.Protocol, SupportedHerdrVersion, SupportedHerdrProtocol)
	}
	return nil
}

func validBinding(binding herdrclient.Binding, cwd, label string) bool {
	return binding.WorkspaceID != "" && binding.TabID != "" && binding.PaneID != "" && binding.TerminalID != "" && binding.Label == label && filepath.Clean(binding.CWD) == filepath.Clean(cwd)
}

func hostIdentity(binding herdrclient.Binding, sentinelWorkspaceID string) *nativejournal.Host {
	return &nativejournal.Host{WorkspaceID: binding.WorkspaceID, SentinelWorkspaceID: sentinelWorkspaceID, PaneID: binding.PaneID, TerminalID: binding.TerminalID}
}

func lastHost(record Record) *nativejournal.Host {
	for index := len(record.Events) - 1; index >= 0; index-- {
		if record.Events[index].Host != nil {
			host := *record.Events[index].Host
			return &host
		}
	}
	return nil
}

func terminalEvent(record Record) *nativejournal.Event {
	for index := len(record.Events) - 1; index >= 0; index-- {
		event := &record.Events[index]
		if event.State == nativejournal.StateCompleted || event.State == nativejournal.StateFailed || event.State == nativejournal.StateCanceled {
			return event
		}
	}
	return nil
}

func nativeOutcome(record Record) string {
	if record.State == nativejournal.StateCompleted {
		return "completed"
	}
	if record.State == nativejournal.StateCanceled {
		return "canceled"
	}
	terminal := terminalEvent(record)
	if terminal != nil && terminal.ExitCode != nil {
		return fmt.Sprintf("exit-code-%d", *terminal.ExitCode)
	}
	return "failed"
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }

func wrapperCommand(executable, root, path string) string {
	return shellQuote(executable) + " session execute-native --root " + shellQuote(root) + " --path " + shellQuote(path)
}

func canonicalExecutable(raw string) (string, string, error) {
	if !filepath.IsAbs(raw) {
		return "", "", fmt.Errorf("Sentinel executable must be an absolute path")
	}
	resolved, err := filepath.EvalSymlinks(raw)
	if err != nil {
		return "", "", fmt.Errorf("resolve Sentinel executable: %w", err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", "", err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", "", fmt.Errorf("Sentinel executable %s is not an executable regular file", resolved)
	}
	digest, err := hashFile(resolved)
	return resolved, digest, err
}

func validateExecutable(intent nativejournal.Intent) error {
	actual, digest, err := canonicalExecutable(intent.SentinelExecutable)
	if err != nil {
		return err
	}
	if actual != intent.SentinelExecutable || digest != intent.SentinelExecutableSHA256 {
		return fmt.Errorf("Sentinel executable identity changed after native session intent was committed")
	}
	current, err := os.Executable()
	if err != nil {
		return fmt.Errorf("identify running Sentinel wrapper: %w", err)
	}
	_, currentDigest, err := canonicalExecutable(current)
	if err != nil {
		return err
	}
	if currentDigest != intent.SentinelExecutableSHA256 {
		return fmt.Errorf("running Sentinel wrapper does not match the executable pinned by the native session intent")
	}
	return nil
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func hashUnderRoot(root, path string) (string, error) {
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer rootHandle.Close()
	file, err := rootHandle.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func markTerminalFailure(root, recordPath, sessionID, reason string) {
	record, err := nativejournal.Load(root, recordPath)
	if err != nil || terminalState(record.State) {
		return
	}
	_, _, _ = appendEvent(root, recordPath, "wrapper-setup-failed:"+sessionID, nativejournal.StateFailed, lastHost(record), safeReason(reason), nil, "wrapper-setup-failed")
}

func markIndeterminate(root, recordPath, sessionID, reason string) {
	record, err := nativejournal.Load(root, recordPath)
	if err != nil || terminalState(record.State) {
		return
	}
	_, _, _ = appendEvent(root, recordPath, "wrapper-indeterminate:"+sessionID, nativejournal.StateIndeterminate, lastHost(record), safeReason(reason), nil, "wrapper-recovery")
}

func createExclusiveUnderRoot(root, path string) (*os.File, error) {
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer rootHandle.Close()
	return rootHandle.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
}

func removeUnderRoot(root, path string) error {
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer rootHandle.Close()
	return rootHandle.Remove(path)
}

func absoluteRoot(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("native Sentinel session project root is required")
	}
	root, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("resolve native Sentinel project root: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve native Sentinel project root symlinks: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("inspect native Sentinel project root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("native Sentinel project root is not a directory")
	}
	return root, nil
}

func relativePath(label, raw string) (string, error) {
	if strings.TrimSpace(raw) == "" || filepath.IsAbs(raw) {
		return "", fmt.Errorf("native Sentinel %s path must be relative to the project root", label)
	}
	clean := filepath.Clean(raw)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("native Sentinel %s path must stay inside the project root", label)
	}
	return clean, nil
}

func nativeRecordPath(raw string) (string, error) {
	clean, err := relativePath("journal", raw)
	if err != nil {
		return "", err
	}
	if filepath.Dir(clean) != ArtifactDirectory || filepath.Ext(clean) != ".json" {
		return "", fmt.Errorf("native journal path must be a .json file directly under %s", ArtifactDirectory)
	}
	return clean, nil
}

func validateRecordLocation(recordPath string, record Record) error {
	if record.Intent.SessionID == "" || filepath.Base(recordPath) != record.Intent.SessionID+".json" {
		return fmt.Errorf("native journal filename does not match its session identity")
	}
	if record.Intent.StdoutPath != filepath.Join(ArtifactDirectory, record.Intent.SessionID+".stdout.log") || record.Intent.StderrPath != filepath.Join(ArtifactDirectory, record.Intent.SessionID+".stderr.log") {
		return fmt.Errorf("native session output paths do not match the journal session identity")
	}
	return nil
}

func findRole(plan capability.Plan, roleID string) (capability.Role, error) {
	if strings.TrimSpace(roleID) == "" {
		return capability.Role{}, fmt.Errorf("Sentinel session role is required")
	}
	for _, role := range plan.Roles {
		if role.ID == roleID {
			return role, nil
		}
	}
	return capability.Role{}, fmt.Errorf("Sentinel workspace has no role %q", roleID)
}

func newSessionID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("generate native Sentinel session ID: %w", err)
	}
	return "native-" + hex.EncodeToString(bytes[:]), nil
}

func nextTimestamp(record Record) time.Time {
	now := time.Now().UTC()
	if len(record.Events) > 0 && !now.After(record.Events[len(record.Events)-1].At) {
		return record.Events[len(record.Events)-1].At.Add(time.Nanosecond)
	}
	return now
}

func safeReason(raw string) string {
	return strings.NewReplacer("\x00", " ", "\r", " ", "\n", " ").Replace(raw)
}

func terminalState(state string) bool {
	return state == nativejournal.StateCompleted || state == nativejournal.StateFailed || state == nativejournal.StateCanceled
}

func hasCancellationRequest(record Record) bool {
	for _, event := range record.Events {
		if event.Kind == "provider-cancel" && event.State == nativejournal.StateCancelRequested {
			return true
		}
	}
	return false
}

func hasWrapperClaim(record Record) bool {
	for _, event := range record.Events {
		if event.Kind == nativejournal.KindWrapperClaim {
			return true
		}
	}
	return false
}

func sameHost(left, right *nativejournal.Host) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func equalIntPtr(left, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func sameIDs(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
