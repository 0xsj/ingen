package nativesession

import (
	"context"
	"fmt"
	"time"

	"ingen/herdr-sentinel/internal/herdrclient"
	"ingen/herdr-sentinel/internal/nativejournal"
)

type processInfoHostClient interface {
	HostClient
	ProcessInfo(context.Context, string) (herdrclient.ProcessInfo, error)
}

type ownedWorkspaceHostClient interface {
	HostClient
	CloseWorkspace(context.Context, string) error
}

// ProcessObservation is an explicitly local and unverified process snapshot
// tied to the exact host pane identity already recorded by Sentinel.
type ProcessObservation struct {
	Schema              string                  `json:"schema"`
	SessionID           string                  `json:"session_id"`
	RunID               string                  `json:"run_id"`
	JournalState        string                  `json:"journal_state"`
	ObservedAt          time.Time               `json:"locally_observed_at"`
	Source              string                  `json:"source"`
	Trust               string                  `json:"trust"`
	SentinelWorkspaceID string                  `json:"sentinel_workspace_id"`
	Host                nativejournal.Host      `json:"host"`
	ProcessInfo         herdrclient.ProcessInfo `json:"process_info"`
}

type WorkspaceCloseObservation struct {
	Schema              string             `json:"schema"`
	SessionID           string             `json:"session_id"`
	RunID               string             `json:"run_id"`
	JournalState        string             `json:"journal_state"`
	ObservedAt          time.Time          `json:"locally_observed_at"`
	Source              string             `json:"source"`
	Trust               string             `json:"trust"`
	SentinelWorkspaceID string             `json:"sentinel_workspace_id"`
	Host                nativejournal.Host `json:"host"`
	Action              string             `json:"action"`
}

// ObservePaneProcesses obtains read-only live process information for the
// exact workspace/pane/terminal binding already stored in the journal.
func ObservePaneProcesses(ctx context.Context, root, path string, host processInfoHostClient) (ProcessObservation, error) {
	if ctx == nil {
		return ProcessObservation{}, fmt.Errorf("native process observation context is required")
	}
	if host == nil {
		return ProcessObservation{}, fmt.Errorf("native Herdr process-info client is required")
	}
	root, err := absoluteRoot(root)
	if err != nil {
		return ProcessObservation{}, err
	}
	record, err := Load(root, path)
	if err != nil {
		return ProcessObservation{}, err
	}
	identity := lastHost(record)
	if identity == nil || identity.WorkspaceID == "" || identity.PaneID == "" || identity.TerminalID == "" || identity.SentinelWorkspaceID != record.Intent.WorkspaceID {
		return ProcessObservation{}, fmt.Errorf("native session has no complete stored workspace/pane/terminal identity")
	}
	if err := checkSupportedHost(ctx, host); err != nil {
		return ProcessObservation{}, err
	}
	if _, err := verifiedPane(ctx, host, root, record, identity); err != nil {
		return ProcessObservation{}, err
	}
	info, err := host.ProcessInfo(ctx, identity.PaneID)
	if err != nil {
		return ProcessObservation{}, fmt.Errorf("read process info for owned Herdr pane %q: %w", identity.PaneID, err)
	}
	if info.PaneID != identity.PaneID {
		return ProcessObservation{}, fmt.Errorf("Herdr process info does not match the native session's exact pane identity")
	}
	return ProcessObservation{
		Schema: "ingen.sentinel-native-process-observation/v1", SessionID: record.Intent.SessionID,
		RunID: record.Intent.RunID, JournalState: record.State, ObservedAt: time.Now().UTC(),
		Source: "herdr-pane-process-info", Trust: "local-observation-unverified",
		SentinelWorkspaceID: record.Intent.WorkspaceID, Host: *identity, ProcessInfo: info,
	}, nil
}

// CloseOwnedWorkspace closes only a settled, exactly bound workspace. A
// process terminal or explicit no-start cancellation event must prove that no
// wrapper/child outcome remains unknown.
func CloseOwnedWorkspace(ctx context.Context, root, path string, host ownedWorkspaceHostClient) (WorkspaceCloseObservation, error) {
	if ctx == nil {
		return WorkspaceCloseObservation{}, fmt.Errorf("native workspace-close context is required")
	}
	if host == nil {
		return WorkspaceCloseObservation{}, fmt.Errorf("native Herdr workspace-close client is required")
	}
	root, err := absoluteRoot(root)
	if err != nil {
		return WorkspaceCloseObservation{}, err
	}
	record, err := Load(root, path)
	if err != nil {
		return WorkspaceCloseObservation{}, err
	}
	if !canCloseSettledWorkspace(record) {
		return WorkspaceCloseObservation{}, fmt.Errorf("native workspace close requires a terminal journal with known process outcome or proven no-start cancellation")
	}
	identity := lastHost(record)
	if identity == nil || identity.WorkspaceID == "" || identity.PaneID == "" || identity.TerminalID == "" || identity.SentinelWorkspaceID != record.Intent.WorkspaceID {
		return WorkspaceCloseObservation{}, fmt.Errorf("native session has no complete stored workspace/pane/terminal identity")
	}
	if err := checkSupportedHost(ctx, host); err != nil {
		return WorkspaceCloseObservation{}, err
	}
	if _, err := verifiedPane(ctx, host, root, record, identity); err != nil {
		return WorkspaceCloseObservation{}, err
	}
	if err := host.CloseWorkspace(ctx, identity.WorkspaceID); err != nil {
		return WorkspaceCloseObservation{}, fmt.Errorf("close exact owned Herdr workspace %q: %w", identity.WorkspaceID, err)
	}
	return WorkspaceCloseObservation{
		Schema: "ingen.sentinel-native-workspace-close/v1", SessionID: record.Intent.SessionID,
		RunID: record.Intent.RunID, JournalState: record.State, ObservedAt: time.Now().UTC(),
		Source: "herdr-workspace-close", Trust: "local-operation-confirmed-unverified",
		SentinelWorkspaceID: record.Intent.WorkspaceID, Host: *identity, Action: "closed-exact-workspace",
	}, nil
}

func canCloseSettledWorkspace(record Record) bool {
	if !terminalState(record.State) {
		return false
	}
	terminal := terminalEvent(record)
	if terminal == nil {
		return false
	}
	if terminal.Kind == nativejournal.KindWrapperTerminal {
		return terminal.ExitCode != nil && terminal.StdoutSHA256 != "" && terminal.StderrSHA256 != ""
	}
	return record.State == nativejournal.StateCanceled && (terminal.Kind == nativejournal.KindProviderCanceledBeforeClaim || terminal.Kind == nativejournal.KindProviderCanceledBeforeStart || terminal.Kind == nativejournal.KindWrapperCanceledBeforeStart)
}
