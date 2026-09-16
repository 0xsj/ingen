package sattler

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"ingen/core/ciresult"
)

func TestCompareReportsVerdictAndInputChangesDeterministically(t *testing.T) {
	before := validArtifact()
	after := validArtifact()
	after.Status = "failed"
	after.ExitCode = 1
	after.Inputs["contract"] = ciresult.FileRef{Path: "contract-v2.yaml", SHA256: strings.Repeat("b", 64)}
	after.Report = json.RawMessage(`{"result":"failed"}`)
	after.Explanation = json.RawMessage(`{"reason":"changed"}`)

	report := Compare(before, after)
	if !report.Compatible {
		t.Fatal("comparison is incompatible despite stable producer and source identity")
	}
	if got, want := len(report.Changes), 5; got != want {
		t.Fatalf("change count = %d, want %d: %+v", got, want, report.Changes)
	}
	assertChange(t, report.Changes[0], "verdict", "status")
	assertChange(t, report.Changes[1], "verdict", "exit_code")
	assertChange(t, report.Changes[2], "contract", "inputs.contract")
	assertChange(t, report.Changes[3], "producer-report", "report")
	assertChange(t, report.Changes[4], "producer-report", "explanation")
}

func TestCompareIgnoresJSONFormattingForOpaqueReports(t *testing.T) {
	before := validArtifact()
	after := validArtifact()
	after.Report = json.RawMessage(`{"ok":true,"items":[1,2]}`)
	before.Report = json.RawMessage(`{ "items": [1, 2], "ok": true }`)

	report := Compare(before, after)
	if len(report.Changes) != 0 {
		t.Fatalf("format-only report change produced changes: %+v", report.Changes)
	}
}

func TestCompareKeepsDifferentSourceComparable(t *testing.T) {
	before := validArtifact()
	after := validArtifact()
	after.Source.Root = "/other/workspace"

	report := Compare(before, after)
	if !report.Compatible {
		t.Fatal("comparison marked a source context change incompatible")
	}
	if len(report.Changes) != 1 || report.Changes[0].Field != "source.root" {
		t.Fatalf("source change = %+v, want only source.root", report.Changes)
	}
}

func TestCompareExplainsIncompatibleProducerIdentity(t *testing.T) {
	before := validArtifact()
	after := validArtifact()
	after.Tool = "paddock"

	report := Compare(before, after)
	if report.Compatible {
		t.Fatal("comparison marked different producers compatible")
	}
	if len(report.CompatibilityReasons) != 1 || !strings.Contains(report.CompatibilityReasons[0], "tool changed") {
		t.Fatalf("compatibility reasons = %+v, want tool-change explanation", report.CompatibilityReasons)
	}
}

func TestCompareAddsSornaMutationCampaignDetail(t *testing.T) {
	before := validArtifact()
	before.Kind = "mutation-campaign"
	before.Report = json.RawMessage(`{
        "schema":"ingen.mutation-campaign-result/v1",
        "status":"passed",
        "plan":{"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
        "summary":{"total":2,"killed":2,"survived":0,"inconclusive":0,"other":0,"errors":0},
        "entries":[
            {"mutation_id":"alpha","status":"passed","outcome":"killed"},
            {"mutation_id":"beta","status":"passed","outcome":"killed"}
        ]
    }`)
	after := validArtifact()
	after.Kind = "mutation-campaign"
	after.Report = json.RawMessage(`{
        "schema":"ingen.mutation-campaign-result/v1",
        "status":"failed",
        "plan":{"sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
        "summary":{"total":2,"killed":1,"survived":1,"inconclusive":0,"other":0,"errors":0},
        "entries":[
            {"mutation_id":"alpha","status":"failed","outcome":"survived"},
            {"mutation_id":"beta","status":"passed","outcome":"killed"}
        ]
    }`)

	report := Compare(before, after)
	if report.MutationCampaign == nil {
		t.Fatal("comparison did not expose the recognized mutation campaign")
	}
	if got, want := report.MutationCampaign.Before.Killed, 2; got != want {
		t.Fatalf("before killed = %d, want %d", got, want)
	}
	if got, want := report.MutationCampaign.After.Survived, 1; got != want {
		t.Fatalf("after survived = %d, want %d", got, want)
	}
	if got, want := len(report.MutationCampaign.ChangedMutations), 1; got != want {
		t.Fatalf("changed mutations = %d, want %d", got, want)
	}
	change := report.MutationCampaign.ChangedMutations[0]
	if change.MutationID != "alpha" || change.Before.Outcome != "killed" || change.After.Outcome != "survived" {
		t.Fatalf("mutation change = %+v, want alpha killed -> survived", change)
	}
}

