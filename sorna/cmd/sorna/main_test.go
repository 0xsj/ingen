package main

import (
	"path/filepath"
	"strings"
	"testing"

	"ingen/sorna/internal/campaign"
	"ingen/sorna/internal/lifecycle"
	"ingen/sorna/internal/mutation"
	"ingen/sorna/internal/runner"
)

func TestExitCodeSeparatesContractVerdictFromKilledMutation(t *testing.T) {
	if got := exitCodeFor(runner.RunRecord{Verdict: runner.ContractVerdict{Status: "pass"}}); got != 0 {
		t.Fatalf("clean pass exit code = %d, want 0", got)
	}
	if got := exitCodeFor(runner.RunRecord{Verdict: runner.ContractVerdict{Status: "fail"}}); got != 1 {
		t.Fatalf("unexpected contract failure exit code = %d, want 1", got)
	}
	if got := exitCodeFor(runner.RunRecord{
		Verdict:  runner.ContractVerdict{Status: "fail"},
		Mutation: &mutation.Result{Outcome: "killed"},
	}); got != 0 {
		t.Fatalf("killed mutation exit code = %d, want 0", got)
	}
	if got := exitCodeFor(runner.RunRecord{
		Verdict:  runner.ContractVerdict{Status: "pass"},
		Mutation: &mutation.Result{Outcome: "survived"},
	}); got != 1 {
		t.Fatalf("surviving mutation exit code = %d, want 1", got)
	}
}

func TestLifecycleObservationCoverageNamesSamplingBlindSpots(t *testing.T) {
	if got := lifecycleObservationCoverage(nil); got != "unavailable" {
		t.Fatalf("nil coverage = %q, want unavailable", got)
	}
	clean := &lifecycle.AccessTelemetry{
		Status:                      "captured",
		ExecutableSampleCount:       3,
		ExecutableObservationCount:  1,
		ExecutableObservationErrors: 0,
	}
	if got := lifecycleObservationCoverage(clean); got != "periodic-best-effort" {
		t.Fatalf("clean coverage = %q, want periodic-best-effort", got)
	}
	withGap := *clean
	withGap.ExecutableObservationErrors = 1
	if got := lifecycleObservationCoverage(&withGap); got != "periodic-best-effort-with-gaps" {
		t.Fatalf("gap coverage = %q, want periodic-best-effort-with-gaps", got)
	}
	if got := lifecycleObservationLimitation(&lifecycle.AccessTelemetry{ExecutableSamplingIntervalMS: 25}); got != "executable identity was sampled every 25 ms; transitions between samples may be unobserved" {
		t.Fatalf("sampling limitation = %q, want explicit interval limitation", got)
	}
}

func TestParseReplayMatrixCases(t *testing.T) {
	cases, err := parseReplayMatrixCases([]string{
		"baseline=.artifacts/baseline.json|passed|matched",
		"defect=.artifacts/defect.json|failed|drifted",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 || cases[1].ID != "defect" || cases[1].ExpectedCIStatus != "failed" || cases[1].ExpectedReplayState != "drifted" {
		t.Fatalf("parsed replay matrix cases = %+v, want two classified cases", cases)
	}
	if _, err := parseReplayMatrixCases([]string{"broken-case"}); err == nil || !strings.Contains(err.Error(), "expected id=path") {
		t.Fatalf("parseReplayMatrixCases() = %v, want syntax error", err)
	}
}

func TestVerifyCampaignPlanReferenceRejectsExactPlanDrift(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.json")
	plan := verificationPlan()
	exactHash, err := campaign.WriteFile(path, plan)
	if err != nil {
		t.Fatal(err)
	}
	semanticHash, err := campaign.SemanticHash(plan)
	if err != nil {
		t.Fatal(err)
	}
	reference := campaign.PlanReference{Path: path, SHA256: exactHash, SemanticSHA256: semanticHash}
	if err := verifyCampaignPlanReference(reference, "."); err != nil {
		t.Fatalf("verifyCampaignPlanReference() = %v, want initial plan accepted", err)
	}

	plan.Baseline.RunID = "run-drifted"
	if _, err := campaign.WriteFile(path, plan); err != nil {
		t.Fatal(err)
	}
	if err := verifyCampaignPlanReference(reference, "."); err == nil || !strings.Contains(err.Error(), "exact hash") {
		t.Fatalf("verifyCampaignPlanReference() after exact drift = %v, want exact hash mismatch", err)
	}
}

func TestVerifyCampaignPlanReferenceRejectsSemanticPlanDrift(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.json")
	plan := verificationPlan()
	semanticHash, err := campaign.SemanticHash(plan)
	if err != nil {
		t.Fatal(err)
	}
	plan.Mutations[0].Spec.Description = "tampered description"
	exactHash, err := campaign.WriteFile(path, plan)
	if err != nil {
		t.Fatal(err)
	}
	reference := campaign.PlanReference{Path: path, SHA256: exactHash, SemanticSHA256: semanticHash}
	if err := verifyCampaignPlanReference(reference, "."); err == nil || !strings.Contains(err.Error(), "semantic hash") {
		t.Fatalf("verifyCampaignPlanReference() after semantic drift = %v, want semantic hash mismatch", err)
	}
}

func verificationPlan() campaign.Plan {
	contractHash := strings.Repeat("a", 64)
	oracle := &runner.OracleReference{Schema: "ingen.oracle/v1", SHA256: strings.Repeat("b", 64)}
	return campaign.Plan{
		Schema:    campaign.Schema,
		Status:    "ready",
		Catalogue: campaign.CatalogueReference{Path: "catalogue.yaml", ID: "catalogue", Version: 1, SHA256: strings.Repeat("c", 64)},
		Contract:  runner.ContractReference{ID: "contract", Version: 1, SHA256: contractHash},
		Oracle:    *oracle,
		Baseline: runner.BaselineReference{
			EvidencePath: "baseline",
			RunID:        "run-baseline",
			Contract:     runner.ContractReference{ID: "contract", Version: 1, SHA256: contractHash},
			Oracle:       oracle,
		},
		OraclePolicySHA256:  strings.Repeat("d", 64),
		SubjectPolicySHA256: strings.Repeat("e", 64),
		Mutations: []campaign.MutationEntry{{
			Sequence: 1,
			Spec: mutation.Spec{
				ID:              "m1",
				Plane:           "implementation",
				Operator:        "test.operator",
				Target:          "GET /",
				Description:     "test mutation",
				Change:          map[string]any{"from": 1, "to": 2},
				ExpectedRuleIDs: []string{"rule-1"},
				Status:          "candidate",
			},
		}},
	}
}
