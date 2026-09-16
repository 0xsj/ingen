package adapter

// This file defines the narrow, provider-neutral ingress that a Herdr
// integration can use until the host application's native plugin API is
// finalized. It carries Herdr's event identity and context into Sentinel's
// durable receipt; it does not claim that the event is independently attested.

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	sentinelrun "ingen/herdr-sentinel/internal/run"
)

const HerdrEventSchema = "ingen.herdr-event/v1"

// HerdrEvent is the smallest event contract needed to connect a Herdr
// lifecycle callback to a Sentinel run. Artifact IDs refer to artifacts
// already present in the receipt; the adapter never trusts a callback to
// introduce unverified file hashes.
type HerdrEvent struct {
	Schema           string   `json:"schema"`
	EventID          string   `json:"event_id"`
	RunID            string   `json:"run_id"`
	WorkspaceID      string   `json:"workspace_id"`
	WorkspaceVersion int64    `json:"workspace_version"`
	Type             string   `json:"type"`
	At               string   `json:"at"`
	Role             string   `json:"role,omitempty"`
	Workspace        string   `json:"workspace,omitempty"`
	SessionID        string   `json:"session_id,omitempty"`
	ReceiptStatus    string   `json:"receipt_status,omitempty"`
	ArtifactIDs      []string `json:"artifact_ids,omitempty"`
	Outcome          string   `json:"outcome,omitempty"`
	Reason           string   `json:"reason,omitempty"`
}

func (e HerdrEvent) Validate() error {
	if e.Schema != HerdrEventSchema {
		return fmt.Errorf("Herdr event schema must be %s, got %q", HerdrEventSchema, e.Schema)
	}
	for name, value := range map[string]string{
		"event_id":     e.EventID,
		"run_id":       e.RunID,
		"workspace_id": e.WorkspaceID,
		"type":         e.Type,
		"at":           e.At,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("Herdr event %s is required", name)
		}
	}
	if e.WorkspaceVersion < 1 {
		return fmt.Errorf("Herdr event workspace_version must be positive")
	}
	if _, err := time.Parse(time.RFC3339Nano, e.At); err != nil {
		return fmt.Errorf("Herdr event at must be RFC3339: %w", err)
	}
	if e.ReceiptStatus != "" {
		if strings.TrimSpace(e.ReceiptStatus) == "" {
			return fmt.Errorf("Herdr event receipt_status must not be blank")
		}
		if err := sentinelrun.ValidateStatus(e.ReceiptStatus); err != nil {
			return fmt.Errorf("Herdr event: %w", err)
		}
	}
	seen := make(map[string]bool, len(e.ArtifactIDs))
	for _, artifactID := range e.ArtifactIDs {
		if strings.TrimSpace(artifactID) == "" {
			return fmt.Errorf("Herdr event artifact_ids must not contain an empty ID")
		}
		if seen[artifactID] {
			return fmt.Errorf("Herdr event artifact ID %q was duplicated", artifactID)
		}
		seen[artifactID] = true
	}
	return nil
}

// ApplyHerdrEvent appends one validated callback to receipt. Replaying the
// same event ID with the same payload is a no-op, which lets Herdr retry after
// an interrupted write. Reusing an ID for different content is rejected.
func ApplyHerdrEvent(receipt *sentinelrun.Receipt, event HerdrEvent) (bool, error) {
	return applyHerdrEvent(receipt, event, "", false)
}

// ApplyHerdrEventWithRoot applies a Herdr callback and verifies every
// referenced receipt artifact against its current bytes under root before the
// receipt is changed. Native hosts should use this form at the filesystem
// boundary; ApplyHerdrEvent remains useful for pure translation tests.
func ApplyHerdrEventWithRoot(receipt *sentinelrun.Receipt, event HerdrEvent, root string) (bool, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	return applyHerdrEvent(receipt, event, root, true)
}

