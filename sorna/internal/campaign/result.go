package campaign

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ResultSchema is the versioned aggregate result produced after a campaign
// has attempted every planned mutation.
const ResultSchema = "ingen.mutation-campaign-result/v1"

// PlanReference binds a campaign result to the exact plan bytes it consumed
// and may expose the stable semantic identity of that plan.
type PlanReference struct {
	Path           string `json:"path"`
	SHA256         string `json:"sha256"`
	SemanticSHA256 string `json:"semantic_sha256,omitempty"`
}

// EvidenceReference binds an aggregate entry to the two files that identify
// and integrity-check its verified evidence bundle.
type EvidenceReference struct {
	ManifestSHA256  string `json:"manifest_sha256"`
	ChecksumsSHA256 string `json:"checksums_sha256"`
}

// Diagnosis preserves the rule-level reason behind a mutation outcome. The
// campaign result keeps this compact summary while the evidence bundle keeps
// the complete rule observations.
type Diagnosis struct {
	ExpectedRuleStatus    map[string]string `json:"expected_rule_status,omitempty"`
	DirectlyFailedRules   []string          `json:"directly_failed_rules,omitempty"`
	CascadingInconclusive []string          `json:"cascading_inconclusive,omitempty"`
	UnaffectedRules       []string          `json:"unaffected_rules,omitempty"`
	UnobservedExpected    []string          `json:"unobserved_expected,omitempty"`
}

// EntryResult records one provider/run handoff.
type EntryResult struct {
	Sequence     int                `json:"sequence"`
	MutationID   string             `json:"mutation_id"`
	EvidencePath string             `json:"evidence_path"`
	Evidence     *EvidenceReference `json:"evidence,omitempty"`
	RunID        string             `json:"run_id,omitempty"`
	Status       string             `json:"status"`
	Outcome      string             `json:"outcome,omitempty"`
	ExitCode     int                `json:"exit_code"`
	Reason       string             `json:"reason,omitempty"`
	Diagnosis    *Diagnosis         `json:"diagnosis,omitempty"`
}

// Summary makes the campaign denominator explicit.
type Summary struct {
	Total        int `json:"total"`
	Killed       int `json:"killed"`
	Survived     int `json:"survived"`
	Inconclusive int `json:"inconclusive"`
	Other        int `json:"other"`
	Errors       int `json:"errors"`
}

// Result is the campaign-level record. Individual Sorna evidence bundles
// remain authoritative for rule observations; this file aggregates outcomes
// and binds each completed entry to its verified evidence files.
type Result struct {
	Schema     string        `json:"schema"`
	Status     string        `json:"status"`
	Plan       PlanReference `json:"plan"`
	StartedAt  time.Time     `json:"started_at"`
	FinishedAt time.Time     `json:"finished_at"`
	Summary    Summary       `json:"summary"`
	Entries    []EntryResult `json:"entries"`
}

