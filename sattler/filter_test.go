package sattler

import "testing"

func TestFilterBundleChangesRecalculatesSummaries(t *testing.T) {
	report := BundleComparison{
		CIResult: &Comparison{
			Compatible: true,
			Changes: []Change{
				NewChange("verdict", "status", "passed", "failed"),
				NewChange("verdict", "exit_code", 0, 1),
			},
		},
		NublarRun: &NublarRunComparison{
			Compatible: true,
			Changes: []Change{
				NewChange("verdict", "status", "passed", "failed"),
			},
		},
	}

	filtered := FilterBundleChanges(report, []string{"verdict.status", "verdict.status"})
	if len(filtered.ChangeIDFilter) != 1 || filtered.ChangeIDFilter[0] != "verdict.status" {
		t.Fatalf("change ID filter = %+v, want one sorted unique ID", filtered.ChangeIDFilter)
	}
	if filtered.CIResult.ChangeSummary.Total != 1 || filtered.NublarRun.ChangeSummary.Total != 1 {
		t.Fatalf("filtered subsystem summaries = %+v / %+v, want one change each", filtered.CIResult.ChangeSummary, filtered.NublarRun.ChangeSummary)
	}
	if filtered.Summary.ChangeSummary.Total != 2 {
		t.Fatalf("filtered bundle summary = %+v, want two changes", filtered.Summary.ChangeSummary)
	}
}

func TestFilterChangesDerivesIDsForManualChanges(t *testing.T) {
	changes := FilterChanges([]Change{{Category: "context", Field: "source.root"}}, []string{"context.source.root"})
	if len(changes) != 1 || changes[0].ID != "context.source.root" {
		t.Fatalf("filtered manual changes = %+v, want derived stable ID", changes)
	}
}

func TestStandaloneFiltersRecordSelectedIDs(t *testing.T) {
	comparison := FilterComparisonChanges(Comparison{Changes: []Change{
		NewChange("verdict", "status", "passed", "failed"),
	}}, []string{"verdict.status"})
	if len(comparison.ChangeIDFilter) != 1 || comparison.ChangeIDFilter[0] != "verdict.status" || comparison.ChangeSummary.Total != 1 {
		t.Fatalf("CI filtered report = %+v, want selected ID and one change", comparison)
	}

	nublar := FilterNublarRunChanges(NublarRunComparison{Changes: []Change{
		NewChange("check", "checks.campaign", "passed", "failed"),
	}}, []string{"verdict.status"})
	if len(nublar.Changes) != 0 || nublar.ChangeSummary.Total != 0 {
		t.Fatalf("Nublar filtered report = %+v, want no unmatched changes", nublar)
	}
}

func TestFilterSornaRunChangesKeepsChangedRulesAligned(t *testing.T) {
	report := SornaRunComparison{
		Changes: []Change{
			NewChange("verdict", "status", "pass", "fail"),
			NewChange("rules.document.create.accepted", "status", "pass", "fail"),
		},
		ChangedRules: []SornaRuleStatusChange{{RuleID: "document.create.accepted", Before: "pass", After: "fail"}},
	}

	filtered := FilterSornaRunChanges(report, []string{"verdict.status"})
	if len(filtered.Changes) != 1 || len(filtered.ChangedRules) != 0 {
		t.Fatalf("verdict-filtered Sorna report = %+v, want no changed rule detail", filtered)
	}

	filtered = FilterSornaRunChanges(report, []string{"rules.document.create.accepted.status"})
	if len(filtered.Changes) != 1 || len(filtered.ChangedRules) != 1 || filtered.ChangedRules[0].RuleID != "document.create.accepted" {
		t.Fatalf("rule-filtered Sorna report = %+v, want matching changed rule detail", filtered)
	}
}
