// Package nativejournal stores Sentinel-owned intent and lifecycle for native
// Herdr sessions. Raw host callbacks do not acquire host event identity here.
package nativejournal

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	Schema      = "ingen.sentinel-native-session/v1"
	Origin      = "sentinel-launch-journal"
	Enforcement = "declaration-only"
	Assurance   = "unverified"

	StatePrepared        = "prepared"
	StateDispatching     = "dispatching"
	StateSubmitted       = "submitted"
	StateRunning         = "running"
	StateCancelRequested = "cancel-requested"
	StateCompleted       = "completed"
	StateFailed          = "failed"
	StateCanceled        = "canceled"
	StateIndeterminate   = "indeterminate"

	KindJournalCreated              = "journal-created"
	KindWrapperClaim                = "wrapper-claim"
	KindHostDispatchAck             = "host-dispatch-ack"
	KindWrapperStarted              = "wrapper-started"
	KindWrapperTerminal             = "wrapper-terminal"
	KindWrapperCanceledBeforeStart  = "wrapper-canceled-before-start"
	KindProviderCanceledBeforeClaim = "provider-canceled-before-claim"

	preparedEventID = "native-journal-prepared"
)

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Intent is immutable launch input. Workdir, ReceiptPath, StdoutPath, and
// StderrPath are normalized paths relative to the root passed to the API.
// SentinelExecutable must be an absolute path; HerdrSocket is an endpoint.
type Intent struct {
	SessionID                string   `json:"session_id"`
	RunID                    string   `json:"run_id"`
	WorkspaceID              string   `json:"workspace_id"`
	WorkspaceVersion         int64    `json:"workspace_version"`
	WorkspaceManifestSHA256  string   `json:"workspace_manifest_sha256"`
	RoleID                   string   `json:"role_id"`
	RoleKind                 string   `json:"role_kind"`
	Workdir                  string   `json:"workdir"`
	ReceiptPath              string   `json:"receipt_path"`
	Argv                     []string `json:"argv"`
	StdoutPath               string   `json:"stdout_path"`
	StderrPath               string   `json:"stderr_path"`
	HerdrSocket              string   `json:"herdr_socket"`
	SentinelExecutable       string   `json:"sentinel_executable"`
	SentinelExecutableSHA256 string   `json:"sentinel_executable_sha256"`
}

// Host is the optional binding supplied by a wrapper that has observed a
// concrete host location. It does not imply that the observation is trusted.
type Host struct {
	WorkspaceID         string `json:"workspace_id"`
	SentinelWorkspaceID string `json:"sentinel_workspace_id"`
	PaneID              string `json:"pane_id,omitempty"`
	TerminalID          string `json:"terminal_id,omitempty"`
}

// Event is one append-only caller-identified observation or state decision.
// Terminal output digests are SHA-256 hex strings when supplied.
type Event struct {
	ID           string    `json:"id"`
	Kind         string    `json:"kind"`
	At           time.Time `json:"at"`
	State        string    `json:"state"`
	Host         *Host     `json:"host,omitempty"`
	Reason       string    `json:"reason,omitempty"`
	ExitCode     *int      `json:"exit_code,omitempty"`
	StdoutSHA256 string    `json:"stdout_sha256,omitempty"`
	StderrSHA256 string    `json:"stderr_sha256,omitempty"`
}

// Record preserves the exact launch intent and the complete event history.
// State is the replayed effective state and must match Events.
type Record struct {
	Schema      string  `json:"schema"`
	Origin      string  `json:"origin"`
	Enforcement string  `json:"enforcement"`
	Assurance   string  `json:"assurance"`
	Intent      Intent  `json:"intent"`
	State       string  `json:"state"`
	Events      []Event `json:"events"`
}

// New validates and creates the initial prepared journal record.
func New(intent Intent, now time.Time) (Record, error) {
	if err := validateIntent(intent); err != nil {
		return Record{}, err
	}
	if now.IsZero() {
		return Record{}, fmt.Errorf("native journal: initial time is required")
	}
	now = now.UTC()
	record := Record{
		Schema:      Schema,
		Origin:      Origin,
		Enforcement: Enforcement,
		Assurance:   Assurance,
		Intent:      cloneIntent(intent),
		State:       StatePrepared,
		Events: []Event{{
			ID:    preparedEventID,
			Kind:  KindJournalCreated,
			At:    now,
			State: StatePrepared,
		}},
	}
	if err := Validate(record); err != nil {
		return Record{}, err
	}
	return record, nil
}

