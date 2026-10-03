// Package session provides Sentinel's local process session provider.
//
// The provider creates a role workspace, runs one command, captures its
// output, and records the handoff in a Sentinel receipt. It deliberately
// reports declaration-only enforcement: the operating system process is not
// sandboxed by this package.
package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"time"

	"ingen/core/ciresult"
	"ingen/herdr-sentinel/internal/capability"
	"ingen/herdr-sentinel/internal/run"
)

const Schema = "ingen.sentinel-session/v1"

type Record struct {
	Schema      string   `json:"schema"`
	SessionID   string   `json:"session_id"`
	RunID       string   `json:"run_id"`
	WorkspaceID string   `json:"workspace_id"`
	RoleID      string   `json:"role_id"`
	RoleKind    string   `json:"role_kind"`
	Workspace   string   `json:"workspace"`
	Command     []string `json:"command"`
	Status      string   `json:"status"`
	Enforcement string   `json:"enforcement"`
	Assurance   string   `json:"assurance"`
	CreatedAt   string   `json:"created_at"`
	StartedAt   string   `json:"started_at,omitempty"`
	FinishedAt  string   `json:"finished_at,omitempty"`
	ExitCode    int      `json:"exit_code,omitempty"`
	Stdout      string   `json:"stdout"`
	Stderr      string   `json:"stderr"`
	Error       string   `json:"error,omitempty"`
}

type Request struct {
	Root          string
	WorkspacePath string
	ReceiptPath   string
	RoleID        string
	OutputPath    string
	StdoutPath    string
	StderrPath    string
	Command       []string
	Now           time.Time
}

