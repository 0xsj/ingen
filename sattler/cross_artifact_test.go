package sattler

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossArtifactFixtureExercisesBundleSornaAndSeries(t *testing.T) {
	fixtureRoot := filepath.Join("testdata", "cross-artifact")
	comparisonPath := filepath.Join(fixtureRoot, "comparison.json")

	bundle, err := CompareBundleFile(comparisonPath)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.CIResult == nil || bundle.SornaRun == nil || bundle.NublarRun == nil || bundle.Custody == nil || bundle.Provenance == nil {
		t.Fatalf("bundle = %+v, want all five declared artifact comparisons", bundle)
	}
	if bundle.Summary.Compatible {
		t.Fatalf("bundle summary = %+v, want retry/provenance drift to remain visible as incompatible", bundle.Summary)
	}
	if bundle.Summary.ChangeSummary.Total != 26 || bundle.Summary.Subsystems["sorna_run"].ChangeSummary.Total != 9 {
		t.Fatalf("bundle change summary = %+v, want 26 total and 9 Sorna changes", bundle.Summary)
	}
	const warning = `ci_result: mutation campaign detail unavailable: Sorna mutation campaign report schema must be ingen.mutation-campaign-result/v1, got "sorna.mutation-campaign-result/v0"`
	if len(bundle.Summary.Warnings) != 1 || bundle.Summary.Warnings[0] != warning {
		t.Fatalf("bundle warnings = %+v, want the explicit producer-detail warning", bundle.Summary.Warnings)
	}
	var contractChange Change
	for _, change := range bundle.CIResult.Changes {
		if change.StableID() == "contract.inputs.contract" {
			contractChange = change
			break
		}
	}
	if contractChange.Identity != ArtifactIdentityReplaced {
		t.Fatalf("contract change = %+v, want replaced artifact identity", contractChange)
	}
	if len(bundle.Correlations) != 4 ||
		bundle.Correlations[0].Relation != BundleCorrelationExactMatch ||
		bundle.Correlations[1].Relation != BundleCorrelationExactMatch ||
		bundle.Correlations[2].Relation != BundleCorrelationExactMatch ||
		bundle.Correlations[3].Relation != BundleCorrelationMismatch {
		t.Fatalf("bundle correlations = %+v, want three exact matches and one mismatch", bundle.Correlations)
	}

	var bundleText bytes.Buffer
	if err := WriteBundleText(&bundleText, bundle); err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"warnings:",
		"sorna run:",
		"nublar-run-to-custody-source after: exact-match",
		"nublar-correlation-to-amber after: mismatch",
		"ci result:",
		"provenance:",
	} {
		if !strings.Contains(bundleText.String(), fragment) {
			t.Fatalf("bundle text = %q, missing %q", bundleText.String(), fragment)
		}
	}

	sorna, err := CompareSornaRunFiles(
		filepath.Join(fixtureRoot, "before-sorna-run.json"),
		filepath.Join(fixtureRoot, "after-sorna-run.json"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !sorna.Compatible || sorna.Transition.Classification != TransitionChanged {
		t.Fatalf("Sorna comparison = %+v, want compatible changed verdict", sorna)
	}
	if len(sorna.ChangedRules) != 1 || sorna.ChangedRules[0].RuleID != "document.create.accepted" {
		t.Fatalf("Sorna changed rules = %+v, want one changed rule", sorna.ChangedRules)
	}
	if sorna.Before.Mutation == nil || sorna.After.Mutation == nil || sorna.Before.Mutation.Outcome != "killed" || sorna.After.Mutation.Outcome != "survived" {
		t.Fatalf("Sorna mutation summaries = %+v -> %+v, want killed to survived", sorna.Before.Mutation, sorna.After.Mutation)
	}
	if !hasChangeID(sorna.Changes, "rules.document.create.accepted.status") {
		t.Fatalf("Sorna changes = %+v, want stable rule status ID", sorna.Changes)
	}

	series, err := CompareSeriesManifestFile(filepath.Join(fixtureRoot, "series.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(series.Entries) != 1 || !series.Entries[0].Latest || series.Summary.Entries != 1 || series.Summary.Compatible != 0 || series.Summary.Incompatible != 1 {
		t.Fatalf("series = %+v, want one latest incompatible point", series)
	}
	if series.Summary.TotalChanges != 26 || series.Summary.ChangesByID["verdict.status"] != 3 || series.Summary.ChangesByID["contract.inputs.contract"] != 1 {
		t.Fatalf("series changes by ID = %+v, want aggregated boundary changes", series.Summary.ChangesByID)
	}
	if series.Summary.SornaRuleChangesByID["document.create.accepted"] != 1 || len(series.Entries[0].SornaRuleChangeIDs) != 1 || series.Entries[0].SornaRuleChangeIDs[0] != "document.create.accepted" {
		t.Fatalf("series Sorna rule changes = %+v, want one indexed changed rule", series.Summary)
	}
	if series.Summary.CorrelationSummary.Observations != 4 || series.Summary.CorrelationSummary.ByRelation["exact-match"] != 3 || series.Summary.CorrelationSummary.ByRelation["mismatch"] != 1 {
		t.Fatalf("series correlations = %+v, want four identity observations", series.Summary.CorrelationSummary)
	}
	if series.Summary.WarningsByMessage[warning] != 1 {
		t.Fatalf("series warnings = %+v, want one propagated warning", series.Summary.WarningsByMessage)
	}
	var seriesText bytes.Buffer
	if err := WriteSeriesText(&seriesText, series); err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"Sorna rule changes by ID:", "document.create.accepted: 1", "Sorna rule changes=document.create.accepted"} {
		if !strings.Contains(seriesText.String(), fragment) {
			t.Fatalf("series text = %q, missing %q", seriesText.String(), fragment)
		}
	}
}
