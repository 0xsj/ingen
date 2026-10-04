// Package herdrclient implements a bounded client for Herdr's local socket API.
package herdrclient

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const (
	defaultTimeout         = 10 * time.Second
	defaultMaxResponseSize = 4 << 20
)

var requestSequence atomic.Uint64

// Client talks to one explicitly configured Herdr UNIX socket. It never starts,
// stops, upgrades, or changes focus in the Herdr host.
type Client struct {
	SocketPath       string
	Timeout          time.Duration
	MaxResponseBytes int64
	peerUIDOverride  func(net.Conn) (uint32, error)
}

// ServerInfo identifies the protocol exposed by the selected socket.
type ServerInfo struct {
	Version  string
	Protocol uint32
}

// Binding records opaque host-issued identities for one workspace and its root
// pane. IDs must be persisted and supplied back exactly; they are not ordinals.
type Binding struct {
	WorkspaceID string
	TabID       string
	PaneID      string
	TerminalID  string
	Label       string
	CWD         string
}

// Pane is the host's current identity and status view for one exact pane ID.
// TerminalID identifies the pane's terminal independently of mutable cwd/status.
type Pane struct {
	WorkspaceID   string
	TabID         string
	PaneID        string
	TerminalID    string
	CWD           string
	ForegroundCWD string
	AgentStatus   string
	Focused       bool
	Revision      uint64
}

// PaneOutput is a bounded observation returned by pane.read.
type PaneOutput struct {
	WorkspaceID string
	TabID       string
	PaneID      string
	Source      string
	Format      string
	Text        string
	Revision    uint64
	Truncated   bool
}

// ProcessInfo reports Herdr's read-only process observation for one exact
// pane. It is useful for correlating an owned terminal to a live process; it
// is not a Sentinel journal event or a host-authenticated lifecycle verdict.
type ProcessInfo struct {
	PaneID                   string    `json:"pane_id"`
	ShellPID                 *uint32   `json:"shell_pid,omitempty"`
	ForegroundProcessGroupID *uint32   `json:"foreground_process_group_id,omitempty"`
	ForegroundProcesses      []Process `json:"foreground_processes"`
}

type Process struct {
	PID     uint32   `json:"pid"`
	Name    string   `json:"name"`
	Argv0   *string  `json:"argv0,omitempty"`
	Argv    []string `json:"argv"`
	Cmdline *string  `json:"cmdline,omitempty"`
	CWD     *string  `json:"cwd,omitempty"`
}

// HostError is a rejection returned by Herdr. Unlike a transport error, it
// confirms that the host processed the request and returned an error response.
type HostError struct {
	Code    string
	Message string
}

func (e *HostError) Error() string {
	if e == nil {
		return "Herdr host error"
	}
	return fmt.Sprintf("Herdr host error %s: %s", e.Code, e.Message)
}

// DeliveryError reports a transport or protocol failure. For mutation calls,
// MayHaveApplied is true once any request bytes reached the socket. Callers must
// reconcile state before deciding whether to issue another mutation.
type DeliveryError struct {
	Method         string
	MayHaveApplied bool
	Err            error
}

// AuthenticationError means the selected UNIX endpoint or connected peer did
// not pass local same-effective-UID checks. It is always returned before any
// request bytes are written. This authenticates a local UID boundary only; it
// does not authenticate Herdr as an application or distinguish same-UID apps.
type AuthenticationError struct {
	Reason string
	Err    error
}

func (e *AuthenticationError) Error() string {
	if e == nil {
		return "Herdr peer authentication failed"
	}
	if e.Err != nil {
		return fmt.Sprintf("Herdr peer authentication failed: %s: %v", e.Reason, e.Err)
	}
	return fmt.Sprintf("Herdr peer authentication failed: %s", e.Reason)
}

func (e *AuthenticationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *DeliveryError) Error() string {
	if e == nil {
		return "Herdr delivery error"
	}
	certainty := "request was not applied"
	if e.MayHaveApplied {
		certainty = "request may have been applied"
	}
	return fmt.Sprintf("Herdr %s: %s: %v", e.Method, certainty, e.Err)
}

