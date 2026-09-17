package sattler

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
)

// SeriesQuerySchema identifies the targeted history projection.
const SeriesQuerySchema = "ingen.sattler-series-query/v0"

// SeriesQueryKind identifies the supported exact history selectors.
type SeriesQueryKind string

const (
	SeriesQueryRule     SeriesQueryKind = "rule"
	SeriesQueryMutation SeriesQueryKind = "mutation"
	SeriesQueryContract SeriesQueryKind = "contract"
	SeriesQueryProvider SeriesQueryKind = "provider"
	SeriesQueryWorkflow SeriesQueryKind = "workflow"
)

// SeriesQuerySelector identifies one exact value to find across an ordered
// series. It is separate from boundary change IDs and does not reinterpret
// producer-owned fields.
type SeriesQuerySelector struct {
	Kind  SeriesQueryKind `json:"kind"`
	Value string          `json:"value"`
}

// Validate checks that a history query has one supported kind and a value.
func (selector SeriesQuerySelector) Validate() error {
	if strings.TrimSpace(selector.Value) == "" {
		return fmt.Errorf("series query value is required")
	}
	switch selector.Kind {
	case SeriesQueryRule, SeriesQueryMutation, SeriesQueryContract, SeriesQueryProvider, SeriesQueryWorkflow:
		return nil
	default:
		return fmt.Errorf("series query kind %q is unsupported", selector.Kind)
	}
}

// SeriesQuerySummary counts matching points and side observations.
type SeriesQuerySummary struct {
	Points  int `json:"points"`
	Matches int `json:"matches"`
}

// Validate checks the aggregate counts for a targeted query report.
func (summary SeriesQuerySummary) Validate(entryCount, matchCount int) error {
	if summary.Points < 0 || summary.Matches < 0 {
		return fmt.Errorf("series query summary counts must not be negative")
	}
	if summary.Points != entryCount || summary.Matches != matchCount {
		return fmt.Errorf("series query summary counts must match emitted entries and matches")
	}
	return nil
}

// SeriesQueryMatch records one exact observation at a source artifact.
type SeriesQueryMatch struct {
	Subsystem string                 `json:"subsystem"`
	Side      string                 `json:"side"`
	Field     string                 `json:"field"`
	Value     string                 `json:"value"`
	Source    InvestigationSourceRef `json:"source"`
}

// Validate checks one exact selector match and its source observation.
func (match SeriesQueryMatch) Validate() error {
	if !isInvestigationSubsystem(match.Subsystem) {
		return fmt.Errorf("query match subsystem %q is unsupported", match.Subsystem)
	}
	switch match.Side {
	case "before", "after":
	default:
		return fmt.Errorf("query match side %q is unsupported", match.Side)
	}
	if strings.TrimSpace(match.Field) == "" || strings.TrimSpace(match.Value) == "" {
		return fmt.Errorf("query match needs a field and value")
	}
	if err := validateInvestigationSource(match.Source); err != nil {
		return fmt.Errorf("query match source: %w", err)
	}
	if match.Source.Subsystem != match.Subsystem || match.Source.Side != match.Side || match.Source.Field != match.Field {
		return fmt.Errorf("query match source must identify the same subsystem, side, and field")
	}
	return nil
}

// SeriesQueryPoint retains the original series point and adds only matching
// observations. The embedded point keeps its bundle manifest reference.
type SeriesQueryPoint struct {
	Point   SeriesPoint        `json:"point"`
	Matches []SeriesQueryMatch `json:"matches"`
}

// Validate checks one retained point and its exact selector matches.
func (point SeriesQueryPoint) Validate() (int, error) {
	if err := validateSeriesPoint(point.Point, false); err != nil {
		return 0, fmt.Errorf("query point: %w", err)
	}
	if len(point.Matches) == 0 {
		return 0, fmt.Errorf("query point needs at least one match")
	}
	for index, match := range point.Matches {
		if err := match.Validate(); err != nil {
			return 0, fmt.Errorf("query match %d: %w", index, err)
		}
	}
	return len(point.Matches), nil
}

// SeriesQueryReport is a deterministic targeted projection over a series.
type SeriesQueryReport struct {
	Schema   string              `json:"schema"`
	Manifest string              `json:"manifest,omitempty"`
	Query    SeriesQuerySelector `json:"query"`
	Summary  SeriesQuerySummary  `json:"summary"`
	Entries  []SeriesQueryPoint  `json:"entries"`
}

