// Package delivery defines the provider-neutral view of a Nublar run.
package delivery

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"ingen/core/ciresult"
	"ingen/nublar/internal/run"
)

const Schema = "ingen.nublar-decision/v1"

// Decision is the small, provider-neutral projection that a delivery adapter
// can publish. It intentionally excludes producer-owned reports; an adapter
// can use RunID to retrieve the complete run record when a destination needs
// detailed evidence.
type Decision struct {
	Schema      string      `json:"schema"`
	RunID       string      `json:"run_id"`
	Workflow    Workflow    `json:"workflow"`
	Status      string      `json:"status"`
	ExitCode    int         `json:"exit_code"`
	CreatedAt   string      `json:"created_at"`
	CompletedAt string      `json:"completed_at"`
	Checks      []Check     `json:"checks"`
	Warnings    []run.Issue `json:"warnings,omitempty"`
	Errors      []run.Issue `json:"errors,omitempty"`
}

type Workflow struct {
	ID   string           `json:"id"`
	File ciresult.FileRef `json:"file"`
}

type Check struct {
	ID       string            `json:"id"`
	Tool     string            `json:"tool"`
	Path     string            `json:"path"`
	Required bool              `json:"required"`
	Status   string            `json:"status"`
	Result   *ciresult.FileRef `json:"result,omitempty"`
	Reason   string            `json:"reason,omitempty"`
}

// Project validates record and creates a delivery-safe decision projection.
// It copies references and issue values so callers cannot mutate the source
// run through the projection's slices.
func Project(record run.Run) (Decision, error) {
	if err := record.Validate(); err != nil {
		return Decision{}, err
	}
	decision := Decision{
		Schema:      Schema,
		RunID:       record.RunID,
		Workflow:    Workflow{ID: record.Workflow.ID, File: record.Workflow.File},
		Status:      record.Status,
		ExitCode:    record.ExitCode,
		CreatedAt:   record.CreatedAt,
		CompletedAt: record.CompletedAt,
		Checks:      make([]Check, 0, len(record.Checks)),
		Warnings:    append([]run.Issue(nil), record.Warnings...),
		Errors:      append([]run.Issue(nil), record.Errors...),
	}
	for _, source := range record.Checks {
		check := Check{
			ID:       source.ID,
			Tool:     source.Tool,
			Path:     source.Path,
			Required: source.Required,
			Status:   source.Status,
			Reason:   source.Reason,
		}
		if source.Result != nil {
			ref := source.Result.Ref
			check.Result = &ref
		}
		decision.Checks = append(decision.Checks, check)
	}
	if err := decision.Validate(); err != nil {
		return Decision{}, err
	}
	return decision, nil
}

// Validate checks the portable decision projection after it has been built.
func (d Decision) Validate() error {
	if d.Schema != Schema {
		return fmt.Errorf("Nublar decision schema must be %s, got %q", Schema, d.Schema)
	}
	if strings.TrimSpace(d.RunID) == "" || strings.TrimSpace(d.Workflow.ID) == "" {
		return fmt.Errorf("Nublar decision run_id and workflow id are required")
	}
	if err := ciresult.ValidateFileRef("decision workflow", &d.Workflow.File); err != nil {
		return err
	}
	if _, err := time.Parse(time.RFC3339Nano, d.CreatedAt); err != nil {
		return fmt.Errorf("Nublar decision created_at must be RFC3339: %w", err)
	}
	if _, err := time.Parse(time.RFC3339Nano, d.CompletedAt); err != nil {
		return fmt.Errorf("Nublar decision completed_at must be RFC3339: %w", err)
	}
	expectedExitCode, err := ciresult.ExitCodeForStatus(d.Status)
	if err != nil {
		return err
	}
	if d.ExitCode != expectedExitCode {
		return fmt.Errorf("Nublar decision status %q must have exit_code %d", d.Status, expectedExitCode)
	}
	if len(d.Checks) == 0 {
		return fmt.Errorf("Nublar decision needs at least one check")
	}
	seen := make(map[string]bool, len(d.Checks))
	for _, check := range d.Checks {
		if strings.TrimSpace(check.ID) == "" || strings.TrimSpace(check.Tool) == "" || strings.TrimSpace(check.Path) == "" {
			return fmt.Errorf("Nublar decision check identity is incomplete")
		}
		if seen[check.ID] {
			return fmt.Errorf("Nublar decision check ID %q was duplicated", check.ID)
		}
		seen[check.ID] = true
		switch check.Status {
		case "passed", "failed":
			if check.Result == nil {
				return fmt.Errorf("Nublar decision check %q needs a result reference", check.ID)
			}
		case "error":
			if check.Result == nil && strings.TrimSpace(check.Reason) == "" {
				return fmt.Errorf("Nublar decision check %q needs a result or reason", check.ID)
			}
		case "missing":
			if check.Result != nil || strings.TrimSpace(check.Reason) == "" {
				return fmt.Errorf("Nublar decision missing check %q needs only a reason", check.ID)
			}
		default:
			return fmt.Errorf("Nublar decision check %q has unsupported status %q", check.ID, check.Status)
		}
		if check.Result != nil {
			if err := ciresult.ValidateFileRef("decision result", check.Result); err != nil {
				return err
			}
		}
	}
	for _, issue := range append(append([]run.Issue(nil), d.Warnings...), d.Errors...) {
		if err := issue.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// WriteJSON writes a validated decision projection to w.
func WriteJSON(w io.Writer, decision Decision) error {
	if err := decision.Validate(); err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(decision)
}
