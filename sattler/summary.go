package sattler

import (
	"fmt"
	"sort"
	"strings"
)

// ChangeSummary counts observable changes without assigning them a quality
// score or inferring their cause.
type ChangeSummary struct {
	Total      int            `json:"total"`
	ByCategory map[string]int `json:"by_category,omitempty"`
}

// Validate checks the count invariants used by compatibility-treated
// projections. It does not assign meaning to a category.
func (summary ChangeSummary) Validate() error {
	if summary.Total < 0 {
		return fmt.Errorf("change summary total must not be negative")
	}
	total := 0
	for category, count := range summary.ByCategory {
		if strings.TrimSpace(category) == "" {
			return fmt.Errorf("change summary category must not be empty")
		}
		if count < 0 {
			return fmt.Errorf("change summary category %q count must not be negative", category)
		}
		total += count
	}
	if total != summary.Total {
		return fmt.Errorf("change summary category counts must equal total")
	}
	return nil
}

// SummarizeChanges returns deterministic category counts for a change list.
func SummarizeChanges(changes []Change) ChangeSummary {
	summary := ChangeSummary{Total: len(changes)}
	if len(changes) == 0 {
		return summary
	}
	summary.ByCategory = make(map[string]int)
	for _, change := range changes {
		summary.ByCategory[change.Category]++
	}
	return summary
}

func (summary ChangeSummary) String() string {
	if summary.Total == 0 {
		return "none"
	}
	names := make([]string, 0, len(summary.ByCategory))
	for name := range summary.ByCategory {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s=%d", name, summary.ByCategory[name]))
	}
	return fmt.Sprintf("total %d (%s)", summary.Total, strings.Join(parts, ", "))
}
