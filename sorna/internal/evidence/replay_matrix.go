package evidence

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"ingen/core/ciresult"
)

const replayMatrixSchema = "sorna.replay-matrix/v1"
const replayMatrixExplanationSchema = "sorna.replay-matrix-explanation/v1"

// ReplayMatrixCase identifies one replay CI result and the classification the
// matrix expects from it. Expected classifications make intentional red
// fixtures reviewable without treating every non-passed input as equivalent.
type ReplayMatrixCase struct {
	ID                  string
	Path                string
	ExpectedCIStatus    string
	ExpectedReplayState string
}

type ReplayMatrixReport struct {
	Schema     string              `json:"schema"`
	Status     string              `json:"status"`
	Total      int                 `json:"total"`
	Matched    int                 `json:"matched"`
	Mismatched int                 `json:"mismatched"`
	Entries    []ReplayMatrixEntry `json:"entries"`
}

type ReplayMatrixEntry struct {
	ID                  string   `json:"id"`
	Path                string   `json:"path"`
	InputSHA256         string   `json:"input_sha256"`
	ExpectedCIStatus    string   `json:"expected_ci_status"`
	ActualCIStatus      string   `json:"actual_ci_status"`
	ExpectedReplayState string   `json:"expected_replay_state"`
	ActualReplayState   string   `json:"actual_replay_state"`
	ContractID          string   `json:"contract_id"`
	RecordedRunID       string   `json:"recorded_run_id"`
	ReplayRunID         string   `json:"replay_run_id"`
	Status              string   `json:"status"`
	Differences         []string `json:"differences,omitempty"`
}

type ReplayMatrixExplanation struct {
	Schema     string                `json:"schema"`
	Status     string                `json:"status"`
	Total      int                   `json:"total"`
	Matched    int                   `json:"matched"`
	Mismatched int                   `json:"mismatched"`
	Failures   []ReplayMatrixFailure `json:"failures,omitempty"`
}

type ReplayMatrixFailure struct {
	ID                  string   `json:"id"`
	ExpectedCIStatus    string   `json:"expected_ci_status"`
	ActualCIStatus      string   `json:"actual_ci_status"`
	ExpectedReplayState string   `json:"expected_replay_state"`
	ActualReplayState   string   `json:"actual_replay_state"`
	Differences         []string `json:"differences,omitempty"`
}

// BuildReplayMatrixCIResult validates and aggregates producer-owned replay CI
// results. A valid member may intentionally be failed or error; the matrix
// passes when each member has the classification declared by its case.
func BuildReplayMatrixCIResult(cases []ReplayMatrixCase, sourceRoot string) (ciresult.Artifact, error) {
	if strings.TrimSpace(sourceRoot) == "" {
		sourceRoot = "."
	}
	if err := validateReplayMatrixCases(cases); err != nil {
		return ciresult.Artifact{}, err
	}
	report := ReplayMatrixReport{
		Schema:  replayMatrixSchema,
		Total:   len(cases),
		Entries: make([]ReplayMatrixEntry, 0, len(cases)),
	}
	inputs := make(map[string]ciresult.FileRef, len(cases))
	for _, item := range cases {
		resolved := resolveReplayPath(item.Path, sourceRoot)
		input, replay, hash, err := loadReplayMatrixInput(resolved)
		if err != nil {
			return ciresult.Artifact{}, fmt.Errorf("load replay matrix case %s: %w", item.ID, err)
		}
		inputs["replay:"+item.ID] = ciresult.FileRef{Path: item.Path, SHA256: hash}
		entry := ReplayMatrixEntry{
			ID:                  item.ID,
			Path:                item.Path,
			InputSHA256:         hash,
			ExpectedCIStatus:    item.ExpectedCIStatus,
			ActualCIStatus:      input.Status,
			ExpectedReplayState: item.ExpectedReplayState,
			ActualReplayState:   replay.Status,
			ContractID:          replay.Contract.ID,
			RecordedRunID:       replay.RecordedRunID,
			ReplayRunID:         replay.ReplayRunID,
			Status:              "matched",
		}
		entry.Differences = replayMatrixDifferences(item.ExpectedCIStatus, input.Status, item.ExpectedReplayState, replay.Status)
		if len(entry.Differences) > 0 {
			entry.Status = "mismatched"
			report.Mismatched++
		} else {
			report.Matched++
		}
		report.Entries = append(report.Entries, entry)
	}
	if report.Mismatched > 0 {
		report.Status = "failed"
	} else {
		report.Status = "passed"
	}
	if err := ValidateReplayMatrixReport(report); err != nil {
		return ciresult.Artifact{}, fmt.Errorf("validate replay matrix report: %w", err)
	}
	reportBytes, err := json.Marshal(report)
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("encode replay matrix report: %w", err)
	}
	explanation := ReplayMatrixExplanation{
		Schema:     replayMatrixExplanationSchema,
		Status:     report.Status,
		Total:      report.Total,
		Matched:    report.Matched,
		Mismatched: report.Mismatched,
	}
	for _, entry := range report.Entries {
		if entry.Status == "mismatched" {
			explanation.Failures = append(explanation.Failures, ReplayMatrixFailure{
				ID:                  entry.ID,
				ExpectedCIStatus:    entry.ExpectedCIStatus,
				ActualCIStatus:      entry.ActualCIStatus,
				ExpectedReplayState: entry.ExpectedReplayState,
				ActualReplayState:   entry.ActualReplayState,
				Differences:         entry.Differences,
			})
		}
	}
	explanationBytes, err := json.Marshal(explanation)
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("encode replay matrix explanation: %w", err)
	}
	status := report.Status
	exitCode, err := ciresult.ExitCodeForStatus(status)
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("map replay matrix status: %w", err)
	}
	artifact := ciresult.Artifact{
		Schema:      ciresult.Schema,
		Tool:        "sorna",
		Kind:        "behavioral-replay-matrix",
		Status:      status,
		ExitCode:    exitCode,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Source:      ciresult.Source{Root: sourceRoot},
		Inputs:      inputs,
		Report:      reportBytes,
		Explanation: explanationBytes,
	}
	if err := artifact.Validate(); err != nil {
		return ciresult.Artifact{}, fmt.Errorf("validate replay matrix CI result: %w", err)
	}
	return artifact, nil
}

