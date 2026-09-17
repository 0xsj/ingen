package sattler

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"ingen/core/ciresult"
)

// NublarRunSchema is the collection-run schema understood by this adapter.
const NublarRunSchema = "ingen.nublar-run/v1"

// NublarWorkflowSummary identifies the workflow used for a collection run.
type NublarWorkflowSummary struct {
	ID   string           `json:"id"`
	File ciresult.FileRef `json:"file"`
}

// NublarCorrelationSummary identifies the external attempt associated with a
// Nublar run when the coordinator records one.
type NublarCorrelationSummary struct {
	System  string `json:"system"`
	ID      string `json:"id"`
	Attempt int64  `json:"attempt"`
}

// NublarCheckSummary contains the coordinator-owned state of one check.
// Nested producer results remain opaque to this adapter.
type NublarCheckSummary struct {
	Tool     string `json:"tool"`
	Required bool   `json:"required"`
	Status   string `json:"status"`
	Reason   string `json:"reason,omitempty"`
}

// NublarRunSummary contains the run fields Sattler can compare directly.
type NublarRunSummary struct {
	Path        string                        `json:"path,omitempty"`
	Schema      string                        `json:"schema"`
	RunID       string                        `json:"run_id"`
	Workflow    NublarWorkflowSummary         `json:"workflow"`
	Correlation *NublarCorrelationSummary     `json:"correlation,omitempty"`
	Status      string                        `json:"status"`
	ExitCode    int                           `json:"exit_code"`
	CreatedAt   string                        `json:"created_at,omitempty"`
	CompletedAt string                        `json:"completed_at,omitempty"`
	Checks      map[string]NublarCheckSummary `json:"checks"`
}

// NublarRunComparison is a deterministic comparison at the Nublar
// coordination boundary.
type NublarRunComparison struct {
	Schema               string           `json:"schema"`
	Compatible           bool             `json:"compatible"`
	CompatibilityReasons []string         `json:"compatibility_reasons,omitempty"`
	Before               NublarRunSummary `json:"before"`
	After                NublarRunSummary `json:"after"`
	Transition           StateTransition  `json:"transition"`
	ChangeIDFilter       []string         `json:"change_id_filter,omitempty"`
	Changes              []Change         `json:"changes,omitempty"`
	ChangeSummary        ChangeSummary    `json:"change_summary"`
}

const nublarRunComparisonSchema = "ingen.sattler-nublar-run-comparison/v0"

// Validate checks the compatibility-treated Nublar comparison projection.
// Nested producer results are intentionally not part of this contract.
func (report NublarRunComparison) Validate() error {
	if report.Schema != nublarRunComparisonSchema {
		return fmt.Errorf("Nublar comparison schema must be %s, got %q", nublarRunComparisonSchema, report.Schema)
	}
	if err := validateNublarRunSummary("before", report.Before); err != nil {
		return err
	}
	if err := validateNublarRunSummary("after", report.After); err != nil {
		return err
	}
	if report.Transition.Field != "status" {
		return fmt.Errorf("Nublar comparison transition field must be status, got %q", report.Transition.Field)
	}
	return validateBundleAdapterEnvelope("nublar_run", nublarRunComparisonSchema, report.Schema, report.Compatible, report.CompatibilityReasons, report.Transition, report.ChangeIDFilter, report.Changes, report.ChangeSummary, nil)
}

func validateNublarRunSummary(side string, summary NublarRunSummary) error {
	if summary.Schema != NublarRunSchema {
		return fmt.Errorf("Nublar %s schema must be %s, got %q", side, NublarRunSchema, summary.Schema)
	}
	if summary.Path != "" && strings.TrimSpace(summary.Path) == "" {
		return fmt.Errorf("Nublar %s path cannot be empty when present", side)
	}
	if strings.TrimSpace(summary.RunID) == "" {
		return fmt.Errorf("Nublar %s needs a run ID", side)
	}
	if strings.TrimSpace(summary.Workflow.ID) == "" {
		return fmt.Errorf("Nublar %s needs a workflow ID", side)
	}
	if strings.TrimSpace(summary.Workflow.File.SHA256) == "" {
		return fmt.Errorf("Nublar %s needs a workflow file sha256", side)
	}
	if err := ciresult.ValidateFileRef("Nublar "+side+" workflow file", &summary.Workflow.File); err != nil {
		return err
	}
	if summary.Correlation != nil {
		if strings.TrimSpace(summary.Correlation.System) == "" || strings.TrimSpace(summary.Correlation.ID) == "" || summary.Correlation.Attempt < 1 {
			return fmt.Errorf("Nublar %s has an invalid correlation", side)
		}
	}
	if strings.TrimSpace(summary.Status) == "" || summary.ExitCode < 0 {
		return fmt.Errorf("Nublar %s needs a status and nonnegative exit code", side)
	}
	for name, timestamp := range map[string]string{"created_at": summary.CreatedAt, "completed_at": summary.CompletedAt} {
		if timestamp == "" {
			continue
		}
		if _, err := time.Parse(time.RFC3339Nano, timestamp); err != nil {
			return fmt.Errorf("Nublar %s %s must be RFC3339: %w", side, name, err)
		}
	}
	if len(summary.Checks) == 0 {
		return fmt.Errorf("Nublar %s needs at least one check", side)
	}
	for checkID, check := range summary.Checks {
		if strings.TrimSpace(checkID) == "" || strings.TrimSpace(check.Tool) == "" || strings.TrimSpace(check.Status) == "" {
			return fmt.Errorf("Nublar %s contains an invalid check %q", side, checkID)
		}
	}
	return nil
}