// Spawn launches one local role process and records its lifecycle in the
// supplied Sentinel receipt. A non-zero child exit is returned as an
// *os/exec.ExitError after the failed session has still been recorded.
func Spawn(request Request) (Record, error) {
	if len(request.Command) == 0 || strings.TrimSpace(request.Command[0]) == "" {
		return Record{}, fmt.Errorf("Sentinel session command is required")
	}
	root, err := absoluteRoot(request.Root)
	if err != nil {
		return Record{}, err
	}
	if strings.TrimSpace(request.WorkspacePath) == "" {
		return Record{}, fmt.Errorf("Sentinel session workspace path is required")
	}
	if strings.TrimSpace(request.ReceiptPath) == "" {
		return Record{}, fmt.Errorf("Sentinel session receipt path is required")
	}
	workspacePath, err := relativePath("workspace", request.WorkspacePath)
	if err != nil {
		return Record{}, err
	}
	receiptPath, err := relativePath("receipt", request.ReceiptPath)
	if err != nil {
		return Record{}, err
	}
	plan, err := capability.FromFileUnderRoot(root, workspacePath)
	if err != nil {
		return Record{}, err
	}
	role, err := findRole(plan, request.RoleID)
	if err != nil {
		return Record{}, err
	}

	receiptFile, err := run.ResolveFileRefUnderRoot(root, ciresult.FileRef{Path: receiptPath})
	if err != nil {
		return Record{}, fmt.Errorf("resolve Sentinel receipt: %w", err)
	}
	receipt, err := run.LoadFile(receiptFile)
	if err != nil {
		return Record{}, fmt.Errorf("load Sentinel receipt: %w", err)
	}
	if receipt.Workspace.ID != plan.Workspace.ID || receipt.Workspace.Version != plan.Workspace.Version || receipt.Workspace.File != plan.Workspace.Manifest {
		return Record{}, fmt.Errorf("Sentinel receipt does not match the capability plan workspace")
	}

	sessionID, err := newID()
	if err != nil {
		return Record{}, err
	}
	recordPath, err := normalizePath(request.OutputPath, filepath.Join(".ingen", "artifacts", "sessions", sessionID+".json"))
	if err != nil {
		return Record{}, err
	}
	stdoutPath, err := normalizePath(request.StdoutPath, filepath.Join(".ingen", "artifacts", "sessions", sessionID+".stdout.log"))
	if err != nil {
		return Record{}, err
	}
	stderrPath, err := normalizePath(request.StderrPath, filepath.Join(".ingen", "artifacts", "sessions", sessionID+".stderr.log"))
	if err != nil {
		return Record{}, err
	}
	roleWorkspace, err := normalizePath(role.Workspace, role.Workspace)
	if err != nil {
		return Record{}, err
	}
	roleWorkspacePath := filepath.Join(root, roleWorkspace)
	if err := run.ValidateDirectoryPathUnderRoot(root, roleWorkspace); err != nil {
		return Record{}, err
	}
	outputs := []string{recordPath, stdoutPath, stderrPath}
	seenOutputs := make(map[string]bool, len(outputs))
	for _, path := range outputs {
		if seenOutputs[path] {
			return Record{}, fmt.Errorf("Sentinel session artifact paths must be distinct: %s", path)
		}
		seenOutputs[path] = true
		if err := run.ValidatePathUnderRoot(root, path); err != nil {
			return Record{}, err
		}
		if _, err := os.Lstat(filepath.Join(root, path)); err == nil {
			return Record{}, fmt.Errorf("refusing to overwrite existing Sentinel session artifact %s", path)
		} else if !os.IsNotExist(err) {
			return Record{}, fmt.Errorf("check Sentinel session artifact %s: %w", path, err)
		}
	}
	if err := os.MkdirAll(roleWorkspacePath, 0o755); err != nil {
		return Record{}, fmt.Errorf("create Sentinel role workspace: %w", err)
	}
	for _, path := range outputs {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755); err != nil {
			return Record{}, fmt.Errorf("create Sentinel session artifact directory: %w", err)
		}
	}

	now := request.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	createdAt := now.UTC().Format(time.RFC3339Nano)
	record := Record{
		Schema:      Schema,
		SessionID:   sessionID,
		RunID:       receipt.RunID,
		WorkspaceID: plan.Workspace.ID,
		RoleID:      role.ID,
		RoleKind:    role.Kind,
		Workspace:   role.Workspace,
		Command:     append([]string(nil), request.Command...),
		Status:      "pending",
		Enforcement: plan.Enforcement,
		Assurance:   plan.Assurance,
		CreatedAt:   createdAt,
		Stdout:      stdoutPath,
		Stderr:      stderrPath,
	}
	if err := record.Validate(); err != nil {
		return Record{}, err
	}
	stdout, err := os.OpenFile(filepath.Join(root, stdoutPath), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return Record{}, fmt.Errorf("create Sentinel session stdout: %w", err)
	}
	stderr, err := os.OpenFile(filepath.Join(root, stderrPath), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		_ = stdout.Close()
		_ = os.Remove(filepath.Join(root, stdoutPath))
		return Record{}, fmt.Errorf("create Sentinel session stderr: %w", err)
	}
	if err := saveNewFile(filepath.Join(root, recordPath), record); err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		_ = os.Remove(filepath.Join(root, stdoutPath))
		_ = os.Remove(filepath.Join(root, stderrPath))
		return Record{}, err
	}

	_, err = run.UpdateFile(receiptFile, func(loaded *run.Receipt) (bool, error) {
		if loaded.Workspace.ID != plan.Workspace.ID || loaded.Workspace.Version != plan.Workspace.Version || loaded.Workspace.File != plan.Workspace.Manifest {
			return false, fmt.Errorf("Sentinel receipt does not match the capability plan workspace")
		}
		if err := loaded.SetStatus("running", now); err != nil {
			return false, err
		}
		return true, loaded.AppendEvent(run.Event{
			Type:      "role-launched",
			At:        createdAt,
			Role:      role.ID,
			Workspace: role.Workspace,
			SessionID: sessionID,
			Status:    "running",
			Outcome:   "local-process-started",
		})
	})
	if err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		_ = os.Remove(filepath.Join(root, recordPath))
		_ = os.Remove(filepath.Join(root, stdoutPath))
		_ = os.Remove(filepath.Join(root, stderrPath))
		return Record{}, fmt.Errorf("record Sentinel session launch: %w", err)
	}
	defer stdout.Close()
	defer stderr.Close()

	startedAt := time.Now().UTC()
	record.StartedAt = startedAt.Format(time.RFC3339Nano)
	record.Status = "running"
	if err := SaveFile(filepath.Join(root, recordPath), record); err != nil {
		return Record{}, err
	}

	command := osexec.Command(request.Command[0], request.Command[1:]...)
	command.Dir = roleWorkspacePath
	command.Stdin = os.Stdin
	command.Stdout = stdout
	command.Stderr = stderr
	command.Env = append(os.Environ(),
		"INGEN_SENTINEL_RUN_ID="+receipt.RunID,
		"INGEN_SENTINEL_WORKSPACE_ID="+plan.Workspace.ID,
		"INGEN_SENTINEL_SESSION_ID="+sessionID,
		"INGEN_SENTINEL_ROLE_ID="+role.ID,
		"INGEN_SENTINEL_ROLE_KIND="+role.Kind,
		"INGEN_SENTINEL_CAPABILITY_ENFORCEMENT="+plan.Enforcement,
	)
	processErr := command.Run()
	finishedAt := time.Now().UTC()
	record.FinishedAt = finishedAt.Format(time.RFC3339Nano)
	record.ExitCode = 0
	record.Status = "completed"
	if processErr != nil {
		record.Status = "failed"
		record.ExitCode = 1
		var exitErr *osexec.ExitError
		if errors.As(processErr, &exitErr) {
			record.ExitCode = exitErr.ExitCode()
		} else {
			record.Error = processErr.Error()
		}
	}
	if err := SaveFile(filepath.Join(root, recordPath), record); err != nil {
		return Record{}, err
	}

	artifactIDs := []string{"session-" + sessionID, "session-" + sessionID + "-stdout", "session-" + sessionID + "-stderr"}
	_, updateErr := run.UpdateFile(receiptFile, func(loaded *run.Receipt) (bool, error) {
		if _, err := loaded.RegisterFileArtifactUnderRoot(artifactIDs[0], role.ID, "sentinel-session", root, recordPath); err != nil {
			return false, err
		}
		if _, err := loaded.RegisterFileArtifactUnderRoot(artifactIDs[1], role.ID, "session-stdout", root, stdoutPath); err != nil {
			return false, err
		}
		if _, err := loaded.RegisterFileArtifactUnderRoot(artifactIDs[2], role.ID, "session-stderr", root, stderrPath); err != nil {
			return false, err
		}
		if err := loaded.AppendEvent(run.Event{
			Type:        "role-completed",
			At:          record.FinishedAt,
			Role:        role.ID,
			Workspace:   role.Workspace,
			SessionID:   sessionID,
			Status:      record.Status,
			ArtifactIDs: artifactIDs,
			Outcome:     sessionOutcome(record),
			Reason:      record.Error,
		}); err != nil {
			return false, err
		}
		if record.Status == "failed" {
			if err := loaded.SetStatus("failed", finishedAt); err != nil {
				return false, err
			}
		}
		return true, nil
	})
	if updateErr != nil {
		return record, fmt.Errorf("record Sentinel session completion: %w", updateErr)
	}
	if processErr != nil {
		return record, processErr
	}
	return record, nil
}