// Create durably publishes a new record at a normalized path under root. An
// existing journal is never replaced.
func Create(root, journalPath string, record Record) error {
	if err := Validate(record); err != nil {
		return err
	}
	rootHandle, err := openRoot(root)
	if err != nil {
		return err
	}
	defer rootHandle.Close()
	cleanPath, err := rootedPath(journalPath)
	if err != nil {
		return err
	}
	if err := validateIntentPaths(rootHandle, record.Intent); err != nil {
		return err
	}
	if err := validateExecutable(record.Intent); err != nil {
		return err
	}
	if cleanPath == record.Intent.ReceiptPath || cleanPath == record.Intent.StdoutPath || cleanPath == record.Intent.StderrPath {
		return fmt.Errorf("native journal: journal, receipt, stdout, and stderr paths must be distinct")
	}
	if err := checkPath(rootHandle, cleanPath, true); err != nil {
		return fmt.Errorf("native journal path: %w", err)
	}
	lock, err := lockPath(rootHandle, cleanPath+".lock")
	if err != nil {
		return err
	}
	defer unlockPath(lock)
	if _, err := rootHandle.Lstat(cleanPath); err == nil {
		return fmt.Errorf("native journal: refusing to replace existing file %q", cleanPath)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("native journal: inspect %q: %w", cleanPath, err)
	}
	data, err := marshal(record)
	if err != nil {
		return err
	}
	return publish(rootHandle, cleanPath, data, true)
}

// Load strictly decodes and replay-validates a journal under root.
func Load(root, journalPath string) (Record, error) {
	rootHandle, err := openRoot(root)
	if err != nil {
		return Record{}, err
	}
	defer rootHandle.Close()
	cleanPath, err := rootedPath(journalPath)
	if err != nil {
		return Record{}, err
	}
	if err := checkPath(rootHandle, cleanPath, false); err != nil {
		return Record{}, fmt.Errorf("native journal path: %w", err)
	}
	data, err := rootHandle.ReadFile(cleanPath)
	if err != nil {
		return Record{}, fmt.Errorf("native journal: read %q: %w", cleanPath, err)
	}
	record, err := decode(cleanPath, data)
	if err != nil {
		return Record{}, err
	}
	if err := validateExistingIntentPaths(rootHandle, record.Intent); err != nil {
		return Record{}, err
	}
	return record, nil
}

// Append serializes the read/replay/append/publish cycle with a process-shared
// advisory lock on supported Unix platforms. The bool reports whether an
// event was appended; an exact retry returns false without rewriting bytes.
func Append(root, journalPath string, event Event) (Record, bool, error) {
	rootHandle, err := openRoot(root)
	if err != nil {
		return Record{}, false, err
	}
	defer rootHandle.Close()
	cleanPath, err := rootedPath(journalPath)
	if err != nil {
		return Record{}, false, err
	}
	if err := checkPath(rootHandle, cleanPath, false); err != nil {
		return Record{}, false, fmt.Errorf("native journal path: %w", err)
	}
	lock, err := lockPath(rootHandle, cleanPath+".lock")
	if err != nil {
		return Record{}, false, err
	}
	defer unlockPath(lock)
	record, err := loadRooted(rootHandle, cleanPath)
	if err != nil {
		return Record{}, false, err
	}
	if event.Kind == KindWrapperClaim {
		return Record{}, false, fmt.Errorf("native journal: wrapper claims must use ClaimExecution")
	}
	if event.Kind == KindWrapperStarted && record.State == StateSubmitted {
		return Record{}, false, fmt.Errorf("native journal: wrapper starts from submitted must use StartExecution")
	}
	updated, changed, err := appendRecord(record, event)
	if err != nil || !changed {
		return updated, changed, err
	}
	data, err := marshal(updated)
	if err != nil {
		return Record{}, false, err
	}
	if err := publish(rootHandle, cleanPath, data, false); err != nil {
		return Record{}, false, err
	}
	return updated, true, nil
}

