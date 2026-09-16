package sattler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ComparisonManifestSchema identifies the provisional input manifest used to
// align related before/after artifacts.
const ComparisonManifestSchema = "ingen.sattler-comparison-input/v0"

// ComparisonInputs names the optional artifact pair for one side of a bundle.
// Relative paths are resolved from the manifest's directory.
type ComparisonInputs struct {
	CIResult   string `json:"ci_result,omitempty"`
	NublarRun  string `json:"nublar_run,omitempty"`
	Custody    string `json:"custody,omitempty"`
	Provenance string `json:"provenance,omitempty"`
}

// ComparisonManifest aligns optional before/after artifacts for one
// investigation.
type ComparisonManifest struct {
	Schema string           `json:"schema"`
	Before ComparisonInputs `json:"before"`
	After  ComparisonInputs `json:"after"`
}

// BundleSubsystemSummary records one adapter's compatibility and change
// counts without collapsing its detailed report.
type BundleSubsystemSummary struct {
	Compatible    bool          `json:"compatible"`
	ChangeSummary ChangeSummary `json:"change_summary"`
}

// BundleSummary is the navigation layer for a combined comparison. It is a
// count and compatibility summary, not a quality score or causal conclusion.
type BundleSummary struct {
	Compatible           bool                              `json:"compatible"`
	CompatibilityReasons []string                          `json:"compatibility_reasons,omitempty"`
	ChangeSummary        ChangeSummary                     `json:"change_summary"`
	Warnings             []string                          `json:"warnings,omitempty"`
	Subsystems           map[string]BundleSubsystemSummary `json:"subsystems"`
}

// BundleComparison combines the independent adapter reports. A missing
// subsystem is represented by a nil report rather than an invented result.
type BundleComparison struct {
	Schema     string                     `json:"schema"`
	Manifest   string                     `json:"manifest,omitempty"`
	Summary    BundleSummary              `json:"summary"`
	CIResult   *Comparison                `json:"ci_result,omitempty"`
	NublarRun  *NublarRunComparison       `json:"nublar_run,omitempty"`
	Custody    *LockwoodCustodyComparison `json:"custody,omitempty"`
	Provenance *AmberProvenanceComparison `json:"provenance,omitempty"`
}

const bundleComparisonSchema = "ingen.sattler-bundle-comparison/v0"

// CompareBundleFile loads a comparison manifest and its declared artifact
// pairs. Every declared pair must have both before and after paths.
func CompareBundleFile(manifestPath string) (BundleComparison, error) {
	manifest, err := loadComparisonManifest(manifestPath)
	if err != nil {
		return BundleComparison{}, err
	}
	root := filepath.Dir(manifestPath)
	report := BundleComparison{Schema: bundleComparisonSchema, Manifest: manifestPath}

	if hasAny(manifest.Before.CIResult, manifest.After.CIResult) {
		before, after, err := pairedPaths(root, "ci_result", manifest.Before.CIResult, manifest.After.CIResult)
		if err != nil {
			return BundleComparison{}, err
		}
		comparison, err := CompareFiles(before, after)
		if err != nil {
			return BundleComparison{}, err
		}
		report.CIResult = &comparison
	}
	if hasAny(manifest.Before.NublarRun, manifest.After.NublarRun) {
		before, after, err := pairedPaths(root, "nublar_run", manifest.Before.NublarRun, manifest.After.NublarRun)
		if err != nil {
			return BundleComparison{}, err
		}
		comparison, err := CompareNublarRunFiles(before, after)
		if err != nil {
			return BundleComparison{}, err
		}
		report.NublarRun = &comparison
	}
	if hasAny(manifest.Before.Custody, manifest.After.Custody) {
		before, after, err := pairedPaths(root, "custody", manifest.Before.Custody, manifest.After.Custody)
		if err != nil {
			return BundleComparison{}, err
		}
		comparison, err := CompareLockwoodCustodyFiles(before, after)
		if err != nil {
			return BundleComparison{}, err
		}
		report.Custody = &comparison
	}
	if hasAny(manifest.Before.Provenance, manifest.After.Provenance) {
		before, after, err := pairedPaths(root, "provenance", manifest.Before.Provenance, manifest.After.Provenance)
		if err != nil {
			return BundleComparison{}, err
		}
		comparison, err := CompareAmberProvenanceFiles(before, after)
		if err != nil {
			return BundleComparison{}, err
		}
		report.Provenance = &comparison
	}
	report.Summary = SummarizeBundle(report)
	return report, nil
}

