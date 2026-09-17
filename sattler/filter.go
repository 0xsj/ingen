package sattler

import (
	"sort"
	"strings"
)

// FilterChanges returns only boundary changes whose stable IDs are selected.
// Empty or whitespace-only filters select every change.
func FilterChanges(changes []Change, ids []string) []Change {
	selected := normalizedChangeIDs(ids)
	filtered := make([]Change, 0, len(changes))
	for _, change := range changes {
		if change.ID == "" {
			change.ID = change.StableID()
		}
		if len(selected) == 0 {
			filtered = append(filtered, change)
			continue
		}
		if _, ok := selected[change.StableID()]; ok {
			filtered = append(filtered, change)
		}
	}
	return filtered
}

// FilterBundleChanges applies a stable-ID filter to all independent boundary
// reports and recalculates their summaries. CI producer-owned mutation details
// remain untouched because they use a separate producer-specific model.
func FilterBundleChanges(report BundleComparison, ids []string) BundleComparison {
	report.ChangeIDFilter = sortedChangeIDs(ids)
	if report.CIResult != nil {
		comparison := FilterComparisonChanges(*report.CIResult, ids)
		report.CIResult = &comparison
	}
	if report.SornaRun != nil {
		comparison := FilterSornaRunChanges(*report.SornaRun, ids)
		report.SornaRun = &comparison
	}
	if report.NublarRun != nil {
		comparison := FilterNublarRunChanges(*report.NublarRun, ids)
		report.NublarRun = &comparison
	}
	if report.Custody != nil {
		comparison := FilterLockwoodCustodyChanges(*report.Custody, ids)
		report.Custody = &comparison
	}
	if report.Provenance != nil {
		comparison := FilterAmberProvenanceChanges(*report.Provenance, ids)
		report.Provenance = &comparison
	}
	report.Summary = SummarizeBundle(report)
	report.Correlations = CorrelateBundle(report)
	return report
}

// FilterComparisonChanges applies a stable-ID filter to a CI comparison.
func FilterComparisonChanges(report Comparison, ids []string) Comparison {
	report.ChangeIDFilter = sortedChangeIDs(ids)
	report.Changes = FilterChanges(report.Changes, ids)
	report.ChangeSummary = SummarizeChanges(report.Changes)
	return report
}

// FilterNublarRunChanges applies a stable-ID filter to a Nublar comparison.
func FilterNublarRunChanges(report NublarRunComparison, ids []string) NublarRunComparison {
	report.ChangeIDFilter = sortedChangeIDs(ids)
	report.Changes = FilterChanges(report.Changes, ids)
	report.ChangeSummary = SummarizeChanges(report.Changes)
	return report
}

// FilterLockwoodCustodyChanges applies a stable-ID filter to a custody comparison.
func FilterLockwoodCustodyChanges(report LockwoodCustodyComparison, ids []string) LockwoodCustodyComparison {
	report.ChangeIDFilter = sortedChangeIDs(ids)
	report.Changes = FilterChanges(report.Changes, ids)
	report.ChangeSummary = SummarizeChanges(report.Changes)
	return report
}

// FilterAmberProvenanceChanges applies a stable-ID filter to a provenance comparison.
func FilterAmberProvenanceChanges(report AmberProvenanceComparison, ids []string) AmberProvenanceComparison {
	report.ChangeIDFilter = sortedChangeIDs(ids)
	report.Changes = FilterChanges(report.Changes, ids)
	report.ChangeSummary = SummarizeChanges(report.Changes)
	return report
}

// FilterSornaRunChanges applies a stable-ID filter to a Sorna run comparison.
func FilterSornaRunChanges(report SornaRunComparison, ids []string) SornaRunComparison {
	report.ChangeIDFilter = sortedChangeIDs(ids)
	report.Changes = FilterChanges(report.Changes, ids)
	if len(normalizedChangeIDs(ids)) > 0 {
		selectedRules := make(map[string]struct{})
		for _, change := range report.Changes {
			id := change.StableID()
			if strings.HasPrefix(id, "rules.") && strings.HasSuffix(id, ".status") {
				selectedRules[id] = struct{}{}
			}
		}
		filteredRules := make([]SornaRuleStatusChange, 0, len(report.ChangedRules))
		for _, rule := range report.ChangedRules {
			if _, ok := selectedRules["rules."+rule.RuleID+".status"]; ok {
				filteredRules = append(filteredRules, rule)
			}
		}
		report.ChangedRules = filteredRules
	}
	report.ChangeSummary = SummarizeChanges(report.Changes)
	return report
}

func sortedChangeIDs(ids []string) []string {
	selected := normalizedChangeIDs(ids)
	result := make([]string, 0, len(selected))
	for id := range selected {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

func normalizedChangeIDs(ids []string) map[string]struct{} {
	selected := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			selected[id] = struct{}{}
		}
	}
	return selected
}
