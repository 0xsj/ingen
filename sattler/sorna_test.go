package sattler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareSornaRunsReportsRuleStatusChanges(t *testing.T) {
	root := t.TempDir()
	beforePath := filepath.Join(root, "before.json")
	afterPath := filepath.Join(root, "after.json")
	writeSornaRunFile(t, beforePath, sornaRunFixture("run-before", 1, "pass", "all rules passed", "killed", "completed", 0))
	writeSornaRunFile(t, afterPath, sornaRunFixture("run-after", 1, "fail", "one rule failed", "survived", "failed", 1))

	report, err := CompareSornaRunFiles(beforePath, afterPath)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Compatible {
		t.Fatalf("Sorna comparison incompatible = %+v, want same contract to remain comparable", report.CompatibilityReasons)
	}
	if report.Transition.Field != "contract_verdict.status" || report.Transition.Classification != TransitionChanged {
		t.Fatalf("transition = %+v, want changed contract verdict", report.Transition)
	}
	if len(report.ChangedRules) != 1 || report.ChangedRules[0].RuleID != "document.create.accepted" || report.ChangedRules[0].Before != "pass" || report.ChangedRules[0].After != "fail" {
		t.Fatalf("changed rules = %+v, want one alpha rule status change", report.ChangedRules)
	}
	if report.Before.Rules["document.create.accepted"].Reason != "" || report.After.Rules["document.create.accepted"].Reason != "validation failed" {
		t.Fatalf("rule summaries = %+v -> %+v, want compact reasons", report.Before.Rules, report.After.Rules)
	}
	if report.Before.Mutation == nil || report.After.Mutation == nil || report.Before.Mutation.Outcome != "killed" || report.After.Mutation.Outcome != "survived" {
		t.Fatalf("mutation summaries = %+v -> %+v, want optional linkage", report.Before.Mutation, report.After.Mutation)
	}
	if !hasChangeID(report.Changes, "rules.document.create.accepted.status") {
		t.Fatalf("changes = %+v, want stable rule status ID", report.Changes)
	}

	var output bytes.Buffer
	if err := WriteSornaText(&output, report); err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"Sattler Sorna run comparison", "contract: document-pipeline@1 -> document-pipeline@1", "rules.document.create.accepted status", "id=rules.document.create.accepted.status"} {
		if !strings.Contains(output.String(), fragment) {
			t.Fatalf("Sorna text = %q, missing %q", output.String(), fragment)
		}
	}
}

func TestCompareSornaRunsMarksContractDriftIncompatible(t *testing.T) {
	root := t.TempDir()
	beforePath := filepath.Join(root, "before.json")
	afterPath := filepath.Join(root, "after.json")
	writeSornaRunFile(t, beforePath, sornaRunFixture("run-before", 1, "pass", "all rules passed", "killed", "completed", 0))
	writeSornaRunFile(t, afterPath, sornaRunFixture("run-after", 2, "pass", "all rules passed", "killed", "completed", 0))

	report, err := CompareSornaRunFiles(beforePath, afterPath)
	if err != nil {
		t.Fatal(err)
	}
	if report.Compatible || report.Transition.Classification != TransitionIncompatible {
		t.Fatalf("comparison = %+v, want incompatible contract transition", report)
	}
	if len(report.CompatibilityReasons) != 1 || !strings.Contains(report.CompatibilityReasons[0], "contract version changed") {
		t.Fatalf("compatibility reasons = %+v, want contract version drift", report.CompatibilityReasons)
	}
}

func TestFilterSornaRunChangesUsesStableRuleIDs(t *testing.T) {
	root := t.TempDir()
	beforePath := filepath.Join(root, "before.json")
	afterPath := filepath.Join(root, "after.json")
	writeSornaRunFile(t, beforePath, sornaRunFixture("run-before", 1, "pass", "all rules passed", "killed", "completed", 0))
	writeSornaRunFile(t, afterPath, sornaRunFixture("run-after", 1, "fail", "one rule failed", "survived", "failed", 1))
	report, err := CompareSornaRunFiles(beforePath, afterPath)
	if err != nil {
		t.Fatal(err)
	}

	filtered := FilterSornaRunChanges(report, []string{"rules.document.create.accepted.status"})
	if len(filtered.Changes) != 1 || filtered.Changes[0].StableID() != "rules.document.create.accepted.status" || filtered.ChangeSummary.Total != 1 {
		t.Fatalf("filtered Sorna changes = %+v, summary=%+v, want one rule status change", filtered.Changes, filtered.ChangeSummary)
	}
}