// ClaimExecution atomically claims one already-reserved dispatch for the
// matching session. The wrapper must launch only when claimed is true. The
// claim is the dispatching -> submitted event transition; retrying the same
// or a later invocation cannot authorize a second wrapper execution.
func ClaimExecution(root, journalPath, sessionID string, event Event) (Record, bool, error) {
	rootHandle, err := openRoot(root)
	if err != nil {
		return Record{}, false, err
	}
	defer rootHandle.Close()
	cleanPath, err := rootedPath(journalPath)
	if err != nil {
		return Record{}, false, err
	}
	if err := checkPath(rootHandle, cleanPath, false); err != nil {
		return Record{}, false, fmt.Errorf("native journal path: %w", err)
	}
	lock, err := lockPath(rootHandle, cleanPath+".lock")
	if err != nil {
		return Record{}, false, err
	}
	defer unlockPath(lock)
	record, err := loadRooted(rootHandle, cleanPath)
	if err != nil {
		return Record{}, false, err
	}
	if record.Intent.SessionID != sessionID {
		return Record{}, false, fmt.Errorf("native journal: session ID %q does not match intent", sessionID)
	}
	if err := validateExecutable(record.Intent); err != nil {
		return Record{}, false, err
	}
	if event.State != StateSubmitted || event.Kind != KindWrapperClaim {
		return Record{}, false, fmt.Errorf("native journal: execution claim event must be a wrapper-claim with state %q", StateSubmitted)
	}
	if event.At.IsZero() {
		return Record{}, false, fmt.Errorf("native journal: execution claim event requires an RFC3339 caller timestamp")
	}
	for _, prior := range record.Events {
		if prior.ID == event.ID {
			event.At = prior.At
			if sameEvent(prior, event) {
				return record, false, nil
			}
			return Record{}, false, fmt.Errorf("native journal: event ID %q was reused with different content", event.ID)
		}
		if prior.Kind == KindWrapperClaim {
			return record, false, nil
		}
	}
	if record.State != StateDispatching {
		return record, false, nil
	}
	// Claim time belongs to this locked Sentinel operation. The caller's time
	// may have been sampled before an acknowledgement won the lock, so choose a
	// fresh UTC time no earlier than the last durable event.
	event.At = logicalNextAt(record.Events[len(record.Events)-1].At, event.At)
	updated, changed, err := appendRecord(record, event)
	if err != nil || !changed {
		return updated, false, err
	}
	data, err := marshal(updated)
	if err != nil {
		return Record{}, false, err
	}
	if err := publish(rootHandle, cleanPath, data, false); err != nil {
		return Record{}, false, err
	}
	return updated, true, nil
}

// StartExecution atomically claims the child-process start after wrapper
// execution was claimed. The caller must start the child only when started is
// true. A cancellation or unknown outcome written first blocks this claim.
func StartExecution(root, journalPath, sessionID string, event Event) (Record, bool, error) {
	rootHandle, err := openRoot(root)
	if err != nil {
		return Record{}, false, err
	}
	defer rootHandle.Close()
	cleanPath, err := rootedPath(journalPath)
	if err != nil {
		return Record{}, false, err
	}
	if err := checkPath(rootHandle, cleanPath, false); err != nil {
		return Record{}, false, fmt.Errorf("native journal path: %w", err)
	}
	lock, err := lockPath(rootHandle, cleanPath+".lock")
	if err != nil {
		return Record{}, false, err
	}
	defer unlockPath(lock)
	record, err := loadRooted(rootHandle, cleanPath)
	if err != nil {
		return Record{}, false, err
	}
	if record.Intent.SessionID != sessionID {
		return Record{}, false, fmt.Errorf("native journal: session ID %q does not match intent", sessionID)
	}
	if err := validateExecutable(record.Intent); err != nil {
		return Record{}, false, err
	}
	if event.State != StateRunning || event.Kind != KindWrapperStarted || event.At.IsZero() {
		return Record{}, false, fmt.Errorf("native journal: child start event must be wrapper-started with state %q and a timestamp", StateRunning)
	}
	for _, prior := range record.Events {
		if prior.ID == event.ID {
			event.At = prior.At
			if sameEvent(prior, event) {
				return record, false, nil
			}
			return Record{}, false, fmt.Errorf("native journal: event ID %q was reused with different content", event.ID)
		}
		if prior.Kind == KindWrapperStarted {
			return record, false, nil
		}
	}
	if record.State != StateSubmitted {
		return record, false, nil
	}
	event.At = logicalNextAt(record.Events[len(record.Events)-1].At, event.At)
	updated, changed, err := appendRecord(record, event)
	if err != nil || !changed {
		return updated, false, err
	}
	data, err := marshal(updated)
	if err != nil {
		return Record{}, false, err
	}
	if err := publish(rootHandle, cleanPath, data, false); err != nil {
		return Record{}, false, err
	}
	return updated, true, nil
}

// WriteJSON validates and writes a journal as indented JSON.
func WriteJSON(w io.Writer, record Record) error {
	if w == nil {
		return fmt.Errorf("native journal: writer is required")
	}
	data, err := marshal(record)
	if err != nil {
		return err
	}
	n, err := w.Write(data)
	if err != nil {
		return fmt.Errorf("native journal: write JSON: %w", err)
	}
	if n != len(data) {
		return fmt.Errorf("native journal: write JSON: %w", io.ErrShortWrite)
	}
	return nil
}

// Validate checks record identity, immutable intent fields, and event replay.
func Validate(record Record) error {
	if record.Schema != Schema || record.Origin != Origin || record.Enforcement != Enforcement || record.Assurance != Assurance {
		return fmt.Errorf("native journal: schema/origin/enforcement/assurance metadata is invalid")
	}
	if err := validateIntent(record.Intent); err != nil {
		return err
	}
	state, err := replay(record)
	if err != nil {
		return err
	}
	if state != record.State {
		return fmt.Errorf("native journal: stored state %q does not match replayed state %q", record.State, state)
	}
	return nil
}