// SummarizeBundle derives deterministic top-level navigation data from the
// independent reports in a bundle.
func SummarizeBundle(report BundleComparison) BundleSummary {
	summary := BundleSummary{
		Compatible: true,
		Subsystems: make(map[string]BundleSubsystemSummary),
	}
	var allChanges []Change
	included := false
	add := func(name string, compatible bool, reasons []string, changes []Change, warnings []string) {
		included = true
		summary.Subsystems[name] = BundleSubsystemSummary{
			Compatible:    compatible,
			ChangeSummary: SummarizeChanges(changes),
		}
		if !compatible {
			summary.Compatible = false
		}
		for _, reason := range reasons {
			summary.CompatibilityReasons = append(summary.CompatibilityReasons, name+": "+reason)
		}
		for _, warning := range warnings {
			summary.Warnings = append(summary.Warnings, name+": "+warning)
		}
		allChanges = append(allChanges, changes...)
	}
	if report.CIResult != nil {
		add("ci_result", report.CIResult.Compatible, report.CIResult.CompatibilityReasons, report.CIResult.Changes, report.CIResult.Warnings)
	}
	if report.NublarRun != nil {
		add("nublar_run", report.NublarRun.Compatible, report.NublarRun.CompatibilityReasons, report.NublarRun.Changes, nil)
	}
	if report.Custody != nil {
		add("custody", report.Custody.Compatible, report.Custody.CompatibilityReasons, report.Custody.Changes, nil)
	}
	if report.Provenance != nil {
		add("provenance", report.Provenance.Compatible, report.Provenance.CompatibilityReasons, report.Provenance.Changes, nil)
	}
	if !included {
		summary.Compatible = false
	}
	summary.ChangeSummary = SummarizeChanges(allChanges)
	return summary
}

// WriteBundleJSON writes a provisional machine-readable bundle comparison.
func WriteBundleJSON(w io.Writer, report BundleComparison) error {
	report.Summary = SummarizeBundle(report)
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

// WriteBundleText writes the independent subsystem reports under one heading.
func WriteBundleText(w io.Writer, report BundleComparison) error {
	report.Summary = SummarizeBundle(report)
	if _, err := fmt.Fprintf(w, "Sattler bundle comparison\n  manifest: %s\n  compatible: %t\n  change summary: %s\n", report.Manifest, report.Summary.Compatible, report.Summary.ChangeSummary); err != nil {
		return err
	}
	if len(report.Summary.CompatibilityReasons) > 0 {
		if _, err := fmt.Fprintln(w, "  compatibility reasons:"); err != nil {
			return err
		}
		for _, reason := range report.Summary.CompatibilityReasons {
			if _, err := fmt.Fprintf(w, "    - %s\n", reason); err != nil {
				return err
			}
		}
	}
	if len(report.Summary.Warnings) > 0 {
		if _, err := fmt.Fprintln(w, "  warnings:"); err != nil {
			return err
		}
		for _, warning := range report.Summary.Warnings {
			if _, err := fmt.Fprintf(w, "    - %s\n", warning); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintln(w, "  subsystems:"); err != nil {
		return err
	}
	for _, name := range []string{"ci_result", "nublar_run", "custody", "provenance"} {
		detail, ok := report.Summary.Subsystems[name]
		if !ok {
			continue
		}
		if _, err := fmt.Fprintf(w, "    - %s: compatible=%t, changes=%s\n", name, detail.Compatible, detail.ChangeSummary); err != nil {
			return err
		}
	}
	sections := []struct {
		name  string
		write func(io.Writer) error
	}{
		{"ci result", func(w io.Writer) error { return WriteText(w, *report.CIResult) }},
		{"nublar run", func(w io.Writer) error { return WriteNublarText(w, *report.NublarRun) }},
		{"custody", func(w io.Writer) error { return WriteLockwoodText(w, *report.Custody) }},
		{"provenance", func(w io.Writer) error { return WriteAmberProvenanceText(w, *report.Provenance) }},
	}
	for _, section := range sections {
		if !sectionPresent(report, section.name) {
			continue
		}
		var body bytes.Buffer
		if err := section.write(&body); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "\n  %s:\n", section.name); err != nil {
			return err
		}
		for _, line := range strings.Split(strings.TrimSuffix(body.String(), "\n"), "\n") {
			if _, err := fmt.Fprintf(w, "    %s\n", line); err != nil {
				return err
			}
		}
	}
	return nil
}

func loadComparisonManifest(path string) (ComparisonManifest, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return ComparisonManifest{}, &ManifestValidationError{Issues: []ErrorIssue{{
			Code:    "read-manifest",
			Path:    "manifest",
			Message: fmt.Sprintf("read comparison manifest %s: %v", path, err),
		}}}
	}
	var manifest ComparisonManifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		return ComparisonManifest{}, &ManifestValidationError{Issues: []ErrorIssue{{
			Code:    "parse-manifest",
			Path:    "manifest",
			Message: fmt.Sprintf("parse comparison manifest %s: %v", path, err),
		}}}
	}
	if err := manifest.Validate(); err != nil {
		return ComparisonManifest{}, err
	}
	return manifest, nil
}

