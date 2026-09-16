package sattler

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ComparisonSeriesManifestSchema identifies an ordered list of existing
// bundle manifests to summarize as a history.
const ComparisonSeriesManifestSchema = "ingen.sattler-comparison-series-input/v0"

const comparisonSeriesSchema = "ingen.sattler-comparison-series/v0"
const comparisonSeriesSummarySchema = "ingen.sattler-comparison-series-summary/v0"

// ComparisonSeriesEntry names one bundle comparison in a series.
type ComparisonSeriesEntry struct {
	ID       string `json:"id"`
	Label    string `json:"label,omitempty"`
	Manifest string `json:"manifest"`
}

// ComparisonSeriesManifest is an ordered history of bundle manifests.
type ComparisonSeriesManifest struct {
	Schema  string                  `json:"schema"`
	Entries []ComparisonSeriesEntry `json:"entries"`
}

// SeriesPoint is the detail-light representation of one bundle in a series.
type SeriesPoint struct {
	ID                string              `json:"id"`
	Label             string              `json:"label,omitempty"`
	Manifest          string              `json:"manifest"`
	Summary           BundleSummary       `json:"summary"`
	Correlations      []BundleCorrelation `json:"correlations,omitempty"`
	ChangeIDFilter    []string            `json:"change_id_filter,omitempty"`
	MutationChangeIDs []string            `json:"mutation_change_ids,omitempty"`
}

// SeriesCorrelationSummary counts identity observations across the ordered
// bundle points. It describes observed relationships, not causal links.
type SeriesCorrelationSummary struct {
	Observations int            `json:"observations"`
	ByKind       map[string]int `json:"by_kind,omitempty"`
	ByRelation   map[string]int `json:"by_relation,omitempty"`
}

// SeriesTransitionCounts counts neutral adapter transition classifications
// across the ordered bundle points.
type SeriesTransitionCounts struct {
	Unchanged    int `json:"unchanged"`
	Changed      int `json:"changed"`
	Incompatible int `json:"incompatible"`
}

// SeriesSummary aggregates navigation data across the ordered bundle points.
// It is not a score and does not infer a trend direction.
type SeriesSummary struct {
	Entries                int                               `json:"entries"`
	Compatible             int                               `json:"compatible"`
	Incompatible           int                               `json:"incompatible"`
	TotalChanges           int                               `json:"total_changes"`
	ChangesByID            map[string]int                    `json:"changes_by_id,omitempty"`
	MutationChangesByID    map[string]int                    `json:"mutation_changes_by_id,omitempty"`
	CorrelationSummary     SeriesCorrelationSummary          `json:"correlations"`
	TransitionsBySubsystem map[string]SeriesTransitionCounts `json:"transitions_by_subsystem,omitempty"`
}

// ComparisonSeries is a deterministic history projection over bundle
// comparisons.
type ComparisonSeries struct {
	Schema         string        `json:"schema"`
	Manifest       string        `json:"manifest,omitempty"`
	ChangeIDFilter []string      `json:"change_id_filter,omitempty"`
	Summary        SeriesSummary `json:"summary"`
	Entries        []SeriesPoint `json:"entries"`
}

// SeriesSummaryReport is the compact projection of a comparison series.
type SeriesSummaryReport struct {
	Schema         string        `json:"schema"`
	Manifest       string        `json:"manifest,omitempty"`
	ChangeIDFilter []string      `json:"change_id_filter,omitempty"`
	Summary        SeriesSummary `json:"summary"`
}

// NewSeriesSummaryReport projects a full series without its ordered points.
func NewSeriesSummaryReport(series ComparisonSeries) SeriesSummaryReport {
	return SeriesSummaryReport{
		Schema:         comparisonSeriesSummarySchema,
		Manifest:       series.Manifest,
		ChangeIDFilter: append([]string(nil), series.ChangeIDFilter...),
		Summary:        series.Summary,
	}
}

// CompareSeriesManifestFile loads and aggregates the ordered bundle
// manifests named by a series manifest.
func CompareSeriesManifestFile(seriesPath string) (ComparisonSeries, error) {
	return CompareSeriesManifestFileWithChanges(seriesPath, nil)
}