// ApplyHerdrEventsWithRoot applies a JSONL-sized batch to a receipt copy and
// publishes it only when every event succeeds. The returned count excludes
// idempotent replays.
func ApplyHerdrEventsWithRoot(receipt *sentinelrun.Receipt, events []HerdrEvent, root string) (int, error) {
	if receipt == nil {
		return 0, fmt.Errorf("apply Herdr events: Sentinel receipt is nil")
	}
	if len(events) == 0 {
		return 0, fmt.Errorf("apply Herdr events: event batch is empty")
	}
	next := *receipt
	appended := 0
	for index, event := range events {
		changed, err := ApplyHerdrEventWithRoot(&next, event, root)
		if err != nil {
			return 0, fmt.Errorf("apply Herdr events: event %d: %w", index+1, err)
		}
		if changed {
			appended++
		}
	}
	*receipt = next
	return appended, nil
}

// ApplyHerdrEventFile applies one callback under Sentinel's receipt lock.
// The receipt is not rewritten for an idempotent replay.
func ApplyHerdrEventFile(receiptPath string, event HerdrEvent, root string) (bool, error) {
	return sentinelrun.UpdateFile(receiptPath, func(receipt *sentinelrun.Receipt) (bool, error) {
		return ApplyHerdrEventWithRoot(receipt, event, root)
	})
}

// ApplyHerdrEventsFile applies a callback batch under one receipt lock and
// publishes it only after the complete batch succeeds.
func ApplyHerdrEventsFile(receiptPath string, events []HerdrEvent, root string) (int, error) {
	var appended int
	_, err := sentinelrun.UpdateFile(receiptPath, func(receipt *sentinelrun.Receipt) (bool, error) {
		count, err := ApplyHerdrEventsWithRoot(receipt, events, root)
		appended = count
		return count > 0, err
	})
	return appended, err
}

func applyHerdrEvent(receipt *sentinelrun.Receipt, event HerdrEvent, root string, verifyArtifacts bool) (bool, error) {
	if receipt == nil {
		return false, fmt.Errorf("apply Herdr event: Sentinel receipt is nil")
	}
	if err := event.Validate(); err != nil {
		return false, fmt.Errorf("apply Herdr event: %w", err)
	}
	if event.RunID != receipt.RunID {
		return false, fmt.Errorf("apply Herdr event: run ID %q does not match receipt %q", event.RunID, receipt.RunID)
	}
	if event.WorkspaceID != receipt.Workspace.ID || event.WorkspaceVersion != receipt.Workspace.Version {
		return false, fmt.Errorf("apply Herdr event: workspace %q v%d does not match receipt %q v%d", event.WorkspaceID, event.WorkspaceVersion, receipt.Workspace.ID, receipt.Workspace.Version)
	}
	if verifyArtifacts {
		if err := verifyEventArtifacts(receipt, event.ArtifactIDs, root); err != nil {
			return false, fmt.Errorf("apply Herdr event: %w", err)
		}
	}
	candidate := sentinelrun.Event{
		SourceID:    event.EventID,
		Type:        event.Type,
		At:          event.At,
		Role:        event.Role,
		Workspace:   event.Workspace,
		SessionID:   event.SessionID,
		Status:      event.ReceiptStatus,
		ArtifactIDs: append([]string(nil), event.ArtifactIDs...),
		Outcome:     event.Outcome,
		Reason:      event.Reason,
	}
	for _, existing := range receipt.Events {
		if existing.SourceID != event.EventID {
			continue
		}
		if sameEvent(existing, candidate) {
			return false, nil
		}
		return false, fmt.Errorf("apply Herdr event: event ID %q was already used for different content", event.EventID)
	}
	eventAt, err := time.Parse(time.RFC3339Nano, event.At)
	if err != nil {
		return false, fmt.Errorf("apply Herdr event: parse event timestamp: %w", err)
	}
	lastAt, err := time.Parse(time.RFC3339Nano, receipt.UpdatedAt)
	if err != nil {
		return false, fmt.Errorf("apply Herdr event: parse receipt timestamp: %w", err)
	}
	if eventAt.Before(lastAt) {
		return false, fmt.Errorf("apply Herdr event: event timestamp %s is before receipt update %s", event.At, receipt.UpdatedAt)
	}
	next := *receipt
	if err := next.AppendEvent(candidate); err != nil {
		return false, fmt.Errorf("apply Herdr event: %w", err)
	}
	if event.ReceiptStatus != "" {
		if err := sentinelrun.ValidateStatusTransition(receipt.Status, event.ReceiptStatus); err != nil {
			return false, fmt.Errorf("apply Herdr event: update receipt status: %w", err)
		}
		if err := next.SetStatus(event.ReceiptStatus, eventAt); err != nil {
			return false, fmt.Errorf("apply Herdr event: update receipt status: %w", err)
		}
	}
	*receipt = next
	return true, nil
}

