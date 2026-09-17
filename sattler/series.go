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
const comparisonSeriesLatestSchema = "ingen.sattler-comparison-series-latest/v0"

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
	ID                 string              `json:"id"`
	Label              string              `json:"label,omitempty"`
	Latest             bool                `json:"latest,omitempty"`
	Manifest           string              `json:"manifest"`
	Summary            BundleSummary       `json:"summary"`
	Correlations       []BundleCorrelation `json:"correlations,omitempty"`
	ChangeIDFilter     []string            `json:"change_id_filter,omitempty"`
	MutationChangeIDs  []string            `json:"mutation_change_ids,omitempty"`
	SornaRuleChangeIDs []string            `json:"sorna_rule_change_ids,omitempty"`
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
	SornaRuleChangesByID   map[string]int                    `json:"sorna_rule_changes_by_id,omitempty"`
	CorrelationSummary     SeriesCorrelationSummary          `json:"correlations"`
	TransitionsBySubsystem map[string]SeriesTransitionCounts `json:"transitions_by_subsystem,omitempty"`
	WarningsByMessage      map[string]int                    `json:"warnings_by_message,omitempty"`
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

// Validate checks the compatibility-treated ordered history projection. It
// reconciles the retained points with aggregate counts but does not infer a
// trend, quality score, or causal relationship.
func (series ComparisonSeries) Validate() error {
	if series.Schema != comparisonSeriesSchema {
		return fmt.Errorf("comparison series schema must be %s, got %q", comparisonSeriesSchema, series.Schema)
	}
	if series.Manifest != "" && strings.TrimSpace(series.Manifest) == "" {
		return fmt.Errorf("comparison series manifest cannot be empty when present")
	}
	if err := validateSeriesStringList(series.ChangeIDFilter, "comparison series change ID filter"); err != nil {
		return err
	}
	if len(series.Entries) == 0 {
		return fmt.Errorf("comparison series needs at least one point")
	}
	if err := series.Summary.Validate(); err != nil {
		return fmt.Errorf("comparison series summary: %w", err)
	}
	if series.Summary.Entries != len(series.Entries) {
		return fmt.Errorf("comparison series summary entries must equal point count")
	}

	pointIDs := make(map[string]struct{}, len(series.Entries))
	expectedCompatible := 0
	expectedIncompatible := 0
	expectedTotalChanges := 0
	for index, point := range series.Entries {
		requireLatest := index == len(series.Entries)-1
		if point.Latest != requireLatest {
			return fmt.Errorf("comparison series point %q has invalid latest marker", point.ID)
		}
		if err := validateSeriesPoint(point, requireLatest); err != nil {
			return fmt.Errorf("comparison series point %d: %w", index, err)
		}
		if _, exists := pointIDs[point.ID]; exists {
			return fmt.Errorf("comparison series point ID %q is duplicated", point.ID)
		}
		pointIDs[point.ID] = struct{}{}
		if point.Summary.Compatible {
			expectedCompatible++
		} else {
			expectedIncompatible++
		}
		expectedTotalChanges += point.Summary.ChangeSummary.Total
	}
	if series.Summary.Compatible != expectedCompatible || series.Summary.Incompatible != expectedIncompatible {
		return fmt.Errorf("comparison series compatibility counts do not match point summaries")
	}
	if series.Summary.TotalChanges != expectedTotalChanges {
		return fmt.Errorf("comparison series total changes do not match point summaries")
	}

	expectedMutations := make(map[string]int)
	expectedRules := make(map[string]int)
	for _, point := range series.Entries {
		for _, mutationID := range point.MutationChangeIDs {
			expectedMutations[mutationID]++
		}
		for _, ruleID := range point.SornaRuleChangeIDs {
			expectedRules[ruleID]++
		}
	}
	if !seriesCountMapsEqual(series.Summary.MutationChangesByID, expectedMutations) {
		return fmt.Errorf("comparison series mutation change counts do not match point identifiers")
	}
	if !seriesCountMapsEqual(series.Summary.SornaRuleChangesByID, expectedRules) {
		return fmt.Errorf("comparison series Sorna rule change counts do not match point identifiers")
	}
	if !seriesCorrelationSummariesEqual(series.Summary.CorrelationSummary, summarizeSeriesCorrelations(series.Entries)) {
		return fmt.Errorf("comparison series correlation counts do not match point correlations")
	}
	if !seriesTransitionMapsEqual(series.Summary.TransitionsBySubsystem, summarizeSeriesTransitions(series.Entries)) {
		return fmt.Errorf("comparison series transition counts do not match point summaries")
	}
	if !seriesCountMapsEqual(series.Summary.WarningsByMessage, summarizeSeriesWarnings(series.Entries)) {
		return fmt.Errorf("comparison series warning counts do not match point summaries")
	}
	return nil
}

