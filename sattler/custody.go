package sattler

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// LockwoodCustodySchema is the custody record schema understood by this
// adapter.
const LockwoodCustodySchema = "lockwood.custody/v1"

// LockwoodCustodySummary contains the custody metadata Sattler can compare
// without importing Lockwood's storage or verification implementation.
type LockwoodCustodySummary struct {
	Path            string `json:"path,omitempty"`
	Schema          string `json:"schema"`
	CustodyID       string `json:"custody_id"`
	Status          string `json:"status"`
	ReceivedAt      string `json:"received_at"`
	ArtifactDigest  string `json:"artifact_digest"`
	ProducerTool    string `json:"producer_tool"`
	ProducerKind    string `json:"producer_kind"`
	SourceRunID     string `json:"source_run_id,omitempty"`
	IntegrityStatus string `json:"integrity_status"`
}

// LockwoodCustodyComparison is a deterministic comparison of two custody
// records. Artifact identity changes remain comparable context.
type LockwoodCustodyComparison struct {
	Schema               string                 `json:"schema"`
	Compatible           bool                   `json:"compatible"`
	CompatibilityReasons []string               `json:"compatibility_reasons,omitempty"`
	Before               LockwoodCustodySummary `json:"before"`
	After                LockwoodCustodySummary `json:"after"`
	Changes              []Change               `json:"changes,omitempty"`
	ChangeSummary        ChangeSummary          `json:"change_summary"`
}

const lockwoodCustodyComparisonSchema = "ingen.sattler-lockwood-custody-comparison/v0"

type lockwoodCustodyDocument struct {
	Schema     string `json:"schema"`
	CustodyID  string `json:"custody_id"`
	Status     string `json:"status"`
	ReceivedAt string `json:"received_at"`
	Artifact   struct {
		Schema string `json:"schema"`
		Digest string `json:"digest"`
	} `json:"artifact"`
	Producer struct {
		Tool string `json:"tool"`
		Kind string `json:"kind"`
	} `json:"producer"`
	Source struct {
		RunID string `json:"run_id"`
	} `json:"source"`
	Integrity struct {
		Status string `json:"status"`
	} `json:"integrity"`
}

// CompareLockwoodCustodyFiles loads and compares two Lockwood custody
// records.
func CompareLockwoodCustodyFiles(beforePath, afterPath string) (LockwoodCustodyComparison, error) {
	before, err := loadLockwoodCustody(beforePath)
	if err != nil {
		return LockwoodCustodyComparison{}, err
	}
	after, err := loadLockwoodCustody(afterPath)
	if err != nil {
		return LockwoodCustodyComparison{}, err
	}
	report := compareLockwoodCustody(before, after)
	report.Before.Path = beforePath
	report.After.Path = afterPath
	return report, nil
}

// WriteLockwoodJSON writes a provisional machine-readable custody comparison.
func WriteLockwoodJSON(w io.Writer, report LockwoodCustodyComparison) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

// WriteLockwoodText writes a compact custody comparison.
func WriteLockwoodText(w io.Writer, report LockwoodCustodyComparison) error {
	if _, err := fmt.Fprintf(w, "Sattler Lockwood custody comparison\n  before: %s (%s)\n  after:  %s (%s)\n  compatible: %t\n", report.Before.Path, report.Before.Status, report.After.Path, report.After.Status, report.Compatible); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  received: %s -> %s\n", report.Before.ReceivedAt, report.After.ReceivedAt); err != nil {
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
		_, err := fmt.Fprintln(w, "  changes: none observable at the Lockwood boundary")
		return err
	}
	if _, err := fmt.Fprintln(w, "  changes:"); err != nil {
		return err
	}
	for _, change := range report.Changes {
		identity := ""
		if change.Identity != "" {
			identity = " [" + string(change.Identity) + "]"
		}
		if _, err := fmt.Fprintf(w, "    - %s %s%s: %s -> %s\n", change.Category, change.Field, identity, displayValue(change.Before), displayValue(change.After)); err != nil {
			return err
		}
	}
	return nil
}

