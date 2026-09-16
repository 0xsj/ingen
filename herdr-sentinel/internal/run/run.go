// Package run defines Sentinel's lifecycle receipt.
//
// The receipt connects workspace activity to immutable artifact references. It
// records orchestration facts only; Sorna and Nublar remain responsible for
// interpreting their own results.
package run

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ingen/core/ciresult"
	"ingen/herdr-sentinel/internal/workspace"
)

const Schema = "ingen.sentinel-run/v1"

type Receipt struct {
	Schema    string        `json:"schema"`
	RunID     string        `json:"run_id"`
	Workspace WorkspaceRef  `json:"workspace"`
	Status    string        `json:"status"`
	CreatedAt string        `json:"created_at"`
	UpdatedAt string        `json:"updated_at"`
	Events    []Event       `json:"events"`
	Artifacts []ArtifactRef `json:"artifacts,omitempty"`
}

type WorkspaceRef struct {
	ID      string           `json:"id"`
	Version int64            `json:"version"`
	File    ciresult.FileRef `json:"file"`
}

type Event struct {
	Sequence    int64    `json:"sequence"`
	SourceID    string   `json:"source_id,omitempty"`
	Type        string   `json:"type"`
	At          string   `json:"at"`
	Role        string   `json:"role,omitempty"`
	Workspace   string   `json:"workspace,omitempty"`
	SessionID   string   `json:"session_id,omitempty"`
	Status      string   `json:"status,omitempty"`
	ArtifactIDs []string `json:"artifact_ids,omitempty"`
	Outcome     string   `json:"outcome,omitempty"`
	Reason      string   `json:"reason,omitempty"`
}

type ArtifactRef struct {
	ID   string           `json:"id"`
	Role string           `json:"role"`
	Kind string           `json:"kind"`
	Ref  ciresult.FileRef `json:"ref"`
}

var statuses = map[string]bool{
	"created":   true,
	"running":   true,
	"completed": true,
	"failed":    true,
	"blocked":   true,
	"cleaned":   true,
}

var eventTypes = map[string]bool{
	"workspace-created": true,
	"worktree-created":  true,
	"role-launched":     true,
	"role-blocked":      true,
	"role-completed":    true,
	"artifact-produced": true,
	"policy-applied":    true,
	"policy-violated":   true,
	"sorna-started":     true,
	"sorna-completed":   true,
	"review-approved":   true,
	"review-waived":     true,
	"cleanup-completed": true,
}