// Validate checks the manifest boundary before any declared artifact is read.
// A manifest may contain any subset of supported artifact pairs, but each
// declared subsystem must have both sides.
func (m ComparisonManifest) Validate() error {
	issues := make([]ErrorIssue, 0, 2)
	if m.Schema != ComparisonManifestSchema {
		issues = append(issues, ErrorIssue{
			Code:    "invalid-schema",
			Path:    "schema",
			Message: fmt.Sprintf("comparison manifest schema must be %s, got %q", ComparisonManifestSchema, m.Schema),
		})
	}

	pairs := []struct {
		name   string
		before string
		after  string
	}{
		{name: "ci_result", before: m.Before.CIResult, after: m.After.CIResult},
		{name: "nublar_run", before: m.Before.NublarRun, after: m.After.NublarRun},
		{name: "custody", before: m.Before.Custody, after: m.After.Custody},
		{name: "provenance", before: m.Before.Provenance, after: m.After.Provenance},
	}
	declared := false
	for _, pair := range pairs {
		beforeSet := strings.TrimSpace(pair.before) != ""
		afterSet := strings.TrimSpace(pair.after) != ""
		if !beforeSet && !afterSet {
			continue
		}
		declared = true
		if beforeSet && afterSet {
			continue
		}
		issues = append(issues, ErrorIssue{
			Code:    "incomplete-pair",
			Path:    pair.name,
			Message: fmt.Sprintf("comparison manifest %s needs both before and after paths", pair.name),
		})
	}
	if !declared {
		issues = append(issues, ErrorIssue{
			Code:    "no-artifact-pairs",
			Path:    "before/after",
			Message: "comparison manifest declares no artifact pairs",
		})
	}
	if len(issues) > 0 {
		return &ManifestValidationError{Issues: issues}
	}
	return nil
}

func pairedPaths(root, name, before, after string) (string, string, error) {
	if strings.TrimSpace(before) == "" || strings.TrimSpace(after) == "" {
		return "", "", &ManifestValidationError{Issues: []ErrorIssue{{
			Code:    "incomplete-pair",
			Path:    name,
			Message: fmt.Sprintf("comparison manifest %s needs both before and after paths", name),
		}}}
	}
	return resolveManifestPath(root, before), resolveManifestPath(root, after), nil
}

func resolveManifestPath(root, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(root, path))
}

func hasAny(before, after string) bool {
	return strings.TrimSpace(before) != "" || strings.TrimSpace(after) != ""
}

func sectionPresent(report BundleComparison, name string) bool {
	switch name {
	case "ci result":
		return report.CIResult != nil
	case "nublar run":
		return report.NublarRun != nil
	case "custody":
		return report.Custody != nil
	case "provenance":
		return report.Provenance != nil
	default:
		return false
	}
}