func loadLockwoodCustody(path string) (lockwoodCustodyDocument, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return lockwoodCustodyDocument{}, fmt.Errorf("read Lockwood custody record %s: %w", path, err)
	}
	var document lockwoodCustodyDocument
	if err := json.Unmarshal(contents, &document); err != nil {
		return lockwoodCustodyDocument{}, fmt.Errorf("parse Lockwood custody record %s: %w", path, err)
	}
	if document.Schema != LockwoodCustodySchema {
		return lockwoodCustodyDocument{}, fmt.Errorf("Lockwood custody schema must be %s, got %q", LockwoodCustodySchema, document.Schema)
	}
	if strings.TrimSpace(document.CustodyID) == "" || strings.TrimSpace(document.Status) == "" {
		return lockwoodCustodyDocument{}, fmt.Errorf("Lockwood custody record %s needs custody_id and status", path)
	}
	if document.Artifact.Schema != "lockwood.artifact/v1" || strings.TrimSpace(document.Artifact.Digest) == "" {
		return lockwoodCustodyDocument{}, fmt.Errorf("Lockwood custody record %s needs a lockwood.artifact/v1 digest", path)
	}
	if strings.TrimSpace(document.Producer.Tool) == "" || strings.TrimSpace(document.Producer.Kind) == "" {
		return lockwoodCustodyDocument{}, fmt.Errorf("Lockwood custody record %s needs producer tool and kind", path)
	}
	if strings.TrimSpace(document.Integrity.Status) == "" {
		return lockwoodCustodyDocument{}, fmt.Errorf("Lockwood custody record %s needs integrity status", path)
	}
	return document, nil
}

func compareLockwoodCustody(before, after lockwoodCustodyDocument) LockwoodCustodyComparison {
	comparison := LockwoodCustodyComparison{
		Schema: lockwoodCustodyComparisonSchema,
		Before: summarizeLockwoodCustody(before),
		After:  summarizeLockwoodCustody(after),
	}
	if before.Producer.Tool != after.Producer.Tool {
		comparison.CompatibilityReasons = append(comparison.CompatibilityReasons, fmt.Sprintf("producer tool changed from %q to %q", before.Producer.Tool, after.Producer.Tool))
	}
	if before.Producer.Kind != after.Producer.Kind {
		comparison.CompatibilityReasons = append(comparison.CompatibilityReasons, fmt.Sprintf("producer kind changed from %q to %q", before.Producer.Kind, after.Producer.Kind))
	}
	comparison.Compatible = len(comparison.CompatibilityReasons) == 0
	add := func(category, field string, oldValue, newValue any, identity ArtifactIdentityRelation) {
		if valuesEqual(oldValue, newValue) {
			return
		}
		comparison.Changes = append(comparison.Changes, Change{Category: category, Field: field, Before: oldValue, After: newValue, Identity: identity})
	}
	add("custody", "status", before.Status, after.Status, "")
	add("artifact", "artifact.digest", before.Artifact.Digest, after.Artifact.Digest, CompareArtifactDigests(before.Artifact.Digest, after.Artifact.Digest))
	add("context", "source.run_id", before.Source.RunID, after.Source.RunID, "")
	add("custody", "integrity.status", before.Integrity.Status, after.Integrity.Status, "")
	comparison.ChangeSummary = SummarizeChanges(comparison.Changes)
	return comparison
}

func summarizeLockwoodCustody(document lockwoodCustodyDocument) LockwoodCustodySummary {
	return LockwoodCustodySummary{
		Schema:          document.Schema,
		CustodyID:       document.CustodyID,
		Status:          document.Status,
		ReceivedAt:      document.ReceivedAt,
		ArtifactDigest:  document.Artifact.Digest,
		ProducerTool:    document.Producer.Tool,
		ProducerKind:    document.Producer.Kind,
		SourceRunID:     document.Source.RunID,
		IntegrityStatus: document.Integrity.Status,
	}
}
