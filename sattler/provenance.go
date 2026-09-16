package sattler

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// AmberProvenanceVersion is the wire version understood by this adapter.
const AmberProvenanceVersion = 1

// AmberProvenanceSummary contains the stable identity and transition fields
// Sattler can compare without importing Amber's SDK or storage implementation.
type AmberProvenanceSummary struct {
	Path              string `json:"path,omitempty"`
	Version           int    `json:"version"`
	WorkID            string `json:"work_id"`
	ExecutionID       string `json:"execution_id"`
	CorrelationID     string `json:"correlation_id"`
	Origin            string `json:"origin"`
	Depth             uint64 `json:"depth"`
	Attempt           uint64 `json:"attempt"`
	ModeKind          string `json:"mode_kind"`
	ModeOfExecutionID string `json:"mode_of_execution_id,omitempty"`
}

// AmberProvenanceComparison compares two Amber values at the identity and
// transition boundary. Amber remains authoritative for provenance semantics.
type AmberProvenanceComparison struct {
	Schema               string                 `json:"schema"`
	Compatible           bool                   `json:"compatible"`
	CompatibilityReasons []string               `json:"compatibility_reasons,omitempty"`
	Before               AmberProvenanceSummary `json:"before"`
	After                AmberProvenanceSummary `json:"after"`
	Changes              []Change               `json:"changes,omitempty"`
	ChangeSummary        ChangeSummary          `json:"change_summary"`
}

const amberProvenanceComparisonSchema = "ingen.sattler-amber-provenance-comparison/v0"

type amberProvenanceDocument struct {
	Version       int    `json:"version"`
	WorkID        string `json:"work_id"`
	ExecutionID   string `json:"execution_id"`
	CorrelationID string `json:"correlation_id"`
	Origin        string `json:"origin"`
	Depth         uint64 `json:"depth"`
	Attempt       uint64 `json:"attempt"`
	Mode          struct {
		Kind          string `json:"kind"`
		OfExecutionID string `json:"of_execution_id"`
	} `json:"mode"`
}

// CompareAmberProvenanceFiles loads and compares two Amber provenance values.
func CompareAmberProvenanceFiles(beforePath, afterPath string) (AmberProvenanceComparison, error) {
	before, err := loadAmberProvenance(beforePath)
	if err != nil {
		return AmberProvenanceComparison{}, err
	}
	after, err := loadAmberProvenance(afterPath)
	if err != nil {
		return AmberProvenanceComparison{}, err
	}
	report := compareAmberProvenance(before, after)
	report.Before.Path = beforePath
	report.After.Path = afterPath
	return report, nil
}

// WriteAmberProvenanceJSON writes a provisional machine-readable comparison.
func WriteAmberProvenanceJSON(w io.Writer, report AmberProvenanceComparison) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

// WriteAmberProvenanceText writes a compact provenance comparison.
func WriteAmberProvenanceText(w io.Writer, report AmberProvenanceComparison) error {
	if _, err := fmt.Fprintf(w, "Sattler Amber provenance comparison\n  before: %s (%s)\n  after:  %s (%s)\n  compatible: %t\n", report.Before.Path, report.Before.ExecutionID, report.After.Path, report.After.ExecutionID, report.Compatible); err != nil {
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
		_, err := fmt.Fprintln(w, "  changes: none observable at the Amber boundary")
		return err
	}
	if _, err := fmt.Fprintln(w, "  changes:"); err != nil {
		return err
	}
	for _, change := range report.Changes {
		if _, err := fmt.Fprintf(w, "    - %s %s: %s -> %s\n", change.Category, change.Field, displayValue(change.Before), displayValue(change.After)); err != nil {
			return err
		}
	}
	return nil
}

func loadAmberProvenance(path string) (amberProvenanceDocument, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return amberProvenanceDocument{}, fmt.Errorf("read Amber provenance %s: %w", path, err)
	}
	var document amberProvenanceDocument
	if err := json.Unmarshal(contents, &document); err != nil {
		return amberProvenanceDocument{}, fmt.Errorf("parse Amber provenance %s: %w", path, err)
	}
	if document.Version != AmberProvenanceVersion {
		return amberProvenanceDocument{}, fmt.Errorf("Amber provenance version must be %d, got %d", AmberProvenanceVersion, document.Version)
	}
	for name, value := range map[string]string{
		"work_id": document.WorkID, "execution_id": document.ExecutionID, "correlation_id": document.CorrelationID,
		"origin": document.Origin, "mode.kind": document.Mode.Kind,
	} {
		if strings.TrimSpace(value) == "" {
			return amberProvenanceDocument{}, fmt.Errorf("Amber provenance %s is required", name)
		}
	}
	if document.Attempt == 0 {
		return amberProvenanceDocument{}, fmt.Errorf("Amber provenance attempt must be positive")
	}
	return document, nil
}

func compareAmberProvenance(before, after amberProvenanceDocument) AmberProvenanceComparison {
	comparison := AmberProvenanceComparison{
		Schema: amberProvenanceComparisonSchema,
		Before: summarizeAmberProvenance(before),
		After:  summarizeAmberProvenance(after),
	}
	if before.WorkID != after.WorkID {
		comparison.CompatibilityReasons = append(comparison.CompatibilityReasons, fmt.Sprintf("work id changed from %q to %q", before.WorkID, after.WorkID))
	}
	if before.CorrelationID != after.CorrelationID {
		comparison.CompatibilityReasons = append(comparison.CompatibilityReasons, fmt.Sprintf("correlation id changed from %q to %q", before.CorrelationID, after.CorrelationID))
	}
	comparison.Compatible = len(comparison.CompatibilityReasons) == 0
	add := func(category, field string, oldValue, newValue any) {
		if valuesEqual(oldValue, newValue) {
			return
		}
		comparison.Changes = append(comparison.Changes, Change{Category: category, Field: field, Before: oldValue, After: newValue})
	}
	add("identity", "work_id", before.WorkID, after.WorkID)
	add("identity", "execution_id", before.ExecutionID, after.ExecutionID)
	add("identity", "correlation_id", before.CorrelationID, after.CorrelationID)
	add("context", "origin", before.Origin, after.Origin)
	add("context", "depth", before.Depth, after.Depth)
	add("context", "attempt", before.Attempt, after.Attempt)
	add("context", "mode.kind", before.Mode.Kind, after.Mode.Kind)
	add("context", "mode.of_execution_id", before.Mode.OfExecutionID, after.Mode.OfExecutionID)
	comparison.ChangeSummary = SummarizeChanges(comparison.Changes)
	return comparison
}

func summarizeAmberProvenance(document amberProvenanceDocument) AmberProvenanceSummary {
	return AmberProvenanceSummary{
		Version:           document.Version,
		WorkID:            document.WorkID,
		ExecutionID:       document.ExecutionID,
		CorrelationID:     document.CorrelationID,
		Origin:            document.Origin,
		Depth:             document.Depth,
		Attempt:           document.Attempt,
		ModeKind:          document.Mode.Kind,
		ModeOfExecutionID: document.Mode.OfExecutionID,
	}
}