// Validate returns structural errors in a campaign result.
func ValidateResult(result Result) []string {
	problems := make([]string, 0)
	if result.Schema != ResultSchema {
		problems = append(problems, fmt.Sprintf("campaign_result.schema must be %s", ResultSchema))
	}
	if result.Status != "passed" && result.Status != "failed" && result.Status != "error" {
		problems = append(problems, "campaign_result.status must be passed, failed, or error")
	}
	if strings.TrimSpace(result.Plan.Path) == "" {
		problems = append(problems, "campaign_result.plan.path must be non-empty")
	}
	if !digestPattern.MatchString(result.Plan.SHA256) {
		problems = append(problems, "campaign_result.plan.sha256 must be a lowercase SHA-256 digest")
	}
	if strings.TrimSpace(result.Plan.SemanticSHA256) != "" && !digestPattern.MatchString(result.Plan.SemanticSHA256) {
		problems = append(problems, "campaign_result.plan.semantic_sha256 must be a lowercase SHA-256 digest when present")
	}
	if result.StartedAt.IsZero() || result.FinishedAt.IsZero() || result.FinishedAt.Before(result.StartedAt) {
		problems = append(problems, "campaign_result timestamps must be ordered and non-zero")
	}
	if len(result.Entries) == 0 {
		problems = append(problems, "campaign_result.entries must contain at least one entry")
	}
	if result.Summary.Total != len(result.Entries) {
		problems = append(problems, "campaign_result.summary.total must equal the entry count")
	}
	seen := make(map[string]bool, len(result.Entries))
	expectedSummary := Summary{Total: len(result.Entries)}
	expectedStatus := "passed"
	for index, entry := range result.Entries {
		path := fmt.Sprintf("campaign_result.entries[%d]", index)
		if entry.Sequence != index+1 {
			problems = append(problems, fmt.Sprintf("%s.sequence must be %d", path, index+1))
		}
		if strings.TrimSpace(entry.MutationID) == "" {
			problems = append(problems, path+".mutation_id must be non-empty")
		} else if seen[entry.MutationID] {
			problems = append(problems, fmt.Sprintf("%s.mutation_id duplicates %q", path, entry.MutationID))
		} else {
			seen[entry.MutationID] = true
		}
		if strings.TrimSpace(entry.EvidencePath) == "" {
			problems = append(problems, path+".evidence_path must be non-empty")
		}
		if entry.Evidence != nil {
			if !digestPattern.MatchString(entry.Evidence.ManifestSHA256) {
				problems = append(problems, path+".evidence.manifest_sha256 must be a lowercase SHA-256 digest")
			}
			if !digestPattern.MatchString(entry.Evidence.ChecksumsSHA256) {
				problems = append(problems, path+".evidence.checksums_sha256 must be a lowercase SHA-256 digest")
			}
		}
		if entry.Status != "passed" && entry.Status != "failed" && entry.Status != "error" {
			problems = append(problems, path+".status must be passed, failed, or error")
		}
		if entry.Status != "error" && entry.Evidence == nil {
			problems = append(problems, path+".evidence is required for a completed entry")
		}
		if entry.Status == "error" && strings.TrimSpace(entry.Reason) == "" {
			problems = append(problems, path+".reason is required for an error")
		}
		if entry.Status != "error" && entry.ExitCode < 0 {
			problems = append(problems, path+".exit_code must not be negative for a completed entry")
		}
		if entry.Status == "passed" && entry.Outcome != "killed" {
			problems = append(problems, path+".outcome must be killed for a passed entry")
		}
		if entry.Status == "failed" && strings.TrimSpace(entry.Outcome) == "" {
			problems = append(problems, path+".outcome is required for a failed entry")
		}
		if entry.Outcome == "killed" || entry.Outcome == "survived" || entry.Outcome == "inconclusive" {
			if entry.Diagnosis == nil {
				problems = append(problems, path+".diagnosis is required for a classified outcome")
			} else {
				problems = append(problems, validateDiagnosis(path+".diagnosis", entry.Outcome, *entry.Diagnosis)...)
			}
		} else if entry.Diagnosis != nil {
			problems = append(problems, validateDiagnosis(path+".diagnosis", entry.Outcome, *entry.Diagnosis)...)
		}
		switch entry.Status {
		case "error":
			expectedSummary.Errors++
			expectedStatus = "error"
		case "failed":
			if expectedStatus == "passed" {
				expectedStatus = "failed"
			}
		}
		switch entry.Outcome {
		case "killed":
			expectedSummary.Killed++
		case "survived":
			expectedSummary.Survived++
		case "inconclusive":
			expectedSummary.Inconclusive++
		case "":
		default:
			expectedSummary.Other++
		}
	}
	if result.Summary != expectedSummary {
		problems = append(problems, "campaign_result.summary counters must match entry outcomes")
	}
	if result.Status != expectedStatus {
		problems = append(problems, "campaign_result.status must match entry statuses")
	}
	return problems
}

