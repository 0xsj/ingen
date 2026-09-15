// Package run defines Nublar's versioned collection-run artifact.
package run

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ingen/core/ciresult"
	"ingen/nublar/internal/output"
)

const Schema = "ingen.nublar-run/v1"

// Run records one Nublar collection attempt. Producer reports remain inside
// the shared CI envelopes and are not reinterpreted by this package.
type Run struct {
	Schema      string   `json:"schema"`
	RunID       string   `json:"run_id"`
	Workflow    Workflow `json:"workflow"`
	Status      string   `json:"status"`
	ExitCode    int      `json:"exit_code"`
	CreatedAt   string   `json:"created_at"`
	CompletedAt string   `json:"completed_at"`
	Checks      []Check  `json:"checks"`
	Warnings    []Issue  `json:"warnings,omitempty"`
	Errors      []Issue  `json:"errors,omitempty"`
}

type Workflow struct {
	ID   string           `json:"id"`
	File ciresult.FileRef `json:"file"`
}

type Check struct {
	ID       string  `json:"id"`
	Tool     string  `json:"tool"`
	Path     string  `json:"path"`
	Required bool    `json:"required"`
	Status   string  `json:"status"`
	Result   *Result `json:"result,omitempty"`
	Reason   string  `json:"reason,omitempty"`
}

type Result struct {
	Ref      ciresult.FileRef  `json:"ref"`
	Artifact ciresult.Artifact `json:"artifact"`
}

type Issue struct {
	CheckID string `json:"check_id,omitempty"`
	Tool    string `json:"tool,omitempty"`
	Path    string `json:"path,omitempty"`
	Reason  string `json:"reason"`
}

// Validate checks the schema-level fields and the relationships that make a
// run decision meaningful. In particular, a run cannot claim passed while a
// required result is absent or a producer envelope reports an error.
func (r Run) Validate() error {
	if r.Schema != Schema {
		return fmt.Errorf("Nublar run schema must be %s, got %q", Schema, r.Schema)
	}
	if strings.TrimSpace(r.RunID) == "" {
		return fmt.Errorf("Nublar run run_id is required")
	}
	if strings.TrimSpace(r.Workflow.ID) == "" {
		return fmt.Errorf("Nublar run workflow id is required")
	}
	if err := validateFileRef("workflow file", r.Workflow.File); err != nil {
		return err
	}
	createdAt, err := parseTimestamp("created_at", r.CreatedAt)
	if err != nil {
		return err
	}
	completedAt, err := parseTimestamp("completed_at", r.CompletedAt)
	if err != nil {
		return err
	}
	if completedAt.Before(createdAt) {
		return fmt.Errorf("Nublar run completed_at must not be before created_at")
	}
	expectedExitCode, err := ciresult.ExitCodeForStatus(r.Status)
	if err != nil {
		return err
	}
	if r.ExitCode != expectedExitCode {
		return fmt.Errorf("Nublar run status %q must have exit_code %d", r.Status, expectedExitCode)
	}
	if len(r.Checks) == 0 {
		return fmt.Errorf("Nublar run needs at least one check")
	}

	seenIDs := make(map[string]bool, len(r.Checks))
	seenPaths := make(map[string]bool, len(r.Checks))
	computedStatus := "passed"
	presentResults := 0
	hasCheckError := false
	for index, check := range r.Checks {
		if err := validateCheckIdentity(check, index, seenIDs, seenPaths); err != nil {
			return err
		}
		if err := validateCheckState(check); err != nil {
			return fmt.Errorf("Nublar run check %q: %w", check.ID, err)
		}
		if check.Status == "missing" {
			continue
		}
		if check.Status == "error" {
			hasCheckError = true
		}
		if check.Result == nil {
			continue
		}
		presentResults++
		if err := check.Result.validate(check); err != nil {
			return fmt.Errorf("Nublar run check %q: %w", check.ID, err)
		}
		if severity(check.Result.Artifact.Status) > severity(computedStatus) {
			computedStatus = check.Result.Artifact.Status
		}
	}
	for index, issue := range r.Warnings {
		if err := issue.Validate(); err != nil {
			return fmt.Errorf("Nublar run warning %d: %w", index+1, err)
		}
	}
	for index, issue := range r.Errors {
		if err := issue.Validate(); err != nil {
			return fmt.Errorf("Nublar run error %d: %w", index+1, err)
		}
	}
	if presentResults == 0 && len(r.Errors) == 0 && !hasCheckError {
		return fmt.Errorf("Nublar run needs at least one present result artifact or collection error")
	}
	if len(r.Errors) > 0 || hasCheckError {
		computedStatus = "error"
	}
	if r.Status != computedStatus {
		return fmt.Errorf("Nublar run status %q does not match check decisions; want %q", r.Status, computedStatus)
	}
	return nil
}

