package campaign

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteAndLoadResultPreservesCampaignDenominator(t *testing.T) {
	started := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
	result := Result{
		Schema: ResultSchema,
		Status: "passed",
		Plan: PlanReference{
			Path:   ".artifacts/document-pipeline-mutation-plan.json",
			SHA256: strings.Repeat("a", 64),
		},
		StartedAt:  started,
		FinishedAt: started.Add(time.Second),
		Summary:    Summary{Total: 1, Killed: 1},
		Entries: []EntryResult{{
			Sequence:     1,
			MutationID:   "status-200-create",
			EvidencePath: ".artifacts/document-pipeline-campaign/001-status-200-create",
			Evidence:     &EvidenceReference{ManifestSHA256: strings.Repeat("b", 64), ChecksumsSHA256: strings.Repeat("c", 64)},
			RunID:        "run-mutated",
			Status:       "passed",
			Outcome:      "killed",
			ExitCode:     0,
			Diagnosis:    &Diagnosis{ExpectedRuleStatus: map[string]string{"document.create.valid.accepted": "fail"}, DirectlyFailedRules: []string{"document.create.valid.accepted"}},
		}},
	}
	path := filepath.Join(t.TempDir(), "result.json")
	if _, err := WriteResult(path, result); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadResult(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != "passed" || loaded.Summary.Killed != 1 || loaded.Entries[0].Outcome != "killed" || loaded.Entries[0].Diagnosis.ExpectedRuleStatus["document.create.valid.accepted"] != "fail" {
		t.Fatalf("loaded result = %+v, want preserved campaign result", loaded)
	}
}

func TestLoadResultRejectsUnknownFields(t *testing.T) {
	started := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
	result := Result{
		Schema:     ResultSchema,
		Status:     "passed",
		Plan:       PlanReference{Path: "plan.json", SHA256: strings.Repeat("a", 64)},
		StartedAt:  started,
		FinishedAt: started.Add(time.Second),
		Summary:    Summary{Total: 1, Killed: 1},
		Entries: []EntryResult{{
			Sequence: 1, MutationID: "m1", EvidencePath: "evidence/m1",
			Evidence: &EvidenceReference{ManifestSHA256: strings.Repeat("b", 64), ChecksumsSHA256: strings.Repeat("c", 64)},
			Status:   "passed", Outcome: "killed", ExitCode: 0,
			Diagnosis: &Diagnosis{ExpectedRuleStatus: map[string]string{"target": "fail"}},
		}},
	}
	path := filepath.Join(t.TempDir(), "result.json")
	if _, err := WriteResult(path, result); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.TrimSpace(string(contents))
	text = strings.TrimSuffix(text, "}") + ",\"unexpected\":true}"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadResult(path); err == nil || !strings.Contains(err.Error(), `unknown field "unexpected"`) {
		t.Fatalf("LoadResult() = %v, want unknown-field error", err)
	}
}

func TestHashEvidenceBindsManifestAndChecksums(t *testing.T) {
	directory := t.TempDir()
	manifest := []byte("manifest\n")
	checksums := []byte("checksums\n")
	if err := os.WriteFile(filepath.Join(directory, "manifest.json"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "checksums.sha256"), checksums, 0o644); err != nil {
		t.Fatal(err)
	}
	reference, err := HashEvidence(directory)
	if err != nil {
		t.Fatal(err)
	}
	manifestHash, err := HashFile(filepath.Join(directory, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	checksumsHash, err := HashFile(filepath.Join(directory, "checksums.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	if reference.ManifestSHA256 != manifestHash || reference.ChecksumsSHA256 != checksumsHash {
		t.Fatalf("evidence reference = %+v, want manifest/checksum hashes", reference)
	}
}

func TestValidateResultRequiresEvidenceBindingForCompletedEntry(t *testing.T) {
	result := Result{
		Schema:     ResultSchema,
		Status:     "passed",
		Plan:       PlanReference{Path: "plan.json", SHA256: strings.Repeat("a", 64)},
		StartedAt:  startedTime,
		FinishedAt: startedTime,
		Summary:    Summary{Total: 1, Killed: 1},
		Entries: []EntryResult{{
			Sequence:     1,
			MutationID:   "m1",
			EvidencePath: "run",
			Status:       "passed",
			Outcome:      "killed",
			ExitCode:     0,
			Diagnosis:    &Diagnosis{ExpectedRuleStatus: map[string]string{"target": "fail"}},
		}},
	}
	problems := ValidateResult(result)
	if len(problems) != 1 || !strings.Contains(problems[0], "evidence is required") {
		t.Fatalf("problems = %v, want missing evidence binding", problems)
	}
}

func TestValidateResultRequiresActionableDiagnosis(t *testing.T) {
	result := Result{
		Schema:     ResultSchema,
		Status:     "failed",
		Plan:       PlanReference{Path: "plan.json", SHA256: strings.Repeat("a", 64)},
		StartedAt:  startedTime,
		FinishedAt: startedTime,
		Summary:    Summary{Total: 1, Survived: 1},
		Entries: []EntryResult{{
			Sequence: 1, MutationID: "m1", EvidencePath: "run",
			Evidence: &EvidenceReference{ManifestSHA256: strings.Repeat("b", 64), ChecksumsSHA256: strings.Repeat("c", 64)},
			Status:   "failed", Outcome: "survived", ExitCode: 1,
			Diagnosis: &Diagnosis{ExpectedRuleStatus: map[string]string{"target": "inconclusive"}},
		}},
	}
	problems := ValidateResult(result)
	if len(problems) != 1 || !strings.Contains(problems[0], "every expected rule passing") {
		t.Fatalf("problems = %v, want inconsistent survivor diagnosis", problems)
	}
}

func TestValidateResultRequiresExplicitErrorReason(t *testing.T) {
	result := Result{
		Schema:     ResultSchema,
		Status:     "error",
		Plan:       PlanReference{Path: "plan.json", SHA256: strings.Repeat("a", 64)},
		StartedAt:  startedTime,
		FinishedAt: startedTime,
		Summary:    Summary{Total: 1, Errors: 1},
		Entries: []EntryResult{{
			Sequence:     1,
			MutationID:   "m1",
			EvidencePath: "run",
			Status:       "error",
			ExitCode:     -1,
		}},
	}
	problems := ValidateResult(result)
	if len(problems) != 1 || !strings.Contains(problems[0], "reason") {
		t.Fatalf("problems = %v, want missing error reason", problems)
	}
}

var startedTime = time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