func replayMatrixDifferences(expectedCIStatus, actualCIStatus, expectedReplayState, actualReplayState string) []string {
	var differences []string
	if actualCIStatus != expectedCIStatus {
		differences = append(differences, fmt.Sprintf("CI status is %q, want %q", actualCIStatus, expectedCIStatus))
	}
	if actualReplayState != expectedReplayState {
		differences = append(differences, fmt.Sprintf("replay status is %q, want %q", actualReplayState, expectedReplayState))
	}
	return differences
}

// BuildReplayMatrixCIErrorResult preserves hashes for available matrix inputs
// when one member cannot be loaded or classified.
func BuildReplayMatrixCIErrorResult(cases []ReplayMatrixCase, sourceRoot string, cause error) (ciresult.Artifact, error) {
	if cause == nil {
		return ciresult.Artifact{}, fmt.Errorf("replay matrix CI error result requires an error")
	}
	if strings.TrimSpace(sourceRoot) == "" {
		sourceRoot = "."
	}
	inputs := make(map[string]ciresult.FileRef)
	for _, item := range cases {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Path) == "" {
			continue
		}
		if hash, err := hashFile(resolveReplayPath(item.Path, sourceRoot)); err == nil {
			inputs["replay:"+item.ID] = ciresult.FileRef{Path: item.Path, SHA256: hash}
		}
	}
	artifact := ciresult.Artifact{
		Schema:    ciresult.Schema,
		Tool:      "sorna",
		Kind:      "behavioral-replay-matrix",
		Status:    "error",
		ExitCode:  2,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Source:    ciresult.Source{Root: sourceRoot},
		Inputs:    inputs,
		Error:     cause.Error(),
	}
	if err := artifact.Validate(); err != nil {
		return ciresult.Artifact{}, fmt.Errorf("validate replay matrix CI error result: %w", err)
	}
	return artifact, nil
}