// New creates a receipt whose workspace reference is bound to the exact bytes
// loaded and validated from workspacePath.
func New(workspacePath string, now time.Time) (Receipt, error) {
	workspacePath = filepath.Clean(workspacePath)
	if err := validateRelativePath("workspace file", workspacePath); err != nil {
		return Receipt{}, err
	}
	contents, err := os.ReadFile(workspacePath)
	if err != nil {
		return Receipt{}, fmt.Errorf("read Sentinel workspace %s: %w", workspacePath, err)
	}
	loaded, err := workspace.LoadBytes(workspacePath, contents)
	if err != nil {
		return Receipt{}, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	timestamp := now.UTC().Format(time.RFC3339Nano)
	digest := sha256.Sum256(contents)
	runID, err := newID()
	if err != nil {
		return Receipt{}, err
	}
	receipt := Receipt{
		Schema: Schema,
		RunID:  runID,
		Workspace: WorkspaceRef{
			ID:      loaded.ID,
			Version: loaded.Version,
			File: ciresult.FileRef{
				Path:   workspacePath,
				SHA256: hex.EncodeToString(digest[:]),
			},
		},
		Status:    "created",
		CreatedAt: timestamp,
		UpdatedAt: timestamp,
		Events: []Event{
			{Sequence: 1, Type: "workspace-created", At: timestamp},
		},
	}
	if err := receipt.Validate(); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

// Validate checks receipt structure, chronology, and artifact references. It
// intentionally does not derive a status from events or reinterpret producer
// results.
func (r Receipt) Validate() error {
	if r.Schema != Schema {
		return fmt.Errorf("Sentinel run schema must be %s, got %q", Schema, r.Schema)
	}
	if strings.TrimSpace(r.RunID) == "" {
		return fmt.Errorf("Sentinel run run_id is required")
	}
	if strings.TrimSpace(r.Workspace.ID) == "" {
		return fmt.Errorf("Sentinel run workspace id is required")
	}
	if r.Workspace.Version < 1 {
		return fmt.Errorf("Sentinel run workspace version must be positive")
	}
	if err := validateFileRef("workspace", r.Workspace.File); err != nil {
		return err
	}
	if err := validateRelativePath("workspace file", r.Workspace.File.Path); err != nil {
		return err
	}
	if !statuses[r.Status] {
		return fmt.Errorf("Sentinel run status %q is unsupported", r.Status)
	}
	createdAt, err := parseTimestamp("created_at", r.CreatedAt)
	if err != nil {
		return err
	}
	updatedAt, err := parseTimestamp("updated_at", r.UpdatedAt)
	if err != nil {
		return err
	}
	if updatedAt.Before(createdAt) {
		return fmt.Errorf("Sentinel run updated_at must not be before created_at")
	}
	if len(r.Events) == 0 {
		return fmt.Errorf("Sentinel run needs at least one lifecycle event")
	}
	if r.Events[0].Type != "workspace-created" {
		return fmt.Errorf("Sentinel run first event must be workspace-created")
	}

	artifactIDs := make(map[string]bool, len(r.Artifacts))
	for index, artifact := range r.Artifacts {
		if err := artifact.validate(index, artifactIDs); err != nil {
			return err
		}
	}
	sourceIDs := make(map[string]bool, len(r.Events))
	for index, event := range r.Events {
		if err := event.validate(index, createdAt, updatedAt, artifactIDs); err != nil {
			return err
		}
		if event.SourceID != "" {
			if sourceIDs[event.SourceID] {
				return fmt.Errorf("Sentinel run event source ID %q was duplicated", event.SourceID)
			}
			sourceIDs[event.SourceID] = true
		}
	}
	return nil
}

// AppendEvent adds the next event and moves UpdatedAt to that event's time.
// The receipt is changed only when the resulting receipt remains valid.
func (r *Receipt) AppendEvent(event Event) error {
	if r == nil {
		return fmt.Errorf("Sentinel run receipt is nil")
	}
	if event.Sequence != 0 && event.Sequence != int64(len(r.Events)+1) {
		return fmt.Errorf("Sentinel run event sequence must be %d", len(r.Events)+1)
	}
	if strings.TrimSpace(event.At) == "" {
		event.At = time.Now().UTC().Format(time.RFC3339Nano)
	}
	event.Sequence = int64(len(r.Events) + 1)
	candidate := *r
	candidate.Events = append(append([]Event(nil), r.Events...), event)
	candidate.UpdatedAt = event.At
	if err := candidate.Validate(); err != nil {
		return err
	}
	*r = candidate
	return nil
}

// SetStatus records the current operator-visible lifecycle state without
// pretending that a label is an executable state transition.
func (r *Receipt) SetStatus(status string, at time.Time) error {
	if r == nil {
		return fmt.Errorf("Sentinel run receipt is nil")
	}
	if err := ValidateStatus(status); err != nil {
		return err
	}
	if err := ValidateStatusTransition(r.Status, status); err != nil {
		return err
	}
	if at.IsZero() {
		at = time.Now()
	}
	candidate := *r
	candidate.Status = status
	candidate.UpdatedAt = at.UTC().Format(time.RFC3339Nano)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*r = candidate
	return nil
}

// ValidateStatus checks the operator-visible receipt states accepted by the
// Sentinel lifecycle model.
func ValidateStatus(status string) error {
	if !statuses[status] {
		return fmt.Errorf("Sentinel run status %q is unsupported", status)
	}
	return nil
}

// ValidateStatusTransition rejects status regressions after a terminal
// lifecycle state. It intentionally leaves the non-terminal graph permissive
// until the native Herdr lifecycle contract defines the complete transition
// model.
func ValidateStatusTransition(from, to string) error {
	if err := ValidateStatus(from); err != nil {
		return err
	}
	if err := ValidateStatus(to); err != nil {
		return err
	}
	if from == to {
		return nil
	}
	if terminalStatus(from) {
		if from != "cleaned" && to == "cleaned" {
			return nil
		}
		return fmt.Errorf("Sentinel run status cannot move from terminal %q to %q", from, to)
	}
	return nil
}

func terminalStatus(status string) bool {
	switch status {
	case "completed", "failed", "blocked", "cleaned":
		return true
	default:
		return false
	}
}

// AddFileArtifact adds a reference whose digest is calculated from the same
// bytes that are read from path.
func (r *Receipt) AddFileArtifact(id, role, kind, path string) error {
	artifact, err := fileArtifact(id, role, kind, path)
	if err != nil {
		return err
	}
	return r.AddArtifact(artifact)
}

// RegisterFileArtifact records a file artifact unless the exact same
// identity is already present. It returns false for that idempotent no-op.
func (r *Receipt) RegisterFileArtifact(id, role, kind, path string) (bool, error) {
	artifact, err := fileArtifact(id, role, kind, path)
	if err != nil {
		return false, err
	}
	return r.RegisterArtifact(artifact)
}

func fileArtifact(id, role, kind, path string) (ArtifactRef, error) {
	path = filepath.Clean(path)
	if err := validateRelativePath("artifact file", path); err != nil {
		return ArtifactRef{}, err
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return ArtifactRef{}, fmt.Errorf("read Sentinel artifact %s: %w", path, err)
	}
	digest := sha256.Sum256(contents)
	return ArtifactRef{
		ID:   id,
		Role: role,
		Kind: kind,
		Ref: ciresult.FileRef{
			Path:   path,
			SHA256: hex.EncodeToString(digest[:]),
		},
	}, nil
}

// AddArtifact adds an already-hashed artifact reference.
func (r *Receipt) AddArtifact(artifact ArtifactRef) error {
	if r == nil {
		return fmt.Errorf("Sentinel run receipt is nil")
	}
	candidate := *r
	candidate.Artifacts = append(append([]ArtifactRef(nil), r.Artifacts...), artifact)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*r = candidate
	return nil
}

// RegisterArtifact adds an already-hashed artifact reference unless the
// exact same identity is already present. A reused ID with different content
// or metadata is rejected as a conflict.
func (r *Receipt) RegisterArtifact(artifact ArtifactRef) (bool, error) {
	if r == nil {
		return false, fmt.Errorf("Sentinel run receipt is nil")
	}
	for _, existing := range r.Artifacts {
		if existing.ID != artifact.ID {
			continue
		}
		if existing == artifact {
			return false, nil
		}
		return false, fmt.Errorf("Sentinel run artifact ID %q conflicts with an existing reference", artifact.ID)
	}
	candidate := *r
	candidate.Artifacts = append(append([]ArtifactRef(nil), r.Artifacts...), artifact)
	if err := candidate.Validate(); err != nil {
		return false, err
	}
	*r = candidate
	return true, nil
}

func (a ArtifactRef) validate(index int, seen map[string]bool) error {
	if strings.TrimSpace(a.ID) == "" {
		return fmt.Errorf("Sentinel run artifact %d needs an id", index+1)
	}
	if seen[a.ID] {
		return fmt.Errorf("Sentinel run artifact ID %q was duplicated", a.ID)
	}
	seen[a.ID] = true
	if strings.TrimSpace(a.Role) == "" || strings.TrimSpace(a.Kind) == "" {
		return fmt.Errorf("Sentinel run artifact %q needs a role and kind", a.ID)
	}
	if err := validateFileRef("artifact "+a.ID, a.Ref); err != nil {
		return err
	}
	return validateRelativePath("artifact "+a.ID+" file", a.Ref.Path)
}

func (e Event) validate(index int, createdAt, updatedAt time.Time, artifactIDs map[string]bool) error {
	wantedSequence := int64(index + 1)
	if e.Sequence != wantedSequence {
		return fmt.Errorf("Sentinel run event %d sequence must be %d", index+1, wantedSequence)
	}
	if e.SourceID != "" && strings.TrimSpace(e.SourceID) == "" {
		return fmt.Errorf("Sentinel run event %d source_id must not be blank", index+1)
	}
	if e.SessionID != "" && strings.TrimSpace(e.SessionID) == "" {
		return fmt.Errorf("Sentinel run event %d session_id must not be blank", index+1)
	}
	if e.Status != "" {
		if strings.TrimSpace(e.Status) == "" {
			return fmt.Errorf("Sentinel run event %d status must not be blank", index+1)
		}
		if err := ValidateStatus(e.Status); err != nil {
			return fmt.Errorf("Sentinel run event %d: %w", index+1, err)
		}
	}
	if !eventTypes[e.Type] {
		return fmt.Errorf("Sentinel run event %d type %q is unsupported", index+1, e.Type)
	}
	at, err := parseTimestamp(fmt.Sprintf("event %d at", index+1), e.At)
	if err != nil {
		return err
	}
	if at.Before(createdAt) || at.After(updatedAt) {
		return fmt.Errorf("Sentinel run event %d at must be between created_at and updated_at", index+1)
	}
	if e.Workspace != "" {
		if err := validateRelativePath(fmt.Sprintf("event %d workspace", index+1), e.Workspace); err != nil {
			return err
		}
	}
	seen := make(map[string]bool, len(e.ArtifactIDs))
	for _, artifactID := range e.ArtifactIDs {
		if strings.TrimSpace(artifactID) == "" {
			return fmt.Errorf("Sentinel run event %d has an empty artifact ID", index+1)
		}
		if seen[artifactID] {
			return fmt.Errorf("Sentinel run event %d artifact ID %q was duplicated", index+1, artifactID)
		}
		seen[artifactID] = true
		if !artifactIDs[artifactID] {
			return fmt.Errorf("Sentinel run event %d references unknown artifact %q", index+1, artifactID)
		}
	}
	return nil
}

func validateFileRef(name string, ref ciresult.FileRef) error {
	if err := ciresult.ValidateFileRef(name, &ref); err != nil {
		return err
	}
	if strings.TrimSpace(ref.SHA256) == "" {
		return fmt.Errorf("Sentinel run %s sha256 is required", name)
	}
	return nil
}

func validateRelativePath(name, raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("Sentinel run %s path must not be empty", name)
	}
	clean := filepath.Clean(raw)
	if filepath.IsAbs(raw) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("Sentinel run %s path must stay inside the project root: %q", name, raw)
	}
	return nil
}