func validateCheckIdentity(check Check, index int, seenIDs, seenPaths map[string]bool) error {
	if strings.TrimSpace(check.ID) == "" {
		return fmt.Errorf("Nublar run check %d needs an id", index+1)
	}
	if seenIDs[check.ID] {
		return fmt.Errorf("Nublar run check ID %q was duplicated", check.ID)
	}
	seenIDs[check.ID] = true
	if strings.TrimSpace(check.Tool) == "" {
		return fmt.Errorf("Nublar run check %q needs a tool", check.ID)
	}
	if err := validateRelativePath(check.Path); err != nil {
		return fmt.Errorf("Nublar run check %q path: %w", check.ID, err)
	}
	normalizedPath := filepath.Clean(check.Path)
	if seenPaths[normalizedPath] {
		return fmt.Errorf("Nublar run result path %q was duplicated", check.Path)
	}
	seenPaths[normalizedPath] = true
	return nil
}

func validateCheckState(check Check) error {
	switch check.Status {
	case "passed", "failed":
		if check.Result == nil {
			return fmt.Errorf("%s check needs a producer result", check.Status)
		}
	case "error":
		if check.Result == nil && strings.TrimSpace(check.Reason) == "" {
			return fmt.Errorf("error check needs a producer result or reason")
		}
	case "missing":
		if check.Required {
			return fmt.Errorf("required check cannot be missing")
		}
		if check.Result != nil {
			return fmt.Errorf("missing check must not contain a producer result")
		}
		if strings.TrimSpace(check.Reason) == "" {
			return fmt.Errorf("missing check needs a reason")
		}
	default:
		return fmt.Errorf("check has unsupported status %q", check.Status)
	}
	return nil
}

func (r Result) validate(check Check) error {
	if err := validateFileRef("result", r.Ref); err != nil {
		return err
	}
	if filepath.Clean(r.Ref.Path) != filepath.Clean(check.Path) {
		return fmt.Errorf("producer result path is %q, check expects %q", r.Ref.Path, check.Path)
	}
	if err := r.Artifact.Validate(); err != nil {
		return fmt.Errorf("producer artifact: %w", err)
	}
	if r.Artifact.Tool != check.Tool {
		return fmt.Errorf("producer tool is %q, check expects %q", r.Artifact.Tool, check.Tool)
	}
	if r.Artifact.Status != check.Status {
		return fmt.Errorf("producer status is %q, check records %q", r.Artifact.Status, check.Status)
	}
	return nil
}

func (i Issue) Validate() error {
	if strings.TrimSpace(i.Reason) == "" {
		return fmt.Errorf("issue reason must not be empty")
	}
	return nil
}

func validateFileRef(name string, ref ciresult.FileRef) error {
	if err := ciresult.ValidateFileRef(name, &ref); err != nil {
		return err
	}
	if strings.TrimSpace(ref.SHA256) == "" {
		return fmt.Errorf("Nublar run %s sha256 is required", name)
	}
	return nil
}

func validateRelativePath(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("path must not be empty")
	}
	clean := filepath.Clean(raw)
	if filepath.IsAbs(raw) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path must stay inside the artifact root: %q", raw)
	}
	return nil
}

func parseTimestamp(name, value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, fmt.Errorf("Nublar run %s is required", name)
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("Nublar run %s must be RFC3339: %w", name, err)
	}
	return parsed, nil
}

func severity(status string) int {
	switch status {
	case "passed":
		return 0
	case "failed":
		return 1
	case "error":
		return 2
	default:
		return -1
	}
}

// NewID returns an opaque identity for a newly collected Nublar run. The
// format is intentionally not part of the run schema so a future persisted
// identity strategy can evolve without changing artifact semantics.
func NewID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate Nublar run ID: %w", err)
	}
	return "run-" + hex.EncodeToString(raw[:]), nil
}

func WriteJSON(w io.Writer, r Run) error {
	if err := r.Validate(); err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(r)
}

func SaveFile(path string, r Run) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Nublar run: %w", err)
	}
	if err := r.Validate(); err != nil {
		return err
	}
	data = append(data, '\n')
	if err := output.WriteFile(path, data); err != nil {
		return fmt.Errorf("write Nublar run %s: %w", path, err)
	}
	return nil
}

func LoadFile(path string) (Run, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Run{}, fmt.Errorf("read Nublar run %s: %w", path, err)
	}
	var r Run
	if err := json.Unmarshal(contents, &r); err != nil {
		return Run{}, fmt.Errorf("parse Nublar run %s: %w", path, err)
	}
	if err := r.Validate(); err != nil {
		return Run{}, fmt.Errorf("validate Nublar run %s: %w", path, err)
	}
	return r, nil
}