func TestLoadSornaRunRejectsInvalidSchemaAndDuplicateRules(t *testing.T) {
	root := t.TempDir()
	wrongSchema := filepath.Join(root, "wrong-schema.json")
	writeSornaRunFile(t, wrongSchema, `{"schema":"ingen.not-a-run/v1"}`)
	if _, err := CompareSornaRunFiles(wrongSchema, wrongSchema); err == nil || !strings.Contains(err.Error(), "schema must be ingen.run/v1") {
		t.Fatalf("wrong schema error = %v, want Sorna schema validation", err)
	}

	duplicatePath := filepath.Join(root, "duplicate.json")
	duplicate := sornaRunFixture("run-duplicate", 1, "pass", "all rules passed", "killed", "completed", 0)
	duplicate = strings.Replace(duplicate, `{"rule_id":"document.process.accepted"`, `{"rule_id":"document.create.accepted"`, 1)
	writeSornaRunFile(t, duplicatePath, duplicate)
	if _, err := CompareSornaRunFiles(duplicatePath, duplicatePath); err == nil || !strings.Contains(err.Error(), `duplicates rule "document.create.accepted"`) {
		t.Fatalf("duplicate rule error = %v, want duplicate rule validation", err)
	}
}

func hasChangeID(changes []Change, id string) bool {
	for _, change := range changes {
		if change.StableID() == id {
			return true
		}
	}
	return false
}

func writeSornaRunFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sornaRunFixture(runID string, contractVersion int, verdict, reason, mutationOutcome, lifecycleOutcome string, lifecycleExitCode int) string {
	const contractSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const oracleSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	const observationSHA = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	passed, failed := 2, 0
	alphaReason := ""
	if verdict == "fail" {
		passed, failed = 1, 1
		alphaReason = "validation failed"
	}
	return fmt.Sprintf(`{
  "schema":"ingen.run/v1",
  "run_id":%q,
  "created_at":"2026-09-17T12:00:00Z",
  "assurance":{"level":1,"status":"observed","observation_coverage":"full","limitations":[]},
  "contract":{"id":"document-pipeline","version":%d,"sha256":"%s"},
  "oracle":{"schema":"ingen.oracle/v1","sha256":"%s"},
  "baseline":{"evidence_path":"evidence/baseline.json","run_id":"baseline-1","contract":{"id":"document-pipeline","version":%d,"sha256":"%s"}},
  "contract_verdict":{"status":%q,"reason":%q},
  "subject":{"base_url":"http://127.0.0.1:8080","adapter":"fixture","variant":"v1"},
  "lifecycle":{"mode":"managed","outcome":%q,"exit_code":%d},
  "summary":{"passed":%d,"failed":%d,"errors":0,"inconclusive":0,"skipped":0},
  "rules":[
    {"rule_id":"document.create.accepted","case_id":"case-0001","subject":"documents","status":%q,"request":{"method":"POST","url":"/documents"},"observation":{"status":201},"observation_sha256":"%s","assertions":[{"path":"response.status","expected":201,"actual":201,"status":"pass"}],"reason":%q},
    {"rule_id":"document.process.accepted","case_id":"case-0002","subject":"documents","status":"pass","request":{"method":"GET","url":"/documents/1"},"observation":{"status":200},"observation_sha256":"%s","assertions":[{"path":"response.status","expected":200,"actual":200,"status":"pass"}]}
  ],
  "mutation":{"spec":{"id":"mut-alpha"},"outcome":%q}
}`, runID, contractVersion, contractSHA, oracleSHA, contractVersion, contractSHA, verdict, reason, lifecycleOutcome, lifecycleExitCode, passed, failed, verdict, observationSHA, alphaReason, observationSHA, mutationOutcome)
}

func TestWriteSornaJSONUsesComparisonSchema(t *testing.T) {
	validSummary := func(runID string) SornaRunSummary {
		return SornaRunSummary{
			Schema:    SornaRunSchema,
			RunID:     runID,
			CreatedAt: "2026-09-17T12:00:00Z",
			Contract:  SornaRunContractSummary{ID: "contract", Version: 1, SHA256: strings.Repeat("a", 64)},
			ContractVerdict: SornaRunVerdictSummary{
				Status: "pass",
				Reason: "ok",
			},
			Subject:   SornaRunSubjectSummary{BaseURL: "http://127.0.0.1:8080", Adapter: "fixture"},
			Assurance: SornaRunAssuranceSummary{Level: 1, Status: "observed", Limitations: []string{}},
			Summary:   SornaRunTotals{},
			Rules: map[string]SornaRuleSummary{
				"rule-a": {RuleID: "rule-a", CaseID: "case-a", Subject: "subject", Status: "pass"},
			},
		}
	}
	report := SornaRunComparison{
		Schema:        sornaRunComparisonSchema,
		Compatible:    true,
		Before:        validSummary("before"),
		After:         validSummary("after"),
		Transition:    NewStateTransition("contract_verdict.status", "pass", "pass", true),
		ChangeSummary: ChangeSummary{},
	}
	var output bytes.Buffer
	if err := WriteSornaJSON(&output, report); err != nil {
		t.Fatal(err)
	}
	var decoded SornaRunComparison
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Schema != sornaRunComparisonSchema || !decoded.Compatible {
		t.Fatalf("decoded report = %+v, want Sorna comparison schema", decoded)
	}
}