// SeriesSummaryReport is the compact projection of a comparison series.
type SeriesSummaryReport struct {
	Schema         string        `json:"schema"`
	Manifest       string        `json:"manifest,omitempty"`
	ChangeIDFilter []string      `json:"change_id_filter,omitempty"`
	Summary        SeriesSummary `json:"summary"`
}

// Validate checks the aggregate invariants of the compatibility-treated
// point-free series summary.
func (report SeriesSummaryReport) Validate() error {
	if report.Schema != comparisonSeriesSummarySchema {
		return fmt.Errorf("series summary schema must be %s, got %q", comparisonSeriesSummarySchema, report.Schema)
	}
	for _, id := range report.ChangeIDFilter {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("series summary change ID filter cannot contain an empty ID")
		}
	}
	if err := report.Summary.Validate(); err != nil {
		return fmt.Errorf("series summary: %w", err)
	}
	return nil
}

// Validate checks aggregate counts without inferring a trend or quality
// direction from the history.
func (summary SeriesSummary) Validate() error {
	if summary.Entries < 1 {
		return fmt.Errorf("series summary needs at least one entry")
	}
	if summary.Compatible < 0 || summary.Incompatible < 0 || summary.Compatible+summary.Incompatible != summary.Entries {
		return fmt.Errorf("series summary compatible and incompatible counts must equal entries")
	}
	if summary.TotalChanges < 0 {
		return fmt.Errorf("series summary total changes must not be negative")
	}
	changeTotal, err := validateSeriesCountMap("changes_by_id", summary.ChangesByID)
	if err != nil {
		return err
	}
	if changeTotal != summary.TotalChanges {
		return fmt.Errorf("series summary changes_by_id counts must equal total changes")
	}
	if _, err := validateSeriesCountMap("mutation_changes_by_id", summary.MutationChangesByID); err != nil {
		return err
	}
	if _, err := validateSeriesCountMap("sorna_rule_changes_by_id", summary.SornaRuleChangesByID); err != nil {
		return err
	}
	if summary.CorrelationSummary.Observations < 0 {
		return fmt.Errorf("series summary correlation observations must not be negative")
	}
	kindTotal, err := validateSeriesCountMap("correlations.by_kind", summary.CorrelationSummary.ByKind)
	if err != nil {
		return err
	}
	for kind := range summary.CorrelationSummary.ByKind {
		switch kind {
		case BundleCorrelationKindNublarCustodySource, BundleCorrelationKindNublarAmber:
		default:
			return fmt.Errorf("series summary has unsupported correlation kind %q", kind)
		}
	}
	relationTotal, err := validateSeriesCountMap("correlations.by_relation", summary.CorrelationSummary.ByRelation)
	if err != nil {
		return err
	}
	for relation := range summary.CorrelationSummary.ByRelation {
		switch BundleCorrelationRelation(relation) {
		case BundleCorrelationExactMatch, BundleCorrelationMismatch, BundleCorrelationUnknown:
		default:
			return fmt.Errorf("series summary has unsupported correlation relation %q", relation)
		}
	}
	if kindTotal != summary.CorrelationSummary.Observations || relationTotal != summary.CorrelationSummary.Observations {
		return fmt.Errorf("series summary correlation counts must equal observations")
	}
	for subsystem, counts := range summary.TransitionsBySubsystem {
		switch subsystem {
		case "ci_result", "sorna_run", "nublar_run", "custody", "provenance":
		default:
			return fmt.Errorf("series summary has unsupported transition subsystem %q", subsystem)
		}
		if counts.Unchanged < 0 || counts.Changed < 0 || counts.Incompatible < 0 {
			return fmt.Errorf("series summary transition counts for %s must not be negative", subsystem)
		}
		if counts.Unchanged+counts.Changed+counts.Incompatible > summary.Entries {
			return fmt.Errorf("series summary transition counts for %s exceed entries", subsystem)
		}
	}
	if _, err := validateSeriesCountMap("warnings_by_message", summary.WarningsByMessage); err != nil {
		return err
	}
	return nil
}