// CompareSeriesManifestFileWithChanges applies an optional stable-ID filter
// to each bundle before aggregating the series.
func CompareSeriesManifestFileWithChanges(seriesPath string, ids []string) (ComparisonSeries, error) {
	manifest, err := loadComparisonSeriesManifest(seriesPath)
	if err != nil {
		return ComparisonSeries{}, err
	}
	series := ComparisonSeries{
		Schema:         comparisonSeriesSchema,
		Manifest:       seriesPath,
		ChangeIDFilter: sortedChangeIDs(ids),
		Entries:        make([]SeriesPoint, 0, len(manifest.Entries)),
	}
	root := filepath.Dir(seriesPath)
	series.Summary.Entries = len(manifest.Entries)
	series.Summary.ChangesByID = make(map[string]int)
	series.Summary.MutationChangesByID = make(map[string]int)
	for _, entry := range manifest.Entries {
		bundlePath := resolveManifestPath(root, entry.Manifest)
		bundle, err := CompareBundleFile(bundlePath)
		if err != nil {
			return ComparisonSeries{}, fmt.Errorf("compare series entry %s: %w", entry.ID, err)
		}
		bundle = FilterBundleChanges(bundle, ids)
		point := SeriesPoint{
			ID:                entry.ID,
			Label:             entry.Label,
			Manifest:          bundlePath,
			Summary:           bundle.Summary,
			Correlations:      append([]BundleCorrelation(nil), bundle.Correlations...),
			ChangeIDFilter:    append([]string(nil), bundle.ChangeIDFilter...),
			MutationChangeIDs: bundleMutationChangeIDs(bundle),
		}
		series.Entries = append(series.Entries, point)
		if point.Summary.Compatible {
			series.Summary.Compatible++
		} else {
			series.Summary.Incompatible++
		}
		series.Summary.TotalChanges += point.Summary.ChangeSummary.Total
		for _, change := range bundleBoundaryChanges(bundle) {
			series.Summary.ChangesByID[change.StableID()]++
		}
		for _, mutationID := range point.MutationChangeIDs {
			series.Summary.MutationChangesByID[mutationID]++
		}
	}
	if len(series.Summary.ChangesByID) == 0 {
		series.Summary.ChangesByID = nil
	}
	if len(series.Summary.MutationChangesByID) == 0 {
		series.Summary.MutationChangesByID = nil
	}
	series.Summary.CorrelationSummary = summarizeSeriesCorrelations(series.Entries)
	series.Summary.TransitionsBySubsystem = summarizeSeriesTransitions(series.Entries)
	return series, nil
}

// WriteSeriesJSON writes the machine-readable history projection.
func WriteSeriesJSON(w io.Writer, series ComparisonSeries) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(series)
}

// WriteSeriesSummaryJSON writes the machine-readable, point-free history
// projection.
func WriteSeriesSummaryJSON(w io.Writer, series ComparisonSeries) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(NewSeriesSummaryReport(series))
}