func (r Record) Validate() error {
	if r.Schema != Schema {
		return fmt.Errorf("Sentinel session schema must be %s, got %q", Schema, r.Schema)
	}
	for name, value := range map[string]string{
		"session_id": r.SessionID, "run_id": r.RunID, "workspace_id": r.WorkspaceID,
		"role_id": r.RoleID, "role_kind": r.RoleKind, "workspace": r.Workspace,
		"status": r.Status, "enforcement": r.Enforcement, "assurance": r.Assurance,
		"created_at": r.CreatedAt, "stdout": r.Stdout, "stderr": r.Stderr,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("Sentinel session %s is required", name)
		}
	}
	if len(r.Command) == 0 || strings.TrimSpace(r.Command[0]) == "" {
		return fmt.Errorf("Sentinel session command is required")
	}
	if r.Status != "pending" && r.Status != "running" && r.Status != "completed" && r.Status != "failed" {
		return fmt.Errorf("Sentinel session status %q is unsupported", r.Status)
	}
	if r.Enforcement != "declaration-only" || r.Assurance != "unverified" {
		return fmt.Errorf("Sentinel session must preserve declaration-only, unverified assurance")
	}
	for name, value := range map[string]string{"workspace": r.Workspace, "stdout": r.Stdout, "stderr": r.Stderr} {
		if _, err := relativePath(name, value); err != nil {
			return err
		}
	}
	if _, err := time.Parse(time.RFC3339Nano, r.CreatedAt); err != nil {
		return fmt.Errorf("Sentinel session created_at must be RFC3339: %w", err)
	}
	for name, value := range map[string]string{"started_at": r.StartedAt, "finished_at": r.FinishedAt} {
		if value == "" {
			continue
		}
		if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
			return fmt.Errorf("Sentinel session %s must be RFC3339: %w", name, err)
		}
	}
	return nil
}

func SaveFile(path string, record Record) error {
	if err := record.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Sentinel session: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write Sentinel session %s: %w", path, err)
	}
	return nil
}

func saveNewFile(path string, record Record) error {
	if err := record.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Sentinel session: %w", err)
	}
	data = append(data, '\n')
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create Sentinel session %s: %w", path, err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("write Sentinel session %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close Sentinel session %s: %w", path, err)
	}
	return nil
}

func LoadFile(path string) (Record, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Record{}, fmt.Errorf("read Sentinel session %s: %w", path, err)
	}
	var record Record
	if err := json.Unmarshal(contents, &record); err != nil {
		return Record{}, fmt.Errorf("parse Sentinel session %s: %w", path, err)
	}
	if err := record.Validate(); err != nil {
		return Record{}, fmt.Errorf("invalid Sentinel session %s: %w", path, err)
	}
	return record, nil
}

func WriteJSON(writer io.Writer, record Record) error {
	if err := record.Validate(); err != nil {
		return err
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(record)
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

func absoluteRoot(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		raw = "."
	}
	root, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("resolve Sentinel session root: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("read Sentinel session root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("Sentinel session root %s is not a directory", root)
	}
	return root, nil
}

func relativePath(name, raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("Sentinel session %s path is required", name)
	}
	if filepath.IsAbs(raw) {
		return "", fmt.Errorf("Sentinel session %s path must be relative to the project root: %q", name, raw)
	}
	clean := filepath.Clean(raw)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("Sentinel session %s path must stay inside the project root: %q", name, raw)
	}
	return clean, nil
}

func normalizePath(raw, fallback string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		raw = fallback
	}
	return relativePath("artifact", raw)
}

func sessionOutcome(record Record) string {
	if record.Status == "completed" {
		return "completed"
	}
	return fmt.Sprintf("exit-code-%d", record.ExitCode)
}

func newID() (string, error) {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate Sentinel session ID: %w", err)
	}
	return "session-" + hex.EncodeToString(bytes), nil
}