func TestCompareSurfacesUnavailableMutationCampaignDetail(t *testing.T) {
	before := validArtifact()
	before.Kind = "mutation-campaign"
	before.Report = nil
	after := validArtifact()
	after.Kind = "mutation-campaign"
	after.Report = json.RawMessage(`{"schema":"ingen.mutation-campaign-result/v1"}`)

	report := Compare(before, after)
	if report.MutationCampaign != nil {
		t.Fatal("comparison exposed campaign detail from an unavailable report")
	}
	if len(report.Warnings) != 1 || !strings.Contains(report.Warnings[0], "mutation campaign detail unavailable") {
		t.Fatalf("warnings = %+v, want an adapter warning", report.Warnings)
	}

	var output bytes.Buffer
	if err := WriteText(&output, report); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "warnings:") {
		t.Fatalf("text output = %q, want warnings section", output.String())
	}
}

func TestCompareClassifiesKnownInputRoles(t *testing.T) {
	before := validArtifact()
	after := validArtifact()
	after.Inputs["contract"] = ciresult.FileRef{Path: "contract-v2.yaml"}
	after.Inputs["plan"] = ciresult.FileRef{Path: "plan-v2.json"}
	after.Inputs["provider"] = ciresult.FileRef{Path: "provider-v2.yaml"}
	after.Inputs["environment"] = ciresult.FileRef{Path: "environment-v2.json"}
	after.Inputs["custom-record"] = ciresult.FileRef{Path: "custom-v2.json"}

	report := Compare(before, after)
	want := map[string]string{
		"inputs.contract":      "contract",
		"inputs.plan":          "plan",
		"inputs.provider":      "provider",
		"inputs.environment":   "environment",
		"inputs.custom-record": "input",
	}
	for _, change := range report.Changes {
		if category, ok := want[change.Field]; ok {
			if change.Category != category {
				t.Errorf("%s category = %q, want %q", change.Field, change.Category, category)
			}
			delete(want, change.Field)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing classified changes: %v", want)
	}
}

func TestWriteTextNamesTheBoundaryAndChanges(t *testing.T) {
	before := validArtifact()
	after := validArtifact()
	after.Status = "failed"
	after.ExitCode = 1
	report := Compare(before, after)
	report.Before.Path = "before.json"
	report.After.Path = "after.json"

	var output bytes.Buffer
	if err := WriteText(&output, report); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, fragment := range []string{"Sattler comparison", "compatible: true", "verdict status"} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("text output = %q, missing %q", text, fragment)
		}
	}
}

func assertChange(t *testing.T, change Change, category, field string) {
	t.Helper()
	if change.Category != category || change.Field != field {
		t.Fatalf("change = %+v, want %s/%s", change, category, field)
	}
}

func validArtifact() ciresult.Artifact {
	return ciresult.Artifact{
		Schema:    ciresult.Schema,
		Tool:      "sorna",
		Kind:      "behavioral-verification",
		Status:    "passed",
		ExitCode:  0,
		CreatedAt: "2026-09-16T10:00:00Z",
		Source:    ciresult.Source{Root: "/workspace/project", ModulePath: "example/project"},
		Inputs: map[string]ciresult.FileRef{
			"contract": {Path: "contract.yaml", SHA256: strings.Repeat("a", 64)},
		},
		Report:      json.RawMessage(`{"result":"passed"}`),
		Explanation: json.RawMessage(`{"reason":"clean"}`),
	}
}