func loadRooted(root *os.Root, name string) (Record, error) {
	if err := checkPath(root, name, false); err != nil {
		return Record{}, fmt.Errorf("native journal path: %w", err)
	}
	data, err := root.ReadFile(name)
	if err != nil {
		return Record{}, fmt.Errorf("native journal: read %q: %w", name, err)
	}
	record, err := decode(name, data)
	if err != nil {
		return Record{}, err
	}
	if err := validateIntentPaths(root, record.Intent); err != nil {
		return Record{}, err
	}
	return record, nil
}

func decode(name string, data []byte) (Record, error) {
	if err := rejectDuplicateKeys(data); err != nil {
		return Record{}, fmt.Errorf("native journal: decode %q: %w", name, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var record Record
	if err := decoder.Decode(&record); err != nil {
		return Record{}, fmt.Errorf("native journal: decode %q: %w", name, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Record{}, fmt.Errorf("native journal: decode %q: trailing JSON value", name)
		}
		return Record{}, fmt.Errorf("native journal: decode %q: trailing data: %w", name, err)
	}
	if err := Validate(record); err != nil {
		return Record{}, fmt.Errorf("native journal: validate %q: %w", name, err)
	}
	return record, nil
}

func rejectDuplicateKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	return scanJSONValue(decoder)
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key is not a string")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate JSON object key %q", key)
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		closeToken, err := decoder.Token()
		if err != nil {
			return err
		}
		if closeToken != json.Delim('}') {
			return fmt.Errorf("malformed JSON object")
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		closeToken, err := decoder.Token()
		if err != nil {
			return err
		}
		if closeToken != json.Delim(']') {
			return fmt.Errorf("malformed JSON array")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	return nil
}

func marshal(record Record) ([]byte, error) {
	if err := Validate(record); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("native journal: encode: %w", err)
	}
	return append(data, '\n'), nil
}

func appendRecord(record Record, event Event) (Record, bool, error) {
	if err := validateEvent(event, record.Intent); err != nil {
		return Record{}, false, err
	}
	event.At = event.At.UTC()
	for _, prior := range record.Events {
		if prior.ID == event.ID {
			if sameEvent(prior, event) {
				return record, false, nil
			}
			return Record{}, false, fmt.Errorf("native journal: event ID %q was reused with different content", event.ID)
		}
	}
	if err := validateHostBinding(record.Events, event); err != nil {
		return Record{}, false, err
	}
	if err := validateCancellationEvidence(record.Events, record.State, event); err != nil {
		return Record{}, false, err
	}
	if isTerminal(record.State) && event.Kind == KindHostDispatchAck && event.State == StateSubmitted {
		return record, false, nil
	}
	if !event.At.After(record.Events[len(record.Events)-1].At) {
		return Record{}, false, fmt.Errorf("native journal: event %q timestamp must be later than the preceding event", event.ID)
	}
	next, err := nextState(record.State, event)
	if err != nil {
		return Record{}, false, err
	}
	updated := record
	updated.Events = append(append([]Event(nil), record.Events...), cloneEvent(event))
	updated.State = next
	if err := Validate(updated); err != nil {
		return Record{}, false, err
	}
	return updated, true, nil
}

func replay(record Record) (string, error) {
	if len(record.Events) == 0 {
		return "", fmt.Errorf("native journal: events must contain the prepared event")
	}
	seen := make(map[string]Event, len(record.Events))
	state := ""
	var previous time.Time
	for index, event := range record.Events {
		if err := validateEvent(event, record.Intent); err != nil {
			return "", fmt.Errorf("native journal: events[%d]: %w", index, err)
		}
		if _, exists := seen[event.ID]; exists {
			return "", fmt.Errorf("native journal: duplicate event ID %q", event.ID)
		}
		seen[event.ID] = event
		if index > 0 {
			if err := validateHostBinding(record.Events[:index], event); err != nil {
				return "", fmt.Errorf("native journal: event %q: %w", event.ID, err)
			}
			if err := validateCancellationEvidence(record.Events[:index], state, event); err != nil {
				return "", fmt.Errorf("native journal: event %q: %w", event.ID, err)
			}
		}
		if index == 0 {
			if event.ID != preparedEventID || event.Kind != KindJournalCreated || event.State != StatePrepared {
				return "", fmt.Errorf("native journal: first event must be the prepared event")
			}
			state = StatePrepared
			previous = event.At
			continue
		}
		if !event.At.After(previous) {
			return "", fmt.Errorf("native journal: event %q timestamp is not later than its predecessor", event.ID)
		}
		previous = event.At
		next, err := nextState(state, event)
		if err != nil {
			return "", fmt.Errorf("native journal: event %q: %w", event.ID, err)
		}
		state = next
	}
	return state, nil
}

func nextState(current string, event Event) (string, error) {
	next := event.State
	if isTerminal(current) {
		return "", fmt.Errorf("native journal: terminal state %q is immutable", current)
	}
	if event.Kind == KindHostDispatchAck {
		if next != StateSubmitted || (current != StateDispatching && current != StateSubmitted && current != StateRunning && current != StateCancelRequested && current != StateIndeterminate) {
			return "", fmt.Errorf("native journal: host dispatch acknowledgement is invalid in state %q", current)
		}
		return current, nil
	}
	if current == StateCancelRequested && event.Kind == KindWrapperStarted && next == StateRunning {
		// The observation can arrive after cancellation even if the process had
		// already entered its start path. Preserve the cancellation request.
		return current, nil
	}
	allowed := false
	switch current {
	case StatePrepared:
		allowed = next == StateDispatching || ((next == StateFailed || next == StateIndeterminate) && strings.TrimSpace(event.Reason) != "")
	case StateDispatching:
		allowed = (next == StateSubmitted && event.Kind == KindWrapperClaim) || next == StateCancelRequested || next == StateIndeterminate || next == StateFailed
	case StateSubmitted:
		allowed = (next == StateRunning && event.Kind == KindWrapperStarted) || next == StateCancelRequested || next == StateIndeterminate || (next == StateFailed && strings.TrimSpace(event.Reason) != "")
	case StateRunning:
		allowed = next == StateCompleted || next == StateFailed || next == StateCancelRequested || next == StateIndeterminate || (next == StateCanceled && (event.Kind == KindWrapperTerminal || event.Kind == KindWrapperCanceledBeforeStart) && strings.TrimSpace(event.Reason) != "")
	case StateCancelRequested:
		allowed = next == StateRunning || next == StateCompleted || next == StateFailed || (next == StateCanceled && (event.Kind == KindWrapperTerminal || event.Kind == KindWrapperCanceledBeforeStart || event.Kind == KindProviderCanceledBeforeClaim) && strings.TrimSpace(event.Reason) != "") || next == StateIndeterminate
	case StateIndeterminate:
		// Recovery is always explicit and records its evidence in the reason.
		allowed = strings.TrimSpace(event.Reason) != "" && (next == StateIndeterminate || next == StateRunning || next == StateCompleted || next == StateFailed || (next == StateCanceled && (event.Kind == KindWrapperTerminal || event.Kind == KindWrapperCanceledBeforeStart)))
	}
	if !allowed {
		return "", fmt.Errorf("native journal: invalid transition %q -> %q", current, next)
	}
	return next, nil
}

func validateCancellationEvidence(events []Event, current string, event Event) error {
	if event.Kind != KindProviderCanceledBeforeClaim {
		return nil
	}
	if current != StateCancelRequested {
		return fmt.Errorf("native journal: provider pre-claim cancellation requires cancel-requested state")
	}
	for _, prior := range events {
		if prior.Kind == KindWrapperClaim {
			return fmt.Errorf("native journal: provider pre-claim cancellation is invalid after wrapper claim")
		}
	}
	return nil
}

func logicalNextAt(previous, requested time.Time) time.Time {
	now := time.Now().UTC()
	requested = requested.UTC()
	minimum := previous.Add(time.Nanosecond)
	if now.Before(requested) {
		now = requested
	}
	if now.Before(minimum) {
		return minimum
	}
	return now
}

func isTerminal(state string) bool {
	return state == StateCompleted || state == StateFailed || state == StateCanceled
}

func validateIntent(intent Intent) error {
	for field, value := range map[string]string{
		"session_id":   intent.SessionID,
		"run_id":       intent.RunID,
		"workspace_id": intent.WorkspaceID,
		"role_id":      intent.RoleID,
		"role_kind":    intent.RoleKind,
		"herdr_socket": intent.HerdrSocket,
	} {
		if err := requiredString("intent."+field, value); err != nil {
			return err
		}
	}
	if intent.WorkspaceVersion < 1 {
		return fmt.Errorf("native journal: intent.workspace_version must be positive")
	}
	if !sha256Pattern.MatchString(intent.WorkspaceManifestSHA256) {
		return fmt.Errorf("native journal: intent.workspace_manifest_sha256 must be lowercase SHA-256 hex")
	}
	if len(intent.Argv) == 0 || strings.TrimSpace(intent.Argv[0]) == "" {
		return fmt.Errorf("native journal: intent.argv must begin with a non-empty executable")
	}
	for i, arg := range intent.Argv {
		if strings.ContainsRune(arg, '\x00') {
			return fmt.Errorf("native journal: intent.argv[%d] contains NUL", i)
		}
	}
	for label, value := range map[string]string{
		"workdir":      intent.Workdir,
		"receipt_path": intent.ReceiptPath,
		"stdout_path":  intent.StdoutPath,
		"stderr_path":  intent.StderrPath,
	} {
		if _, err := rootedPath(value); err != nil {
			return fmt.Errorf("native journal: intent.%s: %w", label, err)
		}
	}
	for _, endpointValue := range []string{intent.HerdrSocket, intent.SentinelExecutable} {
		if strings.ContainsAny(endpointValue, "\x00\r\n") {
			return fmt.Errorf("native journal: endpoint/path contains NUL or newline")
		}
	}
	if !filepath.IsAbs(intent.SentinelExecutable) || filepath.Clean(intent.SentinelExecutable) != intent.SentinelExecutable {
		return fmt.Errorf("native journal: intent.sentinel_executable must be a clean absolute path")
	}
	if !sha256Pattern.MatchString(intent.SentinelExecutableSHA256) {
		return fmt.Errorf("native journal: intent.sentinel_executable_sha256 must be lowercase SHA-256 hex")
	}
	if intent.StdoutPath == intent.StderrPath || intent.ReceiptPath == intent.StdoutPath || intent.ReceiptPath == intent.StderrPath {
		return fmt.Errorf("native journal: receipt/stdout/stderr paths must be distinct")
	}
	return nil
}

func validateEvent(event Event, intent Intent) error {
	if err := requiredString("event.id", event.ID); err != nil {
		return err
	}
	if err := requiredString("event.kind", event.Kind); err != nil {
		return err
	}
	if len(event.ID) > 256 || strings.ContainsAny(event.ID, "\x00\r\n") {
		return fmt.Errorf("native journal: event.id is invalid")
	}
	if event.At.IsZero() {
		return fmt.Errorf("native journal: event.at must be an RFC3339 timestamp")
	}
	if _, err := event.At.MarshalJSON(); err != nil {
		return fmt.Errorf("native journal: event.at must be an RFC3339 timestamp: %w", err)
	}
	switch event.State {
	case StatePrepared, StateDispatching, StateSubmitted, StateRunning, StateCancelRequested, StateCompleted, StateFailed, StateCanceled, StateIndeterminate:
	default:
		return fmt.Errorf("native journal: event.state %q is invalid", event.State)
	}
	if event.Host != nil {
		if event.Host.SentinelWorkspaceID != intent.WorkspaceID {
			return fmt.Errorf("native journal: event.host.sentinel_workspace_id does not match intent workspace")
		}
		if strings.TrimSpace(event.Host.WorkspaceID) == "" || (strings.TrimSpace(event.Host.PaneID) == "" && strings.TrimSpace(event.Host.TerminalID) == "") {
			return fmt.Errorf("native journal: event.host must identify a workspace and pane or terminal")
		}
	}
	if strings.ContainsAny(event.Reason, "\x00\r\n") {
		return fmt.Errorf("native journal: event.reason contains NUL or newline")
	}
	if event.State == StateCanceled {
		if strings.TrimSpace(event.Reason) == "" {
			return fmt.Errorf("native journal: canceled state requires a reason")
		}
		if event.Kind != KindWrapperTerminal && event.Kind != KindWrapperCanceledBeforeStart && event.Kind != KindProviderCanceledBeforeClaim {
			return fmt.Errorf("native journal: canceled state requires wrapper terminal or pre-start cancellation evidence")
		}
	}
	if event.Kind == KindWrapperCanceledBeforeStart || event.Kind == KindProviderCanceledBeforeClaim {
		if event.State != StateCanceled || strings.TrimSpace(event.Reason) == "" {
			return fmt.Errorf("native journal: pre-start cancellation evidence requires canceled state and a reason")
		}
		if event.ExitCode != nil || event.StdoutSHA256 != "" || event.StderrSHA256 != "" {
			return fmt.Errorf("native journal: pre-start cancellation cannot claim process exit or output evidence")
		}
	}
	terminal := isTerminal(event.State)
	if event.ExitCode != nil && !terminal {
		return fmt.Errorf("native journal: event.exit_code is only valid on a terminal state")
	}
	if (event.StdoutSHA256 != "" || event.StderrSHA256 != "") && !terminal {
		return fmt.Errorf("native journal: terminal output digests are only valid on a terminal state")
	}
	for label, digest := range map[string]string{"stdout_sha256": event.StdoutSHA256, "stderr_sha256": event.StderrSHA256} {
		if digest != "" && !sha256Pattern.MatchString(digest) {
			return fmt.Errorf("native journal: event.%s must be lowercase SHA-256 hex", label)
		}
	}
	if event.Kind == KindWrapperTerminal {
		if !terminal || event.ExitCode == nil || event.StdoutSHA256 == "" || event.StderrSHA256 == "" {
			return fmt.Errorf("native journal: wrapper-terminal event requires a process terminal state, exit code, and captured stdout/stderr digests")
		}
		if event.State == StateCompleted && (event.ExitCode == nil || *event.ExitCode != 0) {
			return fmt.Errorf("native journal: wrapper-terminal completed event requires exit code 0")
		}
		if event.State == StateFailed && (event.ExitCode == nil || *event.ExitCode == 0) {
			return fmt.Errorf("native journal: wrapper-terminal failed event requires a nonzero exit code")
		}
	}
	return nil
}

func validateHostBinding(events []Event, event Event) error {
	if event.Host == nil {
		return nil
	}
	for index := len(events) - 1; index >= 0; index-- {
		prior := events[index].Host
		if prior == nil {
			continue
		}
		if event.Host.WorkspaceID != prior.WorkspaceID || event.Host.SentinelWorkspaceID != prior.SentinelWorkspaceID {
			return fmt.Errorf("native journal: host workspace binding is immutable")
		}
		if prior.PaneID != "" && event.Host.PaneID != prior.PaneID {
			return fmt.Errorf("native journal: host pane binding is immutable")
		}
		if prior.TerminalID != "" && event.Host.TerminalID != prior.TerminalID {
			return fmt.Errorf("native journal: host terminal binding is immutable")
		}
		return nil
	}
	return nil
}

func requiredString(label, value string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("native journal: %s must be non-empty and contain no NUL", label)
	}
	return nil
}

