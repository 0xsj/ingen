package evidence

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"ingen/core/ciresult"
)

// VerifyReplayMatrixCIResult verifies a saved replay-matrix envelope and its
// member envelopes. An error envelope is still a valid recorded outcome; in
// that case verification rechecks the hashes of any member inputs that were
// available when the error was produced.
func VerifyReplayMatrixCIResult(path, sourceRoot string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("replay matrix CI result path must not be empty")
	}
	artifact, err := ciresult.LoadFile(path)
	if err != nil {
		return err
	}
	if artifact.Tool != "sorna" || artifact.Kind != "behavioral-replay-matrix" {
		return fmt.Errorf("CI input must be a Sorna behavioral-replay-matrix result")
	}
	if strings.TrimSpace(sourceRoot) == "" {
		sourceRoot = artifact.Source.Root
	}
	if err := verifyReplayMatrixInputHashes(artifact, sourceRoot); err != nil {
		return err
	}
	if artifact.Status == "error" {
		return nil
	}

	report, err := decodeReplayMatrixCIReport(artifact.Report)
	if err != nil {
		return fmt.Errorf("parse replay matrix report: %w", err)
	}
	if err := ValidateReplayMatrixReport(report); err != nil {
		return fmt.Errorf("validate replay matrix report: %w", err)
	}
	if artifact.Status != report.Status {
		return fmt.Errorf("matrix CI status %q does not match report status %q", artifact.Status, report.Status)
	}
	if err := verifyReplayMatrixManifestBinding(artifact, report, sourceRoot); err != nil {
		return err
	}

	explanation, err := decodeReplayMatrixExplanation(artifact.Explanation)
	if err != nil {
		return fmt.Errorf("parse replay matrix explanation: %w", err)
	}
	if err := ValidateReplayMatrixExplanation(explanation, report); err != nil {
		return fmt.Errorf("validate replay matrix explanation: %w", err)
	}
	memberInputs := 0
	for name := range artifact.Inputs {
		if strings.HasPrefix(name, "replay:") {
			memberInputs++
		}
	}
	if memberInputs != len(report.Entries) {
		return fmt.Errorf("matrix CI member input count %d does not match report entries %d", memberInputs, len(report.Entries))
	}

	for _, entry := range report.Entries {
		name := "replay:" + entry.ID
		inputRef, ok := artifact.Inputs[name]
		if !ok {
			return fmt.Errorf("matrix CI input %q is missing", name)
		}
		if inputRef.Path != entry.Path || inputRef.SHA256 != entry.InputSHA256 {
			return fmt.Errorf("matrix CI input %q does not match report entry", name)
		}

		resolved := resolveReplayPath(entry.Path, sourceRoot)
		input, replay, hash, err := loadReplayMatrixInput(resolved)
		if err != nil {
			return fmt.Errorf("verify replay matrix case %s: %w", entry.ID, err)
		}
		if hash != entry.InputSHA256 {
			return fmt.Errorf("replay matrix case %s input hash changed", entry.ID)
		}
		if input.Status != entry.ActualCIStatus || replay.Status != entry.ActualReplayState {
			return fmt.Errorf("replay matrix case %s actual classification changed", entry.ID)
		}
		if replay.Contract.ID != entry.ContractID || replay.RecordedRunID != entry.RecordedRunID || replay.ReplayRunID != entry.ReplayRunID {
			return fmt.Errorf("replay matrix case %s lineage changed", entry.ID)
		}
		expectedDifferences := replayMatrixDifferences(entry.ExpectedCIStatus, input.Status, entry.ExpectedReplayState, replay.Status)
		if entry.Status != matrixEntryStatus(expectedDifferences) || !equalStrings(entry.Differences, expectedDifferences) {
			return fmt.Errorf("replay matrix case %s classification interpretation changed", entry.ID)
		}
	}
	return nil
}