type nublarRunDocument struct {
	Schema   string `json:"schema"`
	RunID    string `json:"run_id"`
	Workflow struct {
		ID   string           `json:"id"`
		File ciresult.FileRef `json:"file"`
	} `json:"workflow"`
	Correlation *struct {
		System  string `json:"system"`
		ID      string `json:"id"`
		Attempt int64  `json:"attempt"`
	} `json:"correlation,omitempty"`
	Status      string `json:"status"`
	ExitCode    int    `json:"exit_code"`
	CreatedAt   string `json:"created_at"`
	CompletedAt string `json:"completed_at"`
	Checks      []struct {
		ID       string `json:"id"`
		Tool     string `json:"tool"`
		Required bool   `json:"required"`
		Status   string `json:"status"`
		Reason   string `json:"reason"`
	} `json:"checks"`
}

// CompareNublarRunFiles loads and compares two Nublar collection runs.
func CompareNublarRunFiles(beforePath, afterPath string) (NublarRunComparison, error) {
	before, err := loadNublarRun(beforePath)
	if err != nil {
		return NublarRunComparison{}, err
	}
	after, err := loadNublarRun(afterPath)
	if err != nil {
		return NublarRunComparison{}, err
	}
	report := compareNublarRuns(before, after)
	report.Before.Path = beforePath
	report.After.Path = afterPath
	return report, nil
}

// WriteNublarJSON writes the compatibility-treated machine-readable Nublar
// comparison.
func WriteNublarJSON(w io.Writer, report NublarRunComparison) error {
	if err := report.Validate(); err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

// WriteNublarText writes a compact Nublar run comparison.
func WriteNublarText(w io.Writer, report NublarRunComparison) error {
	if _, err := fmt.Fprintf(w, "Sattler Nublar run comparison\n  before: %s (%s)\n  after:  %s (%s)\n  compatible: %t\n  transition: %s\n", report.Before.Path, report.Before.Status, report.After.Path, report.After.Status, report.Compatible, report.Transition); err != nil {
		return err
	}
	if len(report.ChangeIDFilter) > 0 {
		if _, err := fmt.Fprintf(w, "  change ID filter: %s\n", strings.Join(report.ChangeIDFilter, ", ")); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "  time: %s -> %s; completed: %s -> %s\n", report.Before.CreatedAt, report.After.CreatedAt, report.Before.CompletedAt, report.After.CompletedAt); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  change summary: %s\n", report.ChangeSummary); err != nil {
		return err
	}
	if len(report.CompatibilityReasons) > 0 {
		if _, err := fmt.Fprintln(w, "  compatibility reasons:"); err != nil {
			return err
		}
		for _, reason := range report.CompatibilityReasons {
			if _, err := fmt.Fprintf(w, "    - %s\n", reason); err != nil {
				return err
			}
		}
	}
	if len(report.Changes) == 0 {
		_, err := fmt.Fprintln(w, "  changes: none observable at the Nublar boundary")
		return err
	}
	if _, err := fmt.Fprintln(w, "  changes:"); err != nil {
		return err
	}
	for _, change := range report.Changes {
		if _, err := fmt.Fprintf(w, "    - %s %s (id=%s): %s -> %s\n", change.Category, change.Field, change.StableID(), displayValue(change.Before), displayValue(change.After)); err != nil {
			return err
		}
	}
	return nil
}