func (e *DeliveryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Ping reports the version and socket protocol selected by SocketPath.
func (c *Client) Ping(ctx context.Context) (ServerInfo, error) {
	var result struct {
		Type     string `json:"type"`
		Version  string `json:"version"`
		Protocol uint32 `json:"protocol"`
	}
	if err := c.call(ctx, "ping", map[string]any{}, false, &result); err != nil {
		return ServerInfo{}, err
	}
	if result.Type != "pong" || result.Version == "" || result.Protocol == 0 {
		return ServerInfo{}, c.protocolError("ping", false, errors.New("unexpected ping result"))
	}
	return ServerInfo{Version: result.Version, Protocol: result.Protocol}, nil
}

// CreateWorkspace creates a background workspace (focus is explicitly false).
func (c *Client) CreateWorkspace(ctx context.Context, cwd, label string) (Binding, error) {
	if strings.TrimSpace(cwd) == "" || strings.TrimSpace(label) == "" {
		return Binding{}, errors.New("workspace cwd and label are required")
	}
	var result struct {
		Type      string `json:"type"`
		Workspace struct {
			ID    string `json:"workspace_id"`
			Label string `json:"label"`
		} `json:"workspace"`
		Tab struct {
			ID string `json:"tab_id"`
		} `json:"tab"`
		RootPane paneWire `json:"root_pane"`
	}
	params := map[string]any{"cwd": cwd, "label": label, "focus": false}
	if err := c.call(ctx, "workspace.create", params, true, &result); err != nil {
		return Binding{}, err
	}
	if result.Type != "workspace_created" || result.Workspace.ID == "" || result.Workspace.Label != label || result.Tab.ID == "" ||
		result.RootPane.PaneID == "" || result.RootPane.TerminalID == "" ||
		result.RootPane.WorkspaceID != result.Workspace.ID || result.RootPane.TabID != result.Tab.ID ||
		result.RootPane.CWD == nil || *result.RootPane.CWD != cwd {
		return Binding{}, c.protocolError("workspace.create", true, errors.New("workspace response omitted or mismatched host identities"))
	}
	return Binding{
		WorkspaceID: result.Workspace.ID,
		TabID:       result.Tab.ID,
		PaneID:      result.RootPane.PaneID,
		TerminalID:  result.RootPane.TerminalID,
		Label:       result.Workspace.Label,
		CWD:         *result.RootPane.CWD,
	}, nil
}

// ListWorkspaces returns only exact label and pane-cwd matches from one host
// snapshot. It is intended for reconciliation after an uncertain create. A
// returned item is one matching pane binding; callers should require uniqueness.
func (c *Client) ListWorkspaces(ctx context.Context, cwd, label string) ([]Binding, error) {
	if strings.TrimSpace(cwd) == "" || strings.TrimSpace(label) == "" {
		return nil, errors.New("workspace cwd and label are required")
	}
	var result struct {
		Type     string `json:"type"`
		Snapshot struct {
			Workspaces []struct {
				ID    string `json:"workspace_id"`
				Label string `json:"label"`
			} `json:"workspaces"`
			Panes []paneWire `json:"panes"`
		} `json:"snapshot"`
	}
	if err := c.call(ctx, "session.snapshot", map[string]any{}, false, &result); err != nil {
		return nil, err
	}
	if result.Type != "session_snapshot" {
		return nil, c.protocolError("session.snapshot", false, errors.New("unexpected snapshot result"))
	}
	matched := make([]Binding, 0)
	for _, workspace := range result.Snapshot.Workspaces {
		if workspace.Label != label || workspace.ID == "" {
			continue
		}
		for _, pane := range result.Snapshot.Panes {
			if pane.WorkspaceID != workspace.ID || pane.CWD == nil || *pane.CWD != cwd {
				continue
			}
			if pane.PaneID == "" || pane.TabID == "" || pane.TerminalID == "" {
				continue
			}
			matched = append(matched, Binding{
				WorkspaceID: workspace.ID,
				TabID:       pane.TabID,
				PaneID:      pane.PaneID,
				TerminalID:  pane.TerminalID,
				Label:       workspace.Label,
				CWD:         *pane.CWD,
			})
		}
	}
	return matched, nil
}

// Snapshot returns the exact host session snapshot result as bounded JSON for
// read-only preservation checks. It is an observation, not an event cursor.
func (c *Client) Snapshot(ctx context.Context) (json.RawMessage, error) {
	var result struct {
		Type     string          `json:"type"`
		Snapshot json.RawMessage `json:"snapshot"`
	}
	if err := c.call(ctx, "session.snapshot", map[string]any{}, false, &result); err != nil {
		return nil, err
	}
	if result.Type != "session_snapshot" || len(result.Snapshot) == 0 || string(result.Snapshot) == "null" {
		return nil, c.protocolError("session.snapshot", false, errors.New("snapshot response omitted the host session snapshot"))
	}
	var identity struct {
		Version    string `json:"version"`
		Protocol   uint32 `json:"protocol"`
		Workspaces []any  `json:"workspaces"`
		Tabs       []any  `json:"tabs"`
		Panes      []any  `json:"panes"`
		Layouts    []any  `json:"layouts"`
		Agents     []any  `json:"agents"`
	}
	if err := json.Unmarshal(result.Snapshot, &identity); err != nil {
		return nil, c.protocolError("session.snapshot", false, fmt.Errorf("decode session snapshot: %w", err))
	}
	if identity.Version == "" || identity.Protocol == 0 || identity.Workspaces == nil || identity.Tabs == nil || identity.Panes == nil || identity.Layouts == nil || identity.Agents == nil {
		return nil, c.protocolError("session.snapshot", false, errors.New("session snapshot omitted required version, protocol, or collection fields"))
	}
	return append(json.RawMessage(nil), result.Snapshot...), nil
}

// Pane returns live identity for the exact requested pane. It rejects a host
// reply that names a different pane, which prevents binding to stale ordinals.
func (c *Client) Pane(ctx context.Context, paneID string) (Pane, error) {
	if strings.TrimSpace(paneID) == "" {
		return Pane{}, errors.New("pane ID is required")
	}
	var result struct {
		Type string   `json:"type"`
		Pane paneWire `json:"pane"`
	}
	if err := c.call(ctx, "pane.get", map[string]any{"pane_id": paneID}, false, &result); err != nil {
		return Pane{}, err
	}
	if result.Type != "pane_info" || result.Pane.PaneID != paneID || result.Pane.WorkspaceID == "" ||
		result.Pane.TabID == "" || result.Pane.TerminalID == "" {
		return Pane{}, c.protocolError("pane.get", false, errors.New("pane response omitted or mismatched host identities"))
	}
	return result.Pane.public(), nil
}

// ReadPane returns a bounded recent text view for the exact pane ID.
func (c *Client) ReadPane(ctx context.Context, paneID string) (PaneOutput, error) {
	if strings.TrimSpace(paneID) == "" {
		return PaneOutput{}, errors.New("pane ID is required")
	}
	var result struct {
		Type string `json:"type"`
		Read struct {
			WorkspaceID string `json:"workspace_id"`
			TabID       string `json:"tab_id"`
			PaneID      string `json:"pane_id"`
			Source      string `json:"source"`
			Format      string `json:"format"`
			Text        string `json:"text"`
			Revision    uint64 `json:"revision"`
			Truncated   bool   `json:"truncated"`
		} `json:"read"`
	}
	params := map[string]any{"pane_id": paneID, "source": "recent", "format": "text", "strip_ansi": true}
	if err := c.call(ctx, "pane.read", params, false, &result); err != nil {
		return PaneOutput{}, err
	}
	if result.Type != "pane_read" || result.Read.PaneID != paneID || result.Read.WorkspaceID == "" || result.Read.TabID == "" {
		return PaneOutput{}, c.protocolError("pane.read", false, errors.New("pane read response omitted or mismatched identities"))
	}
	return PaneOutput{
		WorkspaceID: result.Read.WorkspaceID,
		TabID:       result.Read.TabID,
		PaneID:      result.Read.PaneID,
		Source:      result.Read.Source,
		Format:      result.Read.Format,
		Text:        result.Read.Text,
		Revision:    result.Read.Revision,
		Truncated:   result.Read.Truncated,
	}, nil
}

// ProcessInfo returns Herdr's current process metadata for one exact pane ID.
func (c *Client) ProcessInfo(ctx context.Context, paneID string) (ProcessInfo, error) {
	if strings.TrimSpace(paneID) == "" {
		return ProcessInfo{}, errors.New("pane ID is required")
	}
	var result struct {
		Type        string `json:"type"`
		ProcessInfo struct {
			PaneID                   string  `json:"pane_id"`
			ShellPID                 *uint32 `json:"shell_pid"`
			ForegroundProcessGroupID *uint32 `json:"foreground_process_group_id"`
			ForegroundProcesses      []struct {
				PID     uint32   `json:"pid"`
				Name    string   `json:"name"`
				Argv0   *string  `json:"argv0"`
				Argv    []string `json:"argv"`
				Cmdline *string  `json:"cmdline"`
				CWD     *string  `json:"cwd"`
			} `json:"foreground_processes"`
		} `json:"process_info"`
	}
	if err := c.call(ctx, "pane.process_info", map[string]any{"pane_id": paneID}, false, &result); err != nil {
		return ProcessInfo{}, err
	}
	if result.Type != "pane_process_info" || result.ProcessInfo.PaneID != paneID {
		return ProcessInfo{}, c.protocolError("pane.process_info", false, errors.New("process info response omitted or mismatched pane identity"))
	}
	info := ProcessInfo{
		PaneID:                   paneID,
		ShellPID:                 result.ProcessInfo.ShellPID,
		ForegroundProcessGroupID: result.ProcessInfo.ForegroundProcessGroupID,
		ForegroundProcesses:      make([]Process, 0, len(result.ProcessInfo.ForegroundProcesses)),
	}
	for _, process := range result.ProcessInfo.ForegroundProcesses {
		if process.PID == 0 || strings.TrimSpace(process.Name) == "" {
			return ProcessInfo{}, c.protocolError("pane.process_info", false, errors.New("process info contains an incomplete process identity"))
		}
		info.ForegroundProcesses = append(info.ForegroundProcesses, Process{
			PID: process.PID, Name: process.Name, Argv0: process.Argv0,
			Argv: append([]string(nil), process.Argv...), Cmdline: process.Cmdline, CWD: process.CWD,
		})
	}
	return info, nil
}

// Run sends command text and Enter in one atomic pane.send_input request.
func (c *Client) Run(ctx context.Context, paneID, commandText string) error {
	if strings.TrimSpace(paneID) == "" || commandText == "" {
		return errors.New("pane ID and command text are required")
	}
	var result struct {
		Type string `json:"type"`
	}
	params := map[string]any{"pane_id": paneID, "text": commandText, "keys": []string{"enter"}}
	if err := c.call(ctx, "pane.send_input", params, true, &result); err != nil {
		return err
	}
	if result.Type != "ok" {
		return c.protocolError("pane.send_input", true, errors.New("unexpected send_input result"))
	}
	return nil
}

// Interrupt sends Ctrl-C to one exact pane. It never selects a focused pane.
func (c *Client) Interrupt(ctx context.Context, paneID string) error {
	if strings.TrimSpace(paneID) == "" {
		return errors.New("pane ID is required")
	}
	var result struct {
		Type string `json:"type"`
	}
	if err := c.call(ctx, "pane.send_keys", map[string]any{"pane_id": paneID, "keys": []string{"ctrl+c"}}, true, &result); err != nil {
		return err
	}
	if result.Type != "ok" {
		return c.protocolError("pane.send_keys", true, errors.New("unexpected send_keys result"))
	}
	return nil
}

// CloseWorkspace closes one exact workspace without closing its group.
func (c *Client) CloseWorkspace(ctx context.Context, workspaceID string) error {
	if strings.TrimSpace(workspaceID) == "" {
		return errors.New("workspace ID is required")
	}
	var result struct {
		Type string `json:"type"`
	}
	params := map[string]any{"workspace_id": workspaceID, "close_group": false}
	if err := c.call(ctx, "workspace.close", params, true, &result); err != nil {
		return err
	}
	if result.Type != "ok" {
		return c.protocolError("workspace.close", true, errors.New("unexpected workspace.close result"))
	}
	return nil
}

type paneWire struct {
	PaneID        string  `json:"pane_id"`
	WorkspaceID   string  `json:"workspace_id"`
	TabID         string  `json:"tab_id"`
	TerminalID    string  `json:"terminal_id"`
	CWD           *string `json:"cwd"`
	ForegroundCWD *string `json:"foreground_cwd"`
	AgentStatus   string  `json:"agent_status"`
	Focused       bool    `json:"focused"`
	Revision      uint64  `json:"revision"`
}

func (p paneWire) public() Pane {
	var cwd, foregroundCWD string
	if p.CWD != nil {
		cwd = *p.CWD
	}
	if p.ForegroundCWD != nil {
		foregroundCWD = *p.ForegroundCWD
	}
	return Pane{
		WorkspaceID: p.WorkspaceID, TabID: p.TabID, PaneID: p.PaneID,
		TerminalID: p.TerminalID, CWD: cwd, ForegroundCWD: foregroundCWD,
		AgentStatus: p.AgentStatus, Focused: p.Focused, Revision: p.Revision,
	}
}

type requestEnvelope struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params"`
}