func ValidateReplayMatrixReport(report ReplayMatrixReport) error {
	if report.Schema != replayMatrixSchema {
		return fmt.Errorf("replay matrix report schema must be %s", replayMatrixSchema)
	}
	if report.Status != "passed" && report.Status != "failed" {
		return fmt.Errorf("replay matrix report status %q is invalid", report.Status)
	}
	if report.Total != len(report.Entries) || report.Total == 0 {
		return fmt.Errorf("replay matrix report total must equal non-empty entries")
	}
	if report.Matched < 0 || report.Mismatched < 0 || report.Matched+report.Mismatched != report.Total {
		return fmt.Errorf("replay matrix report counts are inconsistent")
	}
	seen := make(map[string]bool, len(report.Entries))
	for index, entry := range report.Entries {
		if strings.TrimSpace(entry.ID) == "" || strings.TrimSpace(entry.Path) == "" || strings.TrimSpace(entry.ContractID) == "" || strings.TrimSpace(entry.RecordedRunID) == "" || strings.TrimSpace(entry.ReplayRunID) == "" || !validReplayDigest(entry.InputSHA256) {
			return fmt.Errorf("replay matrix report entries[%d] identity or input hash is invalid", index)
		}
		if seen[entry.ID] {
			return fmt.Errorf("replay matrix report entries[%d] duplicates %q", index, entry.ID)
		}
		seen[entry.ID] = true
		if !validCIResultStatus(entry.ExpectedCIStatus) || !validCIResultStatus(entry.ActualCIStatus) || !validReplayStatus(entry.ExpectedReplayState) || !validReplayStatus(entry.ActualReplayState) {
			return fmt.Errorf("replay matrix report entries[%d] classification is invalid", index)
		}
		switch entry.Status {
		case "matched":
			if entry.ExpectedCIStatus != entry.ActualCIStatus || entry.ExpectedReplayState != entry.ActualReplayState || len(entry.Differences) > 0 {
				return fmt.Errorf("replay matrix report entries[%d] has an invalid matched entry", index)
			}
		case "mismatched":
			if len(entry.Differences) == 0 {
				return fmt.Errorf("replay matrix report entries[%d] has an invalid mismatched entry", index)
			}
		default:
			return fmt.Errorf("replay matrix report entries[%d] status %q is invalid", index, entry.Status)
		}
	}
	if report.Status == "passed" && report.Mismatched != 0 {
		return fmt.Errorf("passed replay matrix contains mismatched entries")
	}
	if report.Status == "failed" && report.Mismatched == 0 {
		return fmt.Errorf("failed replay matrix contains no mismatched entries")
	}
	return nil
}

func validateReplayMatrixCases(cases []ReplayMatrixCase) error {
	if len(cases) == 0 {
		return fmt.Errorf("replay matrix needs at least one case")
	}
	seen := make(map[string]bool, len(cases))
	for _, item := range cases {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Path) == "" {
			return fmt.Errorf("replay matrix case ID and path are required")
		}
		if seen[item.ID] {
			return fmt.Errorf("replay matrix case %q is duplicated", item.ID)
		}
		seen[item.ID] = true
		if !validCIResultStatus(item.ExpectedCIStatus) {
			return fmt.Errorf("replay matrix case %s has invalid expected CI status %q", item.ID, item.ExpectedCIStatus)
		}
		if !validReplayStatus(item.ExpectedReplayState) {
			return fmt.Errorf("replay matrix case %s has invalid expected replay status %q", item.ID, item.ExpectedReplayState)
		}
	}
	return nil
}

func loadReplayMatrixInput(path string) (ciresult.Artifact, ReplayResult, string, error) {
	input, err := ciresult.LoadFile(path)
	if err != nil {
		return ciresult.Artifact{}, ReplayResult{}, "", err
	}
	if input.Tool != "sorna" || input.Kind != "behavioral-replay" {
		return ciresult.Artifact{}, ReplayResult{}, "", fmt.Errorf("CI input must be a Sorna behavioral-replay result")
	}
	if len(input.Report) == 0 {
		return ciresult.Artifact{}, ReplayResult{}, "", fmt.Errorf("CI input does not contain a replay report")
	}
	replay, err := decodeReplayMatrixReport(input.Report)
	if err != nil {
		return ciresult.Artifact{}, ReplayResult{}, "", fmt.Errorf("parse replay report: %w", err)
	}
	if err := ValidateReplayResult(replay); err != nil {
		return ciresult.Artifact{}, ReplayResult{}, "", fmt.Errorf("validate replay report: %w", err)
	}
	if replayCIStatus(replay.Status) != input.Status {
		return ciresult.Artifact{}, ReplayResult{}, "", fmt.Errorf("CI status %q does not match replay status %q", input.Status, replay.Status)
	}
	hash, err := hashFile(path)
	if err != nil {
		return ciresult.Artifact{}, ReplayResult{}, "", err
	}
	return input, replay, hash, nil
}

func decodeReplayMatrixReport(contents []byte) (ReplayResult, error) {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var replay ReplayResult
	if err := decoder.Decode(&replay); err != nil {
		return ReplayResult{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return ReplayResult{}, fmt.Errorf("multiple JSON values are not supported")
		}
		return ReplayResult{}, err
	}
	return replay, nil
}

func validCIResultStatus(value string) bool {
	switch value {
	case "passed", "failed", "error":
		return true
	default:
		return false
	}
}

func validReplayStatus(value string) bool {
	switch value {
	case "matched", "drifted", "error", "inconclusive":
		return true
	default:
		return false
	}
}