func loadNublarRun(path string) (nublarRunDocument, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nublarRunDocument{}, fmt.Errorf("read Nublar run %s: %w", path, err)
	}
	var document nublarRunDocument
	if err := json.Unmarshal(contents, &document); err != nil {
		return nublarRunDocument{}, fmt.Errorf("parse Nublar run %s: %w", path, err)
	}
	if document.Schema != NublarRunSchema {
		return nublarRunDocument{}, fmt.Errorf("Nublar run schema must be %s, got %q", NublarRunSchema, document.Schema)
	}
	if strings.TrimSpace(document.RunID) == "" || strings.TrimSpace(document.Workflow.ID) == "" {
		return nublarRunDocument{}, fmt.Errorf("Nublar run %s needs a run_id and workflow id", path)
	}
	if strings.TrimSpace(document.Workflow.File.Path) == "" {
		return nublarRunDocument{}, fmt.Errorf("Nublar run %s needs a workflow file path", path)
	}
	if strings.TrimSpace(document.Workflow.File.SHA256) == "" {
		return nublarRunDocument{}, fmt.Errorf("Nublar run %s needs a workflow file sha256", path)
	}
	if document.Correlation != nil {
		if strings.TrimSpace(document.Correlation.System) == "" || strings.TrimSpace(document.Correlation.ID) == "" || document.Correlation.Attempt < 1 {
			return nublarRunDocument{}, fmt.Errorf("Nublar run %s has an invalid correlation", path)
		}
	}
	if len(document.Checks) == 0 {
		return nublarRunDocument{}, fmt.Errorf("Nublar run %s needs at least one check", path)
	}
	seen := make(map[string]bool, len(document.Checks))
	for _, check := range document.Checks {
		if strings.TrimSpace(check.ID) == "" {
			return nublarRunDocument{}, fmt.Errorf("Nublar run %s contains a check without an id", path)
		}
		if seen[check.ID] {
			return nublarRunDocument{}, fmt.Errorf("Nublar run %s duplicates check %q", path, check.ID)
		}
		seen[check.ID] = true
	}
	return document, nil
}

func compareNublarRuns(before, after nublarRunDocument) NublarRunComparison {
	beforeSummary := summarizeNublarRun(before)
	afterSummary := summarizeNublarRun(after)
	comparison := NublarRunComparison{
		Schema: nublarRunComparisonSchema,
		Before: beforeSummary,
		After:  afterSummary,
	}
	if before.Workflow.ID != after.Workflow.ID {
		comparison.CompatibilityReasons = append(comparison.CompatibilityReasons, fmt.Sprintf("workflow id changed from %q to %q", before.Workflow.ID, after.Workflow.ID))
	}
	comparison.Compatible = len(comparison.CompatibilityReasons) == 0
	comparison.Transition = NewStateTransition("status", before.Status, after.Status, comparison.Compatible)
	add := func(category, field string, oldValue, newValue any) {
		if valuesEqual(oldValue, newValue) {
			return
		}
		comparison.Changes = append(comparison.Changes, NewChange(category, field, oldValue, newValue))
	}
	add("context", "workflow.id", before.Workflow.ID, after.Workflow.ID)
	add("context", "workflow.file", before.Workflow.File, after.Workflow.File)
	add("context", "correlation", before.Correlation, after.Correlation)
	add("verdict", "status", before.Status, after.Status)
	add("verdict", "exit_code", before.ExitCode, after.ExitCode)

	checkIDs := make([]string, 0, len(before.Checks)+len(after.Checks))
	seen := make(map[string]bool, len(before.Checks)+len(after.Checks))
	for id := range beforeSummary.Checks {
		seen[id] = true
	}
	for id := range afterSummary.Checks {
		seen[id] = true
	}
	for id := range seen {
		checkIDs = append(checkIDs, id)
	}
	sort.Strings(checkIDs)
	for _, id := range checkIDs {
		oldValue, oldOK := beforeSummary.Checks[id]
		newValue, newOK := afterSummary.Checks[id]
		var oldAny, newAny any
		if oldOK {
			oldAny = oldValue
		}
		if newOK {
			newAny = newValue
		}
		add("check", "checks."+id, oldAny, newAny)
	}
	comparison.ChangeSummary = SummarizeChanges(comparison.Changes)
	return comparison
}

func summarizeNublarRun(document nublarRunDocument) NublarRunSummary {
	checks := make(map[string]NublarCheckSummary, len(document.Checks))
	for _, check := range document.Checks {
		checks[check.ID] = NublarCheckSummary{Tool: check.Tool, Required: check.Required, Status: check.Status, Reason: check.Reason}
	}
	return NublarRunSummary{
		Schema:      document.Schema,
		RunID:       document.RunID,
		Workflow:    NublarWorkflowSummary{ID: document.Workflow.ID, File: document.Workflow.File},
		Correlation: summarizeNublarCorrelation(document.Correlation),
		Status:      document.Status,
		ExitCode:    document.ExitCode,
		CreatedAt:   document.CreatedAt,
		CompletedAt: document.CompletedAt,
		Checks:      checks,
	}
}

func summarizeNublarCorrelation(correlation *struct {
	System  string `json:"system"`
	ID      string `json:"id"`
	Attempt int64  `json:"attempt"`
}) *NublarCorrelationSummary {
	if correlation == nil {
		return nil
	}
	return &NublarCorrelationSummary{System: correlation.System, ID: correlation.ID, Attempt: correlation.Attempt}
}