// Validate checks the compatibility-treated targeted history projection
// without changing exact-match or producer-owned semantics.
func (report SeriesQueryReport) Validate() error {
	if report.Schema != SeriesQuerySchema {
		return fmt.Errorf("series query schema must be %s, got %q", SeriesQuerySchema, report.Schema)
	}
	if report.Manifest != "" && strings.TrimSpace(report.Manifest) == "" {
		return fmt.Errorf("series query manifest cannot be empty when present")
	}
	if err := report.Query.Validate(); err != nil {
		return fmt.Errorf("series query: %w", err)
	}
	matchCount := 0
	pointIDs := make(map[string]struct{}, len(report.Entries))
	latestCount := 0
	for index, entry := range report.Entries {
		count, err := entry.Validate()
		if err != nil {
			return fmt.Errorf("series query entry %d: %w", index, err)
		}
		if _, exists := pointIDs[entry.Point.ID]; exists {
			return fmt.Errorf("series query point ID %q is duplicated", entry.Point.ID)
		}
		pointIDs[entry.Point.ID] = struct{}{}
		if entry.Point.Latest {
			latestCount++
		}
		matchCount += count
	}
	if latestCount > 1 {
		return fmt.Errorf("series query can contain at most one latest point")
	}
	if err := report.Summary.Validate(len(report.Entries), matchCount); err != nil {
		return fmt.Errorf("series query: %w", err)
	}
	return nil
}

// QuerySeriesManifestFile finds exact selector matches while preserving the
// input series order and each point's original bundle manifest reference.
func QuerySeriesManifestFile(seriesPath string, selector SeriesQuerySelector) (SeriesQueryReport, error) {
	if err := selector.Validate(); err != nil {
		return SeriesQueryReport{}, err
	}
	manifest, err := loadComparisonSeriesManifest(seriesPath)
	if err != nil {
		return SeriesQueryReport{}, err
	}
	report := SeriesQueryReport{
		Schema:   SeriesQuerySchema,
		Manifest: seriesPath,
		Query:    selector,
		Entries:  make([]SeriesQueryPoint, 0),
	}
	root := filepath.Dir(seriesPath)
	for index, entry := range manifest.Entries {
		bundlePath := resolveManifestPath(root, entry.Manifest)
		bundle, err := CompareBundleFile(bundlePath)
		if err != nil {
			return SeriesQueryReport{}, fmt.Errorf("query series entry %s: %w", entry.ID, err)
		}
		matches := matchSeriesQuery(bundle, selector)
		if len(matches) == 0 {
			continue
		}
		point := newSeriesPoint(entry, bundle, index == len(manifest.Entries)-1)
		report.Entries = append(report.Entries, SeriesQueryPoint{Point: point, Matches: matches})
		report.Summary.Points++
		report.Summary.Matches += len(matches)
	}
	return report, nil
}

