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
	ID             string              `json:"id"`
	Label          string              `json:"label,omitempty"`
	Manifest       string              `json:"manifest"`
	Summary        BundleSummary       `json:"summary"`
	Correlations   []BundleCorrelation `json:"correlations,omitempty"`
	ChangeIDFilter []string            `json:"change_id_filter,omitempty"`
}

// SeriesSummary aggregates navigation data across the ordered bundle points.
// It is not a score and does not infer a trend direction.
type SeriesSummary struct {
	Entries      int            `json:"entries"`
	Compatible   int            `json:"compatible"`
	Incompatible int            `json:"incompatible"`
	TotalChanges int            `json:"total_changes"`
	ChangesByID  map[string]int `json:"changes_by_id,omitempty"`
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
	for _, entry := range manifest.Entries {
		bundlePath := resolveManifestPath(root, entry.Manifest)
		bundle, err := CompareBundleFile(bundlePath)
		if err != nil {
			return ComparisonSeries{}, fmt.Errorf("compare series entry %s: %w", entry.ID, err)
		}
		bundle = FilterBundleChanges(bundle, ids)
		point := SeriesPoint{
			ID:             entry.ID,
			Label:          entry.Label,
			Manifest:       bundlePath,
			Summary:        bundle.Summary,
			Correlations:   append([]BundleCorrelation(nil), bundle.Correlations...),
			ChangeIDFilter: append([]string(nil), bundle.ChangeIDFilter...),
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
	}
	if len(series.Summary.ChangesByID) == 0 {
		series.Summary.ChangesByID = nil
	}
	return series, nil
}

// WriteSeriesJSON writes the machine-readable history projection.
func WriteSeriesJSON(w io.Writer, series ComparisonSeries) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(series)
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
	ids := make([]string, 0, len(series.Summary.ChangesByID))
	for id := range series.Summary.ChangesByID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) > 0 {
		if _, err := fmt.Fprintln(w, "  changes by ID:"); err != nil {
			return err
		}
		for _, id := range ids {
			if _, err := fmt.Fprintf(w, "    - %s: %d\n", id, series.Summary.ChangesByID[id]); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintln(w, "  points:"); err != nil {
		return err
	}
	for _, point := range series.Entries {
		label := point.ID
		if point.Label != "" {
			label += " (" + point.Label + ")"
		}
		if _, err := fmt.Fprintf(w, "    - %s: compatible=%t, changes=%s\n", label, point.Summary.Compatible, point.Summary.ChangeSummary); err != nil {
			return err
		}
	}
	return nil
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