func parseTimestamp(name, value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, fmt.Errorf("Sentinel run %s is required", name)
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("Sentinel run %s must be RFC3339: %w", name, err)
	}
	return parsed, nil
}

func newID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate Sentinel run ID: %w", err)
	}
	return "run-" + hex.EncodeToString(raw[:]), nil
}

func WriteJSON(w io.Writer, r Receipt) error {
	if err := r.Validate(); err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(r)
}

func SaveFile(path string, r Receipt) error {
	if err := r.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Sentinel run: %w", err)
	}
	data = append(data, '\n')
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".sentinel-output-*")
	if err != nil {
		return fmt.Errorf("create temporary Sentinel output in %s: %w", directory, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary Sentinel output: %w", err)
	}
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set Sentinel output permissions: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary Sentinel output: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary Sentinel output: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish Sentinel output %s: %w", path, err)
	}
	return nil
}

func LoadFile(path string) (Receipt, error) {
	receipt, _, err := LoadFileSnapshot(path)
	return receipt, err
}

// LoadFileSnapshot reads and validates a receipt once, returning the exact
// bytes that produced the validated value for downstream provenance work.
func LoadFileSnapshot(path string) (Receipt, []byte, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Receipt{}, nil, fmt.Errorf("read Sentinel run %s: %w", path, err)
	}
	receipt, err := loadBytes(path, contents)
	if err != nil {
		return Receipt{}, nil, err
	}
	return receipt, append([]byte(nil), contents...), nil
}

func loadBytes(path string, contents []byte) (Receipt, error) {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var receipt Receipt
	if err := decoder.Decode(&receipt); err != nil {
		return Receipt{}, fmt.Errorf("parse Sentinel run %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Receipt{}, fmt.Errorf("parse Sentinel run %s: multiple JSON values are not supported", path)
		}
		return Receipt{}, fmt.Errorf("parse Sentinel run %s: %w", path, err)
	}
	if err := receipt.Validate(); err != nil {
		return Receipt{}, fmt.Errorf("validate Sentinel run %s: %w", path, err)
	}
	return receipt, nil
}
