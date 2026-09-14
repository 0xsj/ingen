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

// PlanReference binds a campaign result to the exact plan bytes it consumed.
type PlanReference struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// EvidenceReference binds an aggregate entry to the two files that identify
// and integrity-check its verified evidence bundle.
type EvidenceReference struct {
	ManifestSHA256  string `json:"manifest_sha256"`
	ChecksumsSHA256 string `json:"checksums_sha256"`
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