func sameEvent(left, right Event) bool {
	a, _ := json.Marshal(left)
	b, _ := json.Marshal(right)
	return bytes.Equal(a, b)
}

func cloneIntent(intent Intent) Intent {
	intent.Argv = append([]string(nil), intent.Argv...)
	return intent
}

func cloneEvent(event Event) Event {
	if event.Host != nil {
		host := *event.Host
		event.Host = &host
	}
	if event.ExitCode != nil {
		code := *event.ExitCode
		event.ExitCode = &code
	}
	return event
}

func openRoot(root string) (*os.Root, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("native journal: root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("native journal: resolve root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("native journal: resolve root symlinks: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, fmt.Errorf("native journal: inspect root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("native journal: root must be a directory")
	}
	return os.OpenRoot(resolved)
}

func rootedPath(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" || strings.ContainsRune(raw, '\x00') || filepath.IsAbs(raw) || path.IsAbs(raw) {
		return "", fmt.Errorf("path must be a non-empty relative path")
	}
	slash := filepath.ToSlash(raw)
	if strings.Contains(slash, "\\") || strings.HasPrefix(slash, "//") {
		return "", fmt.Errorf("path must use normalized relative components")
	}
	for _, part := range strings.Split(slash, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("path must not contain empty, dot, or parent components")
		}
	}
	clean := path.Clean(slash)
	if clean != slash || !fs.ValidPath(clean) {
		return "", fmt.Errorf("path must be normalized and stay under root")
	}
	return filepath.FromSlash(clean), nil
}