// ValidateReplayMatrixExplanation checks the compact explanation against its
// full matrix report so consumers cannot accept a selectively edited summary.
func ValidateReplayMatrixExplanation(explanation ReplayMatrixExplanation, report ReplayMatrixReport) error {
	if explanation.Schema != replayMatrixExplanationSchema {
		return fmt.Errorf("replay matrix explanation schema must be %s", replayMatrixExplanationSchema)
	}
	if explanation.Status != report.Status || explanation.Total != report.Total || explanation.Matched != report.Matched || explanation.Mismatched != report.Mismatched {
		return fmt.Errorf("replay matrix explanation does not match report summary")
	}
	if len(explanation.Failures) != report.Mismatched {
		return fmt.Errorf("replay matrix explanation failures %d, want %d", len(explanation.Failures), report.Mismatched)
	}
	entries := make(map[string]ReplayMatrixEntry, len(report.Entries))
	for _, entry := range report.Entries {
		if entry.Status == "mismatched" {
			entries[entry.ID] = entry
		}
	}
	seen := make(map[string]bool, len(explanation.Failures))
	for index, failure := range explanation.Failures {
		if strings.TrimSpace(failure.ID) == "" || seen[failure.ID] {
			return fmt.Errorf("replay matrix explanation failures[%d] has a duplicate or empty ID", index)
		}
		seen[failure.ID] = true
		entry, ok := entries[failure.ID]
		if !ok {
			return fmt.Errorf("replay matrix explanation failure %q has no mismatched entry", failure.ID)
		}
		if failure.ExpectedCIStatus != entry.ExpectedCIStatus || failure.ActualCIStatus != entry.ActualCIStatus || failure.ExpectedReplayState != entry.ExpectedReplayState || failure.ActualReplayState != entry.ActualReplayState || !equalStrings(failure.Differences, entry.Differences) {
			return fmt.Errorf("replay matrix explanation failure %q does not match its entry", failure.ID)
		}
	}
	return nil
}

func verifyReplayMatrixInputHashes(artifact ciresult.Artifact, sourceRoot string) error {
	for name, ref := range artifact.Inputs {
		if name != "matrix_manifest" && (!strings.HasPrefix(name, "replay:") || strings.TrimPrefix(name, "replay:") == "") {
			return fmt.Errorf("matrix CI input %q is not a replay member", name)
		}
		if !validReplayDigest(ref.SHA256) {
			return fmt.Errorf("matrix CI input %q has an invalid hash", name)
		}
		resolved := resolveReplayPath(ref.Path, sourceRoot)
		hash, err := hashFile(resolved)
		if err != nil {
			return fmt.Errorf("hash matrix CI input %q: %w", name, err)
		}
		if hash != ref.SHA256 {
			return fmt.Errorf("matrix CI input %q hash changed", name)
		}
	}
	return nil
}

func verifyReplayMatrixManifestBinding(artifact ciresult.Artifact, report ReplayMatrixReport, sourceRoot string) error {
	ref, ok := artifact.Inputs["matrix_manifest"]
	if !ok {
		return nil
	}
	manifest, err := LoadReplayMatrixManifest(replayMatrixManifestPath(ref.Path, sourceRoot))
	if err != nil {
		return fmt.Errorf("load replay matrix manifest: %w", err)
	}
	if len(manifest.Cases) != len(report.Entries) {
		return fmt.Errorf("replay matrix manifest case count %d does not match report entries %d", len(manifest.Cases), len(report.Entries))
	}
	entries := make(map[string]ReplayMatrixEntry, len(report.Entries))
	for _, entry := range report.Entries {
		entries[entry.ID] = entry
	}
	for _, item := range manifest.Cases {
		entry, ok := entries[item.ID]
		if !ok {
			return fmt.Errorf("replay matrix manifest case %q is missing from report", item.ID)
		}
		if item.Path != entry.Path || item.ExpectedCIStatus != entry.ExpectedCIStatus || item.ExpectedReplayState != entry.ExpectedReplayState {
			return fmt.Errorf("replay matrix manifest case %q does not match report expectations", item.ID)
		}
	}
	return nil
}

func decodeReplayMatrixCIReport(contents []byte) (ReplayMatrixReport, error) {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var report ReplayMatrixReport
	if err := decoder.Decode(&report); err != nil {
		return ReplayMatrixReport{}, err
	}
	if err := rejectAdditionalJSON(decoder); err != nil {
		return ReplayMatrixReport{}, err
	}
	return report, nil
}

func decodeReplayMatrixExplanation(contents []byte) (ReplayMatrixExplanation, error) {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var explanation ReplayMatrixExplanation
	if err := decoder.Decode(&explanation); err != nil {
		return ReplayMatrixExplanation{}, err
	}
	if err := rejectAdditionalJSON(decoder); err != nil {
		return ReplayMatrixExplanation{}, err
	}
	return explanation, nil
}

func rejectAdditionalJSON(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not supported")
		}
		return err
	}
	return nil
}

func matrixEntryStatus(differences []string) string {
	if len(differences) == 0 {
		return "matched"
	}
	return "mismatched"
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