func validateSeriesCountMap(name string, counts map[string]int) (int, error) {
	total := 0
	for key, count := range counts {
		if strings.TrimSpace(key) == "" {
			return 0, fmt.Errorf("series summary %s key must not be empty", name)
		}
		if count < 0 {
			return 0, fmt.Errorf("series summary %s count for %q must not be negative", name, key)
		}
		total += count
	}
	return total, nil
}

func validateSeriesStringList(values []string, name string) error {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s cannot contain an empty value", name)
		}
	}
	return nil
}

func seriesCountMapsEqual(got, want map[string]int) bool {
	if len(got) != len(want) {
		return false
	}
	for key, value := range want {
		if got[key] != value {
			return false
		}
	}
	return true
}

func seriesCorrelationSummariesEqual(got, want SeriesCorrelationSummary) bool {
	return got.Observations == want.Observations &&
		seriesCountMapsEqual(got.ByKind, want.ByKind) &&
		seriesCountMapsEqual(got.ByRelation, want.ByRelation)
}

func seriesTransitionMapsEqual(got, want map[string]SeriesTransitionCounts) bool {
	if len(got) != len(want) {
		return false
	}
	for subsystem, expected := range want {
		if got[subsystem] != expected {
			return false
		}
	}
	return true
}

// SeriesLatestReport is the compact projection of the final ordered point.
type SeriesLatestReport struct {
	Schema         string      `json:"schema"`
	Manifest       string      `json:"manifest,omitempty"`
	ChangeIDFilter []string    `json:"change_id_filter,omitempty"`
	Point          SeriesPoint `json:"point"`
}

// Validate checks the compatibility-treated final-point retrieval projection.
func (report SeriesLatestReport) Validate() error {
	if report.Schema != comparisonSeriesLatestSchema {
		return fmt.Errorf("series latest schema must be %s, got %q", comparisonSeriesLatestSchema, report.Schema)
	}
	for _, id := range report.ChangeIDFilter {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("series latest change ID filter cannot contain an empty ID")
		}
	}
	if err := validateSeriesPoint(report.Point, true); err != nil {
		return fmt.Errorf("series latest point: %w", err)
	}
	return nil
}