// WriteSeriesQueryJSON writes the machine-readable targeted history report.
func WriteSeriesQueryJSON(w io.Writer, report SeriesQueryReport) error {
	if err := report.Validate(); err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

// WriteSeriesQueryText writes the operator-oriented targeted history report.
func WriteSeriesQueryText(w io.Writer, report SeriesQueryReport) error {
	if _, err := fmt.Fprintf(w, "Sattler series query\n  manifest: %s\n  query: %s=%q\n  matching points: %d\n  matches: %d\n", report.Manifest, report.Query.Kind, report.Query.Value, report.Summary.Points, report.Summary.Matches); err != nil {
		return err
	}
	if len(report.Entries) == 0 {
		_, err := fmt.Fprintln(w, "  entries: none")
		return err
	}
	if _, err := fmt.Fprintln(w, "  entries:"); err != nil {
		return err
	}
	for _, entry := range report.Entries {
		label := entry.Point.ID
		if entry.Point.Label != "" {
			label += " (" + entry.Point.Label + ")"
		}
		if entry.Point.Latest {
			label += " [latest]"
		}
		if _, err := fmt.Fprintf(w, "    - %s: manifest=%s, compatible=%t, changes=%s\n", label, entry.Point.Manifest, entry.Point.Summary.Compatible, entry.Point.Summary.ChangeSummary); err != nil {
			return err
		}
		for _, match := range entry.Matches {
			if _, err := fmt.Fprintf(w, "      - %s %s %s=%q", match.Subsystem, match.Side, match.Field, match.Value); err != nil {
				return err
			}
			if match.Source.Path != "" {
				if _, err := fmt.Fprintf(w, " (%s)", match.Source.Path); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
	}
	return nil
}

func matchSeriesQuery(bundle BundleComparison, selector SeriesQuerySelector) []SeriesQueryMatch {
	matches := make([]SeriesQueryMatch, 0)
	add := func(subsystem, side, field, value, path string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		matches = append(matches, SeriesQueryMatch{
			Subsystem: subsystem,
			Side:      side,
			Field:     field,
			Value:     value,
			Source:    InvestigationSourceRef{Subsystem: subsystem, Side: side, Path: path, Field: field},
		})
	}
	addFileRef := func(subsystem, side, field, path, sha256 string) {
		if selector.Value != path && selector.Value != sha256 {
			return
		}
		value := path
		if value == "" {
			value = sha256
		}
		add(subsystem, side, field, value, path)
	}

	switch selector.Kind {
	case SeriesQueryRule:
		if bundle.SornaRun != nil {
			for _, rule := range bundle.SornaRun.ChangedRules {
				if rule.RuleID != selector.Value {
					continue
				}
				field := "rules." + rule.RuleID + ".status"
				add("sorna_run", "before", field, rule.Before, bundle.SornaRun.Before.Path)
				add("sorna_run", "after", field, rule.After, bundle.SornaRun.After.Path)
			}
		}
	case SeriesQueryMutation:
		if bundle.SornaRun != nil {
			if mutation := bundle.SornaRun.Before.Mutation; mutation != nil && mutation.ID == selector.Value {
				add("sorna_run", "before", "mutation."+mutation.ID+".outcome", mutation.Outcome, bundle.SornaRun.Before.Path)
			}
			if mutation := bundle.SornaRun.After.Mutation; mutation != nil && mutation.ID == selector.Value {
				add("sorna_run", "after", "mutation."+mutation.ID+".outcome", mutation.Outcome, bundle.SornaRun.After.Path)
			}
		}
		if bundle.CIResult != nil && bundle.CIResult.MutationCampaign != nil {
			for _, mutation := range bundle.CIResult.MutationCampaign.ChangedMutations {
				if mutation.MutationID != selector.Value {
					continue
				}
				if mutation.Before != nil {
					add("ci_result", "before", "producer-report.report", mutationStateValueString(mutation.Before), bundle.CIResult.Before.Path)
				}
				if mutation.After != nil {
					add("ci_result", "after", "producer-report.report", mutationStateValueString(mutation.After), bundle.CIResult.After.Path)
				}
			}
		}
	case SeriesQueryContract:
		if bundle.CIResult != nil {
			if ref, ok := bundle.CIResult.Before.Inputs["contract"]; ok {
				addFileRef("ci_result", "before", "inputs.contract", ref.Path, ref.SHA256)
			}
			if ref, ok := bundle.CIResult.After.Inputs["contract"]; ok {
				addFileRef("ci_result", "after", "inputs.contract", ref.Path, ref.SHA256)
			}
		}
		if bundle.SornaRun != nil {
			addSornaContract := func(side string, contract SornaRunContractSummary, path string) {
				if selector.Value != contract.ID && selector.Value != contract.SHA256 && selector.Value != fmt.Sprintf("%s@%d", contract.ID, contract.Version) {
					return
				}
				add("sorna_run", side, "contract", fmt.Sprintf("%s@%d", contract.ID, contract.Version), path)
			}
			addSornaContract("before", bundle.SornaRun.Before.Contract, bundle.SornaRun.Before.Path)
			addSornaContract("after", bundle.SornaRun.After.Contract, bundle.SornaRun.After.Path)
		}
	case SeriesQueryProvider:
		if bundle.CIResult != nil {
			if ref, ok := bundle.CIResult.Before.Inputs["provider"]; ok {
				addFileRef("ci_result", "before", "inputs.provider", ref.Path, ref.SHA256)
			}
			if ref, ok := bundle.CIResult.After.Inputs["provider"]; ok {
				addFileRef("ci_result", "after", "inputs.provider", ref.Path, ref.SHA256)
			}
		}
	case SeriesQueryWorkflow:
		if bundle.NublarRun != nil {
			addWorkflow := func(side string, workflow NublarWorkflowSummary, path string) {
				if selector.Value == workflow.ID {
					add("nublar_run", side, "workflow.id", workflow.ID, path)
				}
				if selector.Value == workflow.File.Path || selector.Value == workflow.File.SHA256 {
					value := workflow.File.Path
					if value == "" {
						value = workflow.File.SHA256
					}
					add("nublar_run", side, "workflow.file", value, path)
				}
			}
			addWorkflow("before", bundle.NublarRun.Before.Workflow, bundle.NublarRun.Before.Path)
			addWorkflow("after", bundle.NublarRun.After.Workflow, bundle.NublarRun.After.Path)
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Subsystem != matches[j].Subsystem {
			return matches[i].Subsystem < matches[j].Subsystem
		}
		if matches[i].Side != matches[j].Side {
			return matches[i].Side < matches[j].Side
		}
		if matches[i].Field != matches[j].Field {
			return matches[i].Field < matches[j].Field
		}
		return matches[i].Value < matches[j].Value
	})
	return matches
}

func mutationStateValueString(state *MutationState) string {
	if state == nil {
		return ""
	}
	if state.Outcome != "" {
		return state.Outcome
	}
	return state.Status
}