// WriteSeriesText writes a compact operator-oriented history projection.
func WriteSeriesText(w io.Writer, series ComparisonSeries) error {
	if _, err := fmt.Fprintf(w, "Sattler comparison series\n  manifest: %s\n  entries: %d (compatible=%d, incompatible=%d)\n  total changes: %d\n", series.Manifest, series.Summary.Entries, series.Summary.Compatible, series.Summary.Incompatible, series.Summary.TotalChanges); err != nil {
		return err
	}
	if len(series.ChangeIDFilter) > 0 {
		if _, err := fmt.Fprintf(w, "  change ID filter: %s\n", strings.Join(series.ChangeIDFilter, ", ")); err != nil {
			return err
		}
	}
	if err := writeSeriesSummaryDetails(w, series.Summary); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "  points:"); err != nil {
		return err
	}
	for _, point := range series.Entries {
		label := point.ID
		if point.Label != "" {
			label += " (" + point.Label + ")"
		}
		mutationChanges := ""
		if len(point.MutationChangeIDs) > 0 {
			mutationChanges = ", mutation changes=" + strings.Join(point.MutationChangeIDs, ",")
		}
		if _, err := fmt.Fprintf(w, "    - %s: compatible=%t, changes=%s%s\n", label, point.Summary.Compatible, point.Summary.ChangeSummary, mutationChanges); err != nil {
			return err
		}
		if len(point.Correlations) > 0 {
			if _, err := fmt.Fprintln(w, "      correlations:"); err != nil {
				return err
			}
			for _, correlation := range point.Correlations {
				if err := writeBundleCorrelationTextIndented(w, "        ", correlation); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// WriteSeriesSummaryText writes the compact operator-oriented history
// projection without individual points.
func WriteSeriesSummaryText(w io.Writer, series ComparisonSeries) error {
	if _, err := fmt.Fprintf(w, "Sattler comparison series summary\n  manifest: %s\n  entries: %d (compatible=%d, incompatible=%d)\n  total changes: %d\n", series.Manifest, series.Summary.Entries, series.Summary.Compatible, series.Summary.Incompatible, series.Summary.TotalChanges); err != nil {
		return err
	}
	if len(series.ChangeIDFilter) > 0 {
		if _, err := fmt.Fprintf(w, "  change ID filter: %s\n", strings.Join(series.ChangeIDFilter, ", ")); err != nil {
			return err
		}
	}
	return writeSeriesSummaryDetails(w, series.Summary)
}

func writeSeriesSummaryDetails(w io.Writer, summary SeriesSummary) error {
	ids := sortedCounts(summary.ChangesByID)
	if len(ids) > 0 {
		if _, err := fmt.Fprintln(w, "  changes by ID:"); err != nil {
			return err
		}
		for _, id := range ids {
			if _, err := fmt.Fprintf(w, "    - %s: %d\n", id, summary.ChangesByID[id]); err != nil {
				return err
			}
		}
	}
	mutationIDs := sortedCounts(summary.MutationChangesByID)
	if len(mutationIDs) > 0 {
		if _, err := fmt.Fprintln(w, "  mutation changes by ID:"); err != nil {
			return err
		}
		for _, mutationID := range mutationIDs {
			if _, err := fmt.Fprintf(w, "    - %s: %d\n", mutationID, summary.MutationChangesByID[mutationID]); err != nil {
				return err
			}
		}
	}
	if summary.CorrelationSummary.Observations > 0 {
		if _, err := fmt.Fprintf(w, "  correlation observations: %d\n", summary.CorrelationSummary.Observations); err != nil {
			return err
		}
		kindIDs := sortedCounts(summary.CorrelationSummary.ByKind)
		if _, err := fmt.Fprintln(w, "  correlations by kind:"); err != nil {
			return err
		}
		for _, kind := range kindIDs {
			if _, err := fmt.Fprintf(w, "    - %s: %d\n", kind, summary.CorrelationSummary.ByKind[kind]); err != nil {
				return err
			}
		}
		relationIDs := sortedCounts(summary.CorrelationSummary.ByRelation)
		if _, err := fmt.Fprintln(w, "  correlations by relation:"); err != nil {
			return err
		}
		for _, relation := range relationIDs {
			if _, err := fmt.Fprintf(w, "    - %s: %d\n", relation, summary.CorrelationSummary.ByRelation[relation]); err != nil {
				return err
			}
		}
	}
	if len(summary.TransitionsBySubsystem) > 0 {
		if _, err := fmt.Fprintln(w, "  transitions by subsystem:"); err != nil {
			return err
		}
		for _, subsystem := range sortedTransitionSubsystems(summary.TransitionsBySubsystem) {
			counts := summary.TransitionsBySubsystem[subsystem]
			if _, err := fmt.Fprintf(w, "    - %s: unchanged=%d, changed=%d, incompatible=%d\n", subsystem, counts.Unchanged, counts.Changed, counts.Incompatible); err != nil {
				return err
			}
		}
	}
	return nil
}

func sortedCounts(counts map[string]int) []string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedTransitionSubsystems(transitions map[string]SeriesTransitionCounts) []string {
	keys := make([]string, 0, len(transitions))
	for key := range transitions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func summarizeSeriesCorrelations(points []SeriesPoint) SeriesCorrelationSummary {
	summary := SeriesCorrelationSummary{
		ByKind:     make(map[string]int),
		ByRelation: make(map[string]int),
	}
	for _, point := range points {
		for _, correlation := range point.Correlations {
			summary.Observations++
			summary.ByKind[correlation.Kind]++
			summary.ByRelation[string(correlation.Relation)]++
		}
	}
	if len(summary.ByKind) == 0 {
		summary.ByKind = nil
	}
	if len(summary.ByRelation) == 0 {
		summary.ByRelation = nil
	}
	return summary
}

func summarizeSeriesTransitions(points []SeriesPoint) map[string]SeriesTransitionCounts {
	transitions := make(map[string]SeriesTransitionCounts)
	for _, point := range points {
		for subsystem, detail := range point.Summary.Subsystems {
			counts := transitions[subsystem]
			switch detail.Transition.Classification {
			case TransitionUnchanged:
				counts.Unchanged++
			case TransitionChanged:
				counts.Changed++
			case TransitionIncompatible:
				counts.Incompatible++
			default:
				continue
			}
			transitions[subsystem] = counts
		}
	}
	if len(transitions) == 0 {
		return nil
	}
	return transitions
}

func loadComparisonSeriesManifest(path string) (ComparisonSeriesManifest, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return ComparisonSeriesManifest{}, &ManifestValidationError{Issues: []ErrorIssue{{
			Code:    "read-series-manifest",
			Path:    "manifest",
			Message: fmt.Sprintf("read comparison series manifest %s: %v", path, err),
		}}}
	}
	var manifest ComparisonSeriesManifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		return ComparisonSeriesManifest{}, &ManifestValidationError{Issues: []ErrorIssue{{
			Code:    "parse-series-manifest",
			Path:    "manifest",
			Message: fmt.Sprintf("parse comparison series manifest %s: %v", path, err),
		}}}
	}
	if err := manifest.Validate(); err != nil {
		return ComparisonSeriesManifest{}, err
	}
	return manifest, nil
}

// Validate checks the series manifest before any bundle is opened.
func (m ComparisonSeriesManifest) Validate() error {
	issues := make([]ErrorIssue, 0, 1)
	if m.Schema != ComparisonSeriesManifestSchema {
		issues = append(issues, ErrorIssue{
			Code:    "invalid-schema",
			Path:    "schema",
			Message: fmt.Sprintf("comparison series manifest schema must be %s, got %q", ComparisonSeriesManifestSchema, m.Schema),
		})
	}
	if len(m.Entries) == 0 {
		issues = append(issues, ErrorIssue{
			Code:    "no-series-entries",
			Path:    "entries",
			Message: "comparison series manifest declares no entries",
		})
	}
	seen := make(map[string]bool, len(m.Entries))
	for index, entry := range m.Entries {
		path := fmt.Sprintf("entries[%d]", index)
		if strings.TrimSpace(entry.ID) == "" {
			issues = append(issues, ErrorIssue{Code: "missing-entry-id", Path: path + ".id", Message: fmt.Sprintf("comparison series entry %d needs an id", index)})
		} else if seen[entry.ID] {
			issues = append(issues, ErrorIssue{Code: "duplicate-entry-id", Path: path + ".id", Message: fmt.Sprintf("comparison series manifest duplicates entry %q", entry.ID)})
		}
		seen[entry.ID] = true
		if strings.TrimSpace(entry.Manifest) == "" {
			issues = append(issues, ErrorIssue{Code: "missing-entry-manifest", Path: path + ".manifest", Message: fmt.Sprintf("comparison series entry %q needs a manifest path", entry.ID)})
		}
	}
	if len(issues) > 0 {
		return &ManifestValidationError{Issues: issues}
	}
	return nil
}

func bundleBoundaryChanges(report BundleComparison) []Change {
	changes := make([]Change, 0)
	if report.CIResult != nil {
		changes = append(changes, report.CIResult.Changes...)
	}
	if report.NublarRun != nil {
		changes = append(changes, report.NublarRun.Changes...)
	}
	if report.Custody != nil {
		changes = append(changes, report.Custody.Changes...)
	}
	if report.Provenance != nil {
		changes = append(changes, report.Provenance.Changes...)
	}
	return changes
}

func bundleMutationChangeIDs(report BundleComparison) []string {
	if report.CIResult == nil || report.CIResult.MutationCampaign == nil {
		return nil
	}
	ids := make([]string, 0, len(report.CIResult.MutationCampaign.ChangedMutations))
	for _, change := range report.CIResult.MutationCampaign.ChangedMutations {
		if strings.TrimSpace(change.MutationID) != "" {
			ids = append(ids, change.MutationID)
		}
	}
	sort.Strings(ids)
	return ids
}