type responseEnvelope struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) call(ctx context.Context, method string, params any, mutation bool, result any) error {
	if ctx == nil {
		return errors.New("context must not be nil")
	}
	if err := validateSocketEndpoint(c.SocketPath); err != nil {
		return &AuthenticationError{Reason: "unsafe or unavailable explicit socket endpoint", Err: err}
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	if timeout < 0 {
		return errors.New("Herdr client timeout must not be negative")
	}
	maxResponseBytes := c.MaxResponseBytes
	if maxResponseBytes == 0 {
		maxResponseBytes = defaultMaxResponseSize
	}
	if maxResponseBytes < 1 {
		return errors.New("Herdr maximum response size must be positive")
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	transportFailure := func(sent int, err error) error {
		if ctxErr := requestCtx.Err(); ctxErr != nil {
			err = ctxErr
		}
		return &DeliveryError{Method: method, MayHaveApplied: mutation && sent > 0, Err: err}
	}
	conn, err := (&net.Dialer{}).DialContext(requestCtx, "unix", c.SocketPath)
	if err != nil {
		return &DeliveryError{Method: method, Err: err}
	}
	stopClose := context.AfterFunc(requestCtx, func() { _ = conn.Close() })
	defer func() {
		stopClose()
		_ = conn.Close()
	}()
	peerUID := peerUID
	if c.peerUIDOverride != nil {
		peerUID = c.peerUIDOverride
	}
	uid, err := peerUID(conn)
	if err != nil {
		return &AuthenticationError{Reason: "could not inspect connected UNIX peer credentials", Err: err}
	}
	if uid != uint32(os.Geteuid()) {
		return &AuthenticationError{Reason: fmt.Sprintf("connected peer effective UID %d does not match caller effective UID %d", uid, os.Geteuid())}
	}
	if deadline, ok := requestCtx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return transportFailure(0, err)
		}
	}
	id := "ingen-herdr-" + strconv.FormatUint(requestSequence.Add(1), 10)
	wire, err := json.Marshal(requestEnvelope{ID: id, Method: method, Params: params})
	if err != nil {
		return err
	}
	wire = append(wire, '\n')
	sent, err := writeAll(conn, wire)
	if err != nil {
		return transportFailure(sent, err)
	}
	responseBytes, err := readLine(conn, maxResponseBytes)
	if err != nil {
		return transportFailure(sent, err)
	}
	var envelope responseEnvelope
	if err := json.Unmarshal(responseBytes, &envelope); err != nil {
		return &DeliveryError{Method: method, MayHaveApplied: mutation && sent > 0, Err: fmt.Errorf("decode Herdr response: %w", err)}
	}
	if envelope.ID != id {
		return &DeliveryError{Method: method, MayHaveApplied: mutation && sent > 0, Err: fmt.Errorf("response ID %q does not match request ID %q", envelope.ID, id)}
	}
	if envelope.Error != nil && len(envelope.Result) > 0 {
		return &DeliveryError{Method: method, MayHaveApplied: mutation && sent > 0, Err: errors.New("Herdr response contains both result and error")}
	}
	if envelope.Error != nil {
		if strings.TrimSpace(envelope.Error.Code) == "" || strings.TrimSpace(envelope.Error.Message) == "" {
			return &DeliveryError{Method: method, MayHaveApplied: mutation && sent > 0, Err: errors.New("Herdr error response omitted code or message")}
		}
		return &HostError{Code: envelope.Error.Code, Message: envelope.Error.Message}
	}
	if len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		return &DeliveryError{Method: method, MayHaveApplied: mutation && sent > 0, Err: errors.New("Herdr response has no result")}
	}
	if err := json.Unmarshal(envelope.Result, result); err != nil {
		return &DeliveryError{Method: method, MayHaveApplied: mutation && sent > 0, Err: fmt.Errorf("decode Herdr result: %w", err)}
	}
	return nil
}

func (c *Client) protocolError(method string, mutation bool, err error) error {
	return &DeliveryError{Method: method, MayHaveApplied: mutation, Err: err}
}

func writeAll(writer io.Writer, data []byte) (int, error) {
	total := 0
	for total < len(data) {
		n, err := writer.Write(data[total:])
		total += n
		if err != nil {
			return total, err
		}
		if n == 0 {
			return total, io.ErrShortWrite
		}
	}
	return total, nil
}

func readLine(conn net.Conn, maxBytes int64) ([]byte, error) {
	reader := bufio.NewReaderSize(conn, 32*1024)
	line := make([]byte, 0, 4096)
	for {
		part, err := reader.ReadSlice('\n')
		if int64(len(line))+int64(len(part)) > maxBytes {
			return nil, fmt.Errorf("Herdr response exceeds %d bytes", maxBytes)
		}
		line = append(line, part...)
		if err == nil {
			return line[:len(line)-1], nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) {
			if len(line) > 0 {
				return nil, io.ErrUnexpectedEOF
			}
			return nil, io.EOF
		}
		return nil, err
	}
}