func verifyEventArtifacts(receipt *sentinelrun.Receipt, artifactIDs []string, root string) error {
	for _, artifactID := range artifactIDs {
		var reference *sentinelrun.ArtifactRef
		for index := range receipt.Artifacts {
			if receipt.Artifacts[index].ID == artifactID {
				reference = &receipt.Artifacts[index]
				break
			}
		}
		if reference == nil {
			return fmt.Errorf("event references unknown artifact %q", artifactID)
		}
		path := filepath.Join(root, filepath.Clean(reference.Ref.Path))
		contents, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read artifact %q at %s: %w", artifactID, reference.Ref.Path, err)
		}
		digest := sha256.Sum256(contents)
		actual := hex.EncodeToString(digest[:])
		if !strings.EqualFold(actual, reference.Ref.SHA256) {
			return fmt.Errorf("artifact %q sha256 mismatch: expected %s, got %s", artifactID, reference.Ref.SHA256, actual)
		}
	}
	return nil
}

func sameEvent(left, right sentinelrun.Event) bool {
	if left.SourceID != right.SourceID || left.Type != right.Type || left.At != right.At || left.Role != right.Role || left.Workspace != right.Workspace || left.SessionID != right.SessionID || left.Status != right.Status || left.Outcome != right.Outcome || left.Reason != right.Reason {
		return false
	}
	if len(left.ArtifactIDs) != len(right.ArtifactIDs) {
		return false
	}
	for index := range left.ArtifactIDs {
		if left.ArtifactIDs[index] != right.ArtifactIDs[index] {
			return false
		}
	}
	return true
}

func LoadHerdrEvent(path string) (HerdrEvent, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return HerdrEvent{}, fmt.Errorf("read Herdr event %s: %w", path, err)
	}
	event, err := parseHerdrEvent(contents, path)
	if err != nil {
		return HerdrEvent{}, err
	}
	return event, nil
}

// LoadHerdrEventStream loads newline-delimited ingen.herdr-event/v1 objects.
// Empty lines are ignored; each non-empty line must contain exactly one
// object. The stream is parsed before a caller is allowed to mutate a receipt.
func LoadHerdrEventStream(path string) ([]HerdrEvent, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read Herdr event stream %s: %w", path, err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	events := make([]HerdrEvent, 0)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		contents := bytes.TrimSpace(scanner.Bytes())
		if len(contents) == 0 {
			continue
		}
		event, err := parseHerdrEvent(contents, fmt.Sprintf("%s line %d", path, lineNumber))
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read Herdr event stream %s: %w", path, err)
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("read Herdr event stream %s: no events found", path)
	}
	return events, nil
}

func parseHerdrEvent(contents []byte, name string) (HerdrEvent, error) {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var event HerdrEvent
	if err := decoder.Decode(&event); err != nil {
		return HerdrEvent{}, fmt.Errorf("parse Herdr event %s: %w", name, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return HerdrEvent{}, fmt.Errorf("parse Herdr event %s: multiple JSON values are not supported", name)
		}
		return HerdrEvent{}, fmt.Errorf("parse Herdr event %s: %w", name, err)
	}
	if err := event.Validate(); err != nil {
		return HerdrEvent{}, fmt.Errorf("validate Herdr event %s: %w", name, err)
	}
	return event, nil
}