// checkPath rejects symlinks in every existing component. When allowMissingLeaf
// is true, only the final component may not yet exist.
func checkPath(root *os.Root, relative string, allowMissingLeaf bool) error {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	for index := range parts {
		current := filepath.FromSlash(strings.Join(parts[:index+1], "/"))
		info, err := root.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			if allowMissingLeaf && index == len(parts)-1 {
				return nil
			}
			return fmt.Errorf("path component %q does not exist", current)
		}
		if err != nil {
			return fmt.Errorf("inspect path component %q: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path component %q is a symlink", current)
		}
		if index < len(parts)-1 && !info.IsDir() {
			return fmt.Errorf("path component %q is not a directory", current)
		}
	}
	return nil
}

func validateIntentPaths(root *os.Root, intent Intent) error {
	for label, raw := range map[string]string{
		"workdir":      intent.Workdir,
		"receipt_path": intent.ReceiptPath,
		"stdout_path":  intent.StdoutPath,
		"stderr_path":  intent.StderrPath,
	} {
		clean, err := rootedPath(raw)
		if err != nil {
			return err
		}
		allowMissing := label == "stdout_path" || label == "stderr_path"
		if err := checkPath(root, clean, allowMissing); err != nil {
			return fmt.Errorf("native journal: intent.%s: %w", label, err)
		}
		info, err := root.Lstat(clean)
		if err == nil {
			if label == "workdir" && !info.IsDir() {
				return fmt.Errorf("native journal: intent.workdir is not a directory")
			}
			if label != "workdir" && !info.Mode().IsRegular() {
				return fmt.Errorf("native journal: intent.%s is not a regular file", label)
			}
		} else if !errors.Is(err, fs.ErrNotExist) || !allowMissing {
			return fmt.Errorf("native journal: inspect intent.%s: %w", label, err)
		}
	}
	return nil
}