func validateSeriesPoint(point SeriesPoint, requireLatest bool) error {
	if strings.TrimSpace(point.ID) == "" || strings.TrimSpace(point.Manifest) == "" {
		return fmt.Errorf("point needs an id and manifest")
	}
	if requireLatest && !point.Latest {
		return fmt.Errorf("latest point must set latest=true")
	}
	if err := point.Summary.Validate(); err != nil {
		return fmt.Errorf("point summary: %w", err)
	}
	for _, correlation := range point.Correlations {
		if err := validateBundleCorrelation(correlation); err != nil {
			return err
		}
	}
	for _, ids := range [][]string{point.ChangeIDFilter, point.MutationChangeIDs, point.SornaRuleChangeIDs} {
		for _, id := range ids {
			if strings.TrimSpace(id) == "" {
				return fmt.Errorf("point identifiers cannot contain an empty value")
			}
		}
	}
	return nil
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

// LatestSeriesPoint returns the final point in the ordered history.
func LatestSeriesPoint(series ComparisonSeries) (SeriesPoint, bool) {
	if len(series.Entries) == 0 {
		return SeriesPoint{}, false
	}
	return series.Entries[len(series.Entries)-1], true
}

// NewSeriesLatestReport projects the final point without the other history
// entries.
func NewSeriesLatestReport(series ComparisonSeries) (SeriesLatestReport, error) {
	point, ok := LatestSeriesPoint(series)
	if !ok {
		return SeriesLatestReport{}, fmt.Errorf("comparison series has no points")
	}
	return SeriesLatestReport{
		Schema:         comparisonSeriesLatestSchema,
		Manifest:       series.Manifest,
		ChangeIDFilter: append([]string(nil), series.ChangeIDFilter...),
		Point:          point,
	}, nil
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
	series.Summary.SornaRuleChangesByID = make(map[string]int)
	for _, entry := range manifest.Entries {
		bundlePath := resolveManifestPath(root, entry.Manifest)
		bundle, err := CompareBundleFile(bundlePath)
		if err != nil {
			return ComparisonSeries{}, fmt.Errorf("compare series entry %s: %w", entry.ID, err)
		}
		bundle = FilterBundleChanges(bundle, ids)
		point := newSeriesPoint(entry, bundle, len(series.Entries) == len(manifest.Entries)-1)
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
		for _, ruleID := range point.SornaRuleChangeIDs {
			series.Summary.SornaRuleChangesByID[ruleID]++
		}
	}
	if len(series.Summary.ChangesByID) == 0 {
		series.Summary.ChangesByID = nil
	}
	if len(series.Summary.MutationChangesByID) == 0 {
		series.Summary.MutationChangesByID = nil
	}
	if len(series.Summary.SornaRuleChangesByID) == 0 {
		series.Summary.SornaRuleChangesByID = nil
	}
	series.Summary.CorrelationSummary = summarizeSeriesCorrelations(series.Entries)
	series.Summary.TransitionsBySubsystem = summarizeSeriesTransitions(series.Entries)
	series.Summary.WarningsByMessage = summarizeSeriesWarnings(series.Entries)
	return series, nil
}

// WriteSeriesJSON writes the machine-readable history projection.
func WriteSeriesJSON(w io.Writer, series ComparisonSeries) error {
	if err := series.Validate(); err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(series)
}

// WriteSeriesSummaryJSON writes the machine-readable, point-free history
// projection.
func WriteSeriesSummaryJSON(w io.Writer, series ComparisonSeries) error {
	report := NewSeriesSummaryReport(series)
	if err := report.Validate(); err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

// WriteSeriesLatestJSON writes the machine-readable final-point projection.
func WriteSeriesLatestJSON(w io.Writer, series ComparisonSeries) error {
	report, err := NewSeriesLatestReport(series)
	if err != nil {
		return err
	}
	if err := report.Validate(); err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
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
		if point.Latest {
			label += " [latest]"
		}
		mutationChanges := ""
		if len(point.MutationChangeIDs) > 0 {
			mutationChanges = ", mutation changes=" + strings.Join(point.MutationChangeIDs, ",")
		}
		ruleChanges := ""
		if len(point.SornaRuleChangeIDs) > 0 {
			ruleChanges = ", Sorna rule changes=" + strings.Join(point.SornaRuleChangeIDs, ",")
		}
		if _, err := fmt.Fprintf(w, "    - %s: compatible=%t, changes=%s%s%s\n", label, point.Summary.Compatible, point.Summary.ChangeSummary, mutationChanges, ruleChanges); err != nil {
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

// WriteSeriesLatestText writes the compact operator-oriented final-point
// projection.
func WriteSeriesLatestText(w io.Writer, series ComparisonSeries) error {
	report, err := NewSeriesLatestReport(series)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Sattler latest comparison point\n  manifest: %s\n  id: %s\n  compatible: %t\n  changes: %s\n", report.Manifest, report.Point.ID, report.Point.Summary.Compatible, report.Point.Summary.ChangeSummary); err != nil {
		return err
	}
	if report.Point.Label != "" {
		if _, err := fmt.Fprintf(w, "  label: %s\n", report.Point.Label); err != nil {
			return err
		}
	}
	if len(report.ChangeIDFilter) > 0 {
		if _, err := fmt.Fprintf(w, "  change ID filter: %s\n", strings.Join(report.ChangeIDFilter, ", ")); err != nil {
			return err
		}
	}
	if len(report.Point.MutationChangeIDs) > 0 {
		if _, err := fmt.Fprintf(w, "  mutation changes: %s\n", strings.Join(report.Point.MutationChangeIDs, ",")); err != nil {
			return err
		}
	}
	if len(report.Point.SornaRuleChangeIDs) > 0 {
		if _, err := fmt.Fprintf(w, "  Sorna rule changes: %s\n", strings.Join(report.Point.SornaRuleChangeIDs, ",")); err != nil {
			return err
		}
	}
	if len(report.Point.Correlations) > 0 {
		if _, err := fmt.Fprintln(w, "  correlations:"); err != nil {
			return err
		}
		for _, correlation := range report.Point.Correlations {
			if err := writeBundleCorrelationTextIndented(w, "    ", correlation); err != nil {
				return err
			}
		}
	}
	return nil
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
	ruleIDs := sortedCounts(summary.SornaRuleChangesByID)
	if len(ruleIDs) > 0 {
		if _, err := fmt.Fprintln(w, "  Sorna rule changes by ID:"); err != nil {
			return err
		}
		for _, ruleID := range ruleIDs {
			if _, err := fmt.Fprintf(w, "    - %s: %d\n", ruleID, summary.SornaRuleChangesByID[ruleID]); err != nil {
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
	if len(summary.WarningsByMessage) > 0 {
		if _, err := fmt.Fprintln(w, "  warnings by message:"); err != nil {
			return err
		}
		for _, warning := range sortedCounts(summary.WarningsByMessage) {
			if _, err := fmt.Fprintf(w, "    - %s: %d\n", warning, summary.WarningsByMessage[warning]); err != nil {
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

func summarizeSeriesWarnings(points []SeriesPoint) map[string]int {
	warnings := make(map[string]int)
	for _, point := range points {
		for _, warning := range point.Summary.Warnings {
			if strings.TrimSpace(warning) != "" {
				warnings[warning]++
			}
		}
	}
	if len(warnings) == 0 {
		return nil
	}
	return warnings
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
	if err := decodeStrictJSON(contents, &manifest); err != nil {
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
	if report.SornaRun != nil {
		changes = append(changes, report.SornaRun.Changes...)
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

func newSeriesPoint(entry ComparisonSeriesEntry, bundle BundleComparison, latest bool) SeriesPoint {
	return SeriesPoint{
		ID:                 entry.ID,
		Label:              entry.Label,
		Latest:             latest,
		Manifest:           bundle.Manifest,
		Summary:            bundle.Summary,
		Correlations:       append([]BundleCorrelation(nil), bundle.Correlations...),
		ChangeIDFilter:     append([]string(nil), bundle.ChangeIDFilter...),
		MutationChangeIDs:  bundleMutationChangeIDs(bundle),
		SornaRuleChangeIDs: bundleSornaRuleChangeIDs(bundle),
	}
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

func bundleSornaRuleChangeIDs(report BundleComparison) []string {
	if report.SornaRun == nil {
		return nil
	}
	ids := make([]string, 0, len(report.SornaRun.ChangedRules))
	for _, change := range report.SornaRun.ChangedRules {
		if strings.TrimSpace(change.RuleID) != "" {
			ids = append(ids, change.RuleID)
		}
	}
	sort.Strings(ids)
	return ids
}
