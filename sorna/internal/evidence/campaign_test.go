package evidence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ingen/sorna/internal/campaign"
)

func TestBuildMutationCampaignCIResultPreservesResultAndInputs(t *testing.T) {
	directory := t.TempDir()
	resultPath := filepath.Join(directory, "campaign-result.json")
	result := campaign.Result{
		Schema:     campaign.ResultSchema,
		Status:     "passed",
		Plan:       campaign.PlanReference{Path: "plan.json", SHA256: strings.Repeat("a", 64)},
		StartedAt:  time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC),
		FinishedAt: time.Date(2026, 9, 15, 10, 0, 1, 0, time.UTC),
		Summary:    campaign.Summary{Total: 1, Killed: 1},
		Entries: []campaign.EntryResult{{
			Sequence: 1, MutationID: "status-200-create", EvidencePath: "evidence", Evidence: &campaign.EvidenceReference{ManifestSHA256: strings.Repeat("c", 64), ChecksumsSHA256: strings.Repeat("d", 64)}, Status: "passed", Outcome: "killed", ExitCode: 0,
			Diagnosis: &campaign.Diagnosis{ExpectedRuleStatus: map[string]string{"document.create.valid.accepted": "fail"}, DirectlyFailedRules: []string{"document.create.valid.accepted"}},
		}},
	}
	if _, err := campaign.WriteResult(resultPath, result); err != nil {
		t.Fatal(err)
	}

	artifact, err := BuildMutationCampaignCIResult(result, resultPath, ".")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Tool != "sorna" || artifact.Kind != "mutation-campaign" || artifact.Status != "passed" || artifact.ExitCode != 0 {
		t.Fatalf("artifact = %+v, want passing mutation campaign envelope", artifact)
	}
	if artifact.Inputs["plan"].SHA256 != result.Plan.SHA256 || artifact.Inputs["campaign_result"].Path != resultPath {
		t.Fatalf("artifact inputs = %+v, want plan and campaign result references", artifact.Inputs)
	}
	var decoded campaign.Result
	if err := json.Unmarshal(artifact.Report, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Status != "passed" || decoded.Summary.Killed != 1 {
		t.Fatalf("decoded report = %+v, want original campaign result", decoded)
	}
}

func TestBuildMutationCampaignCIResultExplainsSurvivors(t *testing.T) {
	resultPath := filepath.Join(t.TempDir(), "campaign-result.json")
	result := campaign.Result{
		Schema:     campaign.ResultSchema,
		Status:     "failed",
		Plan:       campaign.PlanReference{Path: "plan.json", SHA256: strings.Repeat("b", 64)},
		StartedAt:  time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC),
		FinishedAt: time.Date(2026, 9, 15, 10, 0, 1, 0, time.UTC),
		Summary:    campaign.Summary{Total: 1, Survived: 1},
		Entries: []campaign.EntryResult{{
			Sequence: 1, MutationID: "remove-name-create", EvidencePath: "evidence", Evidence: &campaign.EvidenceReference{ManifestSHA256: strings.Repeat("e", 64), ChecksumsSHA256: strings.Repeat("f", 64)}, Status: "failed", Outcome: "survived", ExitCode: 1, Reason: "contract did not detect the mutation",
			Diagnosis: &campaign.Diagnosis{ExpectedRuleStatus: map[string]string{"document.create.valid.accepted": "pass"}, UnaffectedRules: []string{"document.create.valid.accepted"}},
		}},
	}
	if _, err := campaign.WriteResult(resultPath, result); err != nil {
		t.Fatal(err)
	}

	artifact, err := BuildMutationCampaignCIResult(result, resultPath, ".")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Status != "failed" || artifact.ExitCode != 1 {
		t.Fatalf("artifact = %+v, want failed mutation campaign envelope", artifact)
	}
	var explanation MutationCampaignExplanation
	if err := json.Unmarshal(artifact.Explanation, &explanation); err != nil {
		t.Fatal(err)
	}
	if explanation.Schema != mutationCampaignExplanationSchema || len(explanation.Failures) != 1 || explanation.Failures[0].Outcome != "survived" {
		t.Fatalf("explanation = %+v, want survivor details", explanation)
	}
	if got := explanation.Failures[0].Category; got != mutationFailureCategoryContractInsensitive {
		t.Fatalf("failure category = %q, want %q", got, mutationFailureCategoryContractInsensitive)
	}
	if got := explanation.FailureCategories[mutationFailureCategoryContractInsensitive]; got != 1 {
		t.Fatalf("failure categories = %+v, want one contract-insensitive failure", explanation.FailureCategories)
	}
	if got := explanation.Failures[0].Diagnosis.ExpectedRuleStatus["document.create.valid.accepted"]; got != "pass" {
		t.Fatalf("survivor diagnosis = %+v, want passing target status", explanation.Failures[0].Diagnosis)
	}
}

func TestBuildMutationCampaignCIResultClassifiesObservationAndExecutionFailures(t *testing.T) {
	resultPath := filepath.Join(t.TempDir(), "campaign-result.json")
	result := campaign.Result{
		Schema:     campaign.ResultSchema,
		Status:     "error",
		Plan:       campaign.PlanReference{Path: "plan.json", SHA256: strings.Repeat("b", 64)},
		StartedAt:  time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC),
		FinishedAt: time.Date(2026, 9, 15, 10, 0, 1, 0, time.UTC),
		Summary:    campaign.Summary{Total: 2, Inconclusive: 1, Errors: 1},
		Entries: []campaign.EntryResult{
			{Sequence: 1, MutationID: "inconclusive", EvidencePath: "evidence-1", Evidence: &campaign.EvidenceReference{ManifestSHA256: strings.Repeat("e", 64), ChecksumsSHA256: strings.Repeat("f", 64)}, Status: "failed", Outcome: "inconclusive", ExitCode: 1, Diagnosis: &campaign.Diagnosis{ExpectedRuleStatus: map[string]string{"target": "inconclusive"}}},
			{Sequence: 2, MutationID: "provider-error", EvidencePath: "evidence-2", Status: "error", ExitCode: 2, Reason: "provider failed"},
		},
	}
	if _, err := campaign.WriteResult(resultPath, result); err != nil {
		t.Fatal(err)
	}

	artifact, err := BuildMutationCampaignCIResult(result, resultPath, ".")
	if err != nil {
		t.Fatal(err)
	}
	var explanation MutationCampaignExplanation
	if err := json.Unmarshal(artifact.Explanation, &explanation); err != nil {
		t.Fatal(err)
	}
	if got := explanation.FailureCategories[mutationFailureCategoryInsufficientObservation]; got != 1 {
		t.Fatalf("failure categories = %+v, want one insufficient-observation failure", explanation.FailureCategories)
	}
	if got := explanation.FailureCategories[mutationFailureCategoryExecutionError]; got != 1 {
		t.Fatalf("failure categories = %+v, want one execution-error failure", explanation.FailureCategories)
	}
}

func TestBuildMutationCampaignCIErrorResultIsCollectorFriendly(t *testing.T) {
	resultPath := filepath.Join(t.TempDir(), "campaign-result.json")
	if err := os.WriteFile(resultPath, []byte("not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	artifact, err := BuildMutationCampaignCIErrorResult(resultPath, ".", os.ErrInvalid)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Status != "error" || artifact.ExitCode != 2 || artifact.Error == "" {
		t.Fatalf("artifact = %+v, want error envelope", artifact)
	}
	if artifact.Inputs["campaign_result"].Path != resultPath {
		t.Fatalf("artifact inputs = %+v, want campaign result reference", artifact.Inputs)
	}
}