func validateDiagnosis(path, outcome string, diagnosis Diagnosis) []string {
	problems := make([]string, 0)
	if len(diagnosis.ExpectedRuleStatus) == 0 {
		problems = append(problems, path+".expected_rule_status must contain at least one rule")
	}
	for ruleID, status := range diagnosis.ExpectedRuleStatus {
		if strings.TrimSpace(ruleID) == "" {
			problems = append(problems, path+".expected_rule_status contains an empty rule ID")
		}
		switch status {
		case "pass", "fail", "inconclusive", "error", "skipped", "unobserved":
		default:
			problems = append(problems, fmt.Sprintf("%s.expected_rule_status[%q] has unsupported status %q", path, ruleID, status))
		}
	}
	for field, values := range map[string][]string{
		"directly_failed_rules":  diagnosis.DirectlyFailedRules,
		"cascading_inconclusive": diagnosis.CascadingInconclusive,
		"unaffected_rules":       diagnosis.UnaffectedRules,
		"unobserved_expected":    diagnosis.UnobservedExpected,
	} {
		seen := make(map[string]bool, len(values))
		for index, ruleID := range values {
			if strings.TrimSpace(ruleID) == "" {
				problems = append(problems, fmt.Sprintf("%s.%s[%d] must be non-empty", path, field, index))
			}
			if seen[ruleID] {
				problems = append(problems, fmt.Sprintf("%s.%s duplicates %q", path, field, ruleID))
			}
			seen[ruleID] = true
		}
	}
	if len(problems) > 0 || outcome == "" {
		return problems
	}
	hasFailedTarget := false
	hasUnresolvedTarget := false
	allTargetsPassed := true
	for _, status := range diagnosis.ExpectedRuleStatus {
		switch status {
		case "fail":
			hasFailedTarget = true
			allTargetsPassed = false
		case "pass":
		case "inconclusive", "error", "skipped", "unobserved":
			hasUnresolvedTarget = true
			allTargetsPassed = false
		}
	}
	switch outcome {
	case "killed":
		if !hasFailedTarget {
			problems = append(problems, path+" must identify a failed expected rule for a killed outcome")
		}
	case "survived":
		if !allTargetsPassed {
			problems = append(problems, path+" must show every expected rule passing for a survived outcome")
		}
	case "inconclusive":
		if hasFailedTarget {
			problems = append(problems, path+" must not identify a failed expected rule for an inconclusive outcome")
		}
		if !hasUnresolvedTarget {
			problems = append(problems, path+" must identify an unresolved expected rule for an inconclusive outcome")
		}
	}
	return problems
}

// HashEvidence returns the exact hashes of the manifest and checksum files in
// an evidence bundle. The caller should run evidence verification separately;
// this function only records the bytes that the aggregate result references.
func HashEvidence(outputDir string) (EvidenceReference, error) {
	if strings.TrimSpace(outputDir) == "" {
		return EvidenceReference{}, fmt.Errorf("evidence output directory must not be empty")
	}
	manifestHash, err := HashFile(filepath.Join(outputDir, "manifest.json"))
	if err != nil {
		return EvidenceReference{}, fmt.Errorf("hash evidence manifest: %w", err)
	}
	checksumsHash, err := HashFile(filepath.Join(outputDir, "checksums.sha256"))
	if err != nil {
		return EvidenceReference{}, fmt.Errorf("hash evidence checksums: %w", err)
	}
	return EvidenceReference{ManifestSHA256: manifestHash, ChecksumsSHA256: checksumsHash}, nil
}

// WriteResult writes a canonical campaign result and returns its hash.
func WriteResult(path string, result Result) (string, error) {
	contents, err := CanonicalResultJSON(result)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		return "", err
	}
	return HashBytes(contents), nil
}

// LoadResult loads and verifies canonical campaign result JSON.
func LoadResult(path string) (Result, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Result{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	var result Result
	if err := decoder.Decode(&result); err != nil {
		return Result{}, fmt.Errorf("parse campaign result %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Result{}, fmt.Errorf("parse campaign result %s: multiple JSON values are not supported", path)
		}
		return Result{}, fmt.Errorf("parse campaign result %s: %w", path, err)
	}
	if problems := ValidateResult(result); len(problems) > 0 {
		return Result{}, fmt.Errorf("invalid campaign result: %s", strings.Join(problems, "; "))
	}
	canonical, err := CanonicalResultJSON(result)
	if err != nil {
		return Result{}, err
	}
	if !bytes.Equal(contents, canonical) {
		return Result{}, fmt.Errorf("campaign result %s is not canonical JSON", path)
	}
	return result, nil
}

// CanonicalResultJSON returns deterministic JSON bytes for a valid result.
func CanonicalResultJSON(result Result) ([]byte, error) {
	if problems := ValidateResult(result); len(problems) > 0 {
		return nil, fmt.Errorf("invalid campaign result: %s", strings.Join(problems, "; "))
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(result); err != nil {
		return nil, fmt.Errorf("canonicalize campaign result: %w", err)
	}
	return buffer.Bytes(), nil
}