func validateExistingIntentPaths(root *os.Root, intent Intent) error {
	for label, raw := range map[string]string{
		"workdir":      intent.Workdir,
		"receipt_path": intent.ReceiptPath,
		"stdout_path":  intent.StdoutPath,
		"stderr_path":  intent.StderrPath,
	} {
		clean, err := rootedPath(raw)
		if err != nil {
			return fmt.Errorf("native journal: intent.%s: %w", label, err)
		}
		if err := checkExistingPath(root, clean); err != nil {
			return fmt.Errorf("native journal: intent.%s: %w", label, err)
		}
	}
	if err := validateExistingExecutablePath(intent.SentinelExecutable); err != nil {
		return err
	}
	return nil
}

// checkExistingPath rejects any symlink in the existing prefix but allows the
// target or a later parent to have been removed since the intent was captured.
func checkExistingPath(root *os.Root, relative string) error {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	for index := range parts {
		current := filepath.FromSlash(strings.Join(parts[:index+1], "/"))
		info, err := root.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect path component %q: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path component %q is a symlink", current)
		}
		if index < len(parts)-1 && !info.IsDir() {
			return fmt.Errorf("path component %q is not a directory", current)
		}
	}
	return nil
}

func validateExecutable(intent Intent) error {
	if err := validateExistingExecutablePath(intent.SentinelExecutable); err != nil {
		return err
	}
	info, err := os.Stat(intent.SentinelExecutable)
	if err != nil {
		return fmt.Errorf("native journal: inspect Sentinel executable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("native journal: Sentinel executable is not a regular file")
	}
	data, err := os.ReadFile(intent.SentinelExecutable)
	if err != nil {
		return fmt.Errorf("native journal: read Sentinel executable for hash: %w", err)
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != intent.SentinelExecutableSHA256 {
		return fmt.Errorf("native journal: Sentinel executable SHA-256 does not match intent")
	}
	return nil
}

func validateExistingExecutablePath(executable string) error {
	current := string(filepath.Separator)
	volume := filepath.VolumeName(executable)
	pathWithoutVolume := strings.TrimPrefix(executable, volume)
	parts := strings.Split(strings.TrimPrefix(pathWithoutVolume, string(filepath.Separator)), string(filepath.Separator))
	for index, part := range parts {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("native journal: inspect Sentinel executable path %q: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("native journal: Sentinel executable path component %q is a symlink", current)
		}
		if index < len(parts)-1 && !info.IsDir() {
			return fmt.Errorf("native journal: Sentinel executable parent %q is not a directory", current)
		}
	}
	return nil
}

func publish(root *os.Root, target string, data []byte, createOnly bool) error {
	directory := filepath.Dir(target)
	if directory == "." {
		directory = "."
	}
	if err := checkPath(root, filepath.Join(directory, filepath.Base(target)), createOnly); err != nil {
		return fmt.Errorf("native journal: publication target: %w", err)
	}
	var temporaryPath string
	var temporary *os.File
	for attempt := 0; attempt < 8; attempt++ {
		var nonce [12]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return fmt.Errorf("native journal: create temporary name: %w", err)
		}
		name := ".native-journal-" + hex.EncodeToString(nonce[:])
		temporaryPath = filepath.Join(directory, name)
		file, err := root.OpenFile(temporaryPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("native journal: create temporary file: %w", err)
		}
		temporary = file
		break
	}
	if temporary == nil {
		return fmt.Errorf("native journal: could not allocate temporary file")
	}
	defer root.Remove(temporaryPath)
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("native journal: write temporary file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("native journal: sync temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("native journal: close temporary file: %w", err)
	}
	if createOnly {
		if err := root.Link(temporaryPath, target); err != nil {
			return fmt.Errorf("native journal: publish new file %q: %w", target, err)
		}
		if err := root.Remove(temporaryPath); err != nil {
			return fmt.Errorf("native journal: remove publication temporary file: %w", err)
		}
	} else {
		if err := root.Rename(temporaryPath, target); err != nil {
			return fmt.Errorf("native journal: replace %q: %w", target, err)
		}
	}
	directoryFile, err := root.Open(directory)
	if err != nil {
		return fmt.Errorf("native journal: open publication directory: %w", err)
	}
	defer directoryFile.Close()
	if err := directoryFile.Sync(); err != nil {
		return fmt.Errorf("native journal: sync publication directory: %w", err)
	}
	return nil
}
