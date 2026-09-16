package evidence

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"ingen/sorna/internal/oracle"
)

// ValidateReplayResult checks the structural and cross-field invariants of a
// replay report. It deliberately validates the report's interpretation, not
// whether the external subject was actually equivalent.
func ValidateReplayResult(result ReplayResult) error {
	if result.Schema != replaySchema {
		return fmt.Errorf("replay report schema must be %s", replaySchema)
	}
	switch result.Status {
	case "matched", "drifted", "error", "inconclusive":
	default:
		return fmt.Errorf("replay report status %q is invalid", result.Status)
	}
	if strings.TrimSpace(result.EvidencePath) == "" || strings.TrimSpace(result.RecordedRunID) == "" || strings.TrimSpace(result.ReplayRunID) == "" {
		return fmt.Errorf("replay report evidence path and run IDs must be non-empty")
	}
	if strings.TrimSpace(result.Contract.ID) == "" || result.Contract.Version < 1 || !validReplayDigest(result.Contract.SHA256) {
		return fmt.Errorf("replay report contract reference is invalid")
	}
	if result.Oracle.Schema != oracle.Schema || !validReplayDigest(result.Oracle.SHA256) {
		return fmt.Errorf("replay report oracle reference is invalid")
	}
	if result.Integrity.Status != "verified" {
		return fmt.Errorf("replay report integrity status must be verified")
	}
	if strings.TrimSpace(result.RecordedSubject.BaseURL) == "" || strings.TrimSpace(result.RecordedSubject.Adapter) == "" || strings.TrimSpace(result.ReplaySubject.BaseURL) == "" || strings.TrimSpace(result.ReplaySubject.Adapter) == "" {
		return fmt.Errorf("replay report subject references are incomplete")
	}
	if !validReplayVerdictStatus(result.RecordedVerdict.Status) || !validReplayVerdictStatus(result.ReplayVerdict.Status) {
		return fmt.Errorf("replay report verdict status is invalid")
	}
	if result.Behavior.Status != result.Status {
		return fmt.Errorf("replay report behavior status %q does not match report status %q", result.Behavior.Status, result.Status)
	}
	if !validObservationStatus(result.Behavior.ObservationStatus) {
		return fmt.Errorf("replay report behavior observation status %q is invalid", result.Behavior.ObservationStatus)
	}
	if !validRequestStatus(result.Behavior.RequestStatus) {
		return fmt.Errorf("replay report behavior request status %q is invalid", result.Behavior.RequestStatus)
	}
	if len(result.Rules) == 0 {
		return fmt.Errorf("replay report rules must not be empty")
	}

	seenCases := make(map[string]bool, len(result.Rules))
	seenRules := make(map[string]bool, len(result.Rules))
	semanticDrift := false
	observationChanges := 0
	requestChanges := 0
	requestUnavailable := 0
	for index, rule := range result.Rules {
		if strings.TrimSpace(rule.CaseID) == "" || strings.TrimSpace(rule.RuleID) == "" {
			return fmt.Errorf("replay report rules[%d] case and rule IDs must be non-empty", index)
		}
		if seenCases[rule.CaseID] {
			return fmt.Errorf("replay report rules[%d].case_id duplicates %q", index, rule.CaseID)
		}
		if seenRules[rule.RuleID] {
			return fmt.Errorf("replay report rules[%d].rule_id duplicates %q", index, rule.RuleID)
		}
		seenCases[rule.CaseID] = true
		seenRules[rule.RuleID] = true
		switch rule.Status {
		case "match":
			if rule.RecordedStatus == "" || rule.ReplayStatus == "" || len(rule.Differences) > 0 {
				return fmt.Errorf("replay report rules[%d] has an invalid match comparison", index)
			}
		case "outcome-drift":
			if rule.RecordedStatus == "" || rule.ReplayStatus == "" || len(rule.Differences) == 0 {
				return fmt.Errorf("replay report rules[%d] has an invalid outcome-drift comparison", index)
			}
			semanticDrift = true
		case "request-drift":
			if rule.RecordedStatus == "" || rule.ReplayStatus == "" || rule.RequestStatus != "changed" || len(rule.Differences) == 0 {
				return fmt.Errorf("replay report rules[%d] has an invalid request-drift comparison", index)
			}
		case "missing":
			if rule.RecordedStatus == "" || rule.ReplayStatus != "" || len(rule.Differences) == 0 {
				return fmt.Errorf("replay report rules[%d] has an invalid missing comparison", index)
			}
			semanticDrift = true
		case "added":
			if rule.RecordedStatus != "" || rule.ReplayStatus == "" || len(rule.Differences) == 0 {
				return fmt.Errorf("replay report rules[%d] has an invalid added comparison", index)
			}
			semanticDrift = true
		default:
			return fmt.Errorf("replay report rules[%d] status %q is invalid", index, rule.Status)
		}
		if !validObservationStatus(rule.ObservationStatus) {
			return fmt.Errorf("replay report rules[%d] observation status %q is invalid", index, rule.ObservationStatus)
		}
		if !validRequestStatus(rule.RequestStatus) {
			return fmt.Errorf("replay report rules[%d] request status %q is invalid", index, rule.RequestStatus)
		}
		switch {
		case rule.RecordedRequestSHA256 == "" || rule.ReplayRequestSHA256 == "":
			if rule.RequestStatus != "unavailable" {
				return fmt.Errorf("replay report rules[%d] must mark a missing request fingerprint unavailable", index)
			}
			requestUnavailable++
		case !validReplayDigest(rule.RecordedRequestSHA256) || !validReplayDigest(rule.ReplayRequestSHA256):
			return fmt.Errorf("replay report rules[%d] request fingerprint is invalid", index)
		case rule.RecordedRequestSHA256 == rule.ReplayRequestSHA256:
			if rule.RequestStatus != "same" {
				return fmt.Errorf("replay report rules[%d] must mark equal request fingerprints same", index)
			}
		default:
			if rule.RequestStatus != "changed" {
				return fmt.Errorf("replay report rules[%d] must mark different request fingerprints changed", index)
			}
			requestChanges++
		}
		switch {
		case rule.RecordedObservationSHA256 == "" || rule.ReplayObservationSHA256 == "":
			if rule.ObservationStatus != "unavailable" {
				return fmt.Errorf("replay report rules[%d] must mark a missing observation hash unavailable", index)
			}
		case !validReplayDigest(rule.RecordedObservationSHA256) || !validReplayDigest(rule.ReplayObservationSHA256):
			return fmt.Errorf("replay report rules[%d] observation hash is invalid", index)
		case rule.RecordedObservationSHA256 == rule.ReplayObservationSHA256:
			if rule.ObservationStatus != "same" {
				return fmt.Errorf("replay report rules[%d] must mark equal observation hashes same", index)
			}
		default:
			if rule.ObservationStatus != "changed" {
				return fmt.Errorf("replay report rules[%d] must mark different observation hashes changed", index)
			}
			observationChanges++
		}
	}
	if result.ObservationChanges != observationChanges {
		return fmt.Errorf("replay report observation_changes is %d, want %d", result.ObservationChanges, observationChanges)
	}
	if result.RequestChanges != requestChanges {
		return fmt.Errorf("replay report request_changes is %d, want %d", result.RequestChanges, requestChanges)
	}
	expectedRequestStatus := "same"
	if requestUnavailable > 0 {
		expectedRequestStatus = "unavailable"
	} else if requestChanges > 0 {
		expectedRequestStatus = "changed"
	}
	if result.Behavior.RequestStatus != expectedRequestStatus {
		return fmt.Errorf("replay report behavior request status %q, want %q", result.Behavior.RequestStatus, expectedRequestStatus)
	}
	expectedObservationStatus := "same"
	if observationChanges > 0 {
		expectedObservationStatus = "changed"
	}
	if result.Status == "error" || result.Status == "inconclusive" {
		expectedObservationStatus = "unavailable"
	}
	if result.Behavior.ObservationStatus != expectedObservationStatus {
		return fmt.Errorf("replay report behavior observation status %q, want %q", result.Behavior.ObservationStatus, expectedObservationStatus)
	}

	switch result.Status {
	case "matched":
		if semanticDrift || result.RecordedVerdict.Status != result.ReplayVerdict.Status || requestChanges > 0 || requestUnavailable > 0 {
			return fmt.Errorf("matched replay contains a contract-visible difference or unverified request intent")
		}
	case "drifted":
		if !semanticDrift && result.RecordedVerdict.Status == result.ReplayVerdict.Status && requestChanges == 0 && requestUnavailable == 0 {
			return fmt.Errorf("drifted replay contains no contract-visible difference")
		}
	case "error":
		if result.ReplayVerdict.Status != "error" {
			return fmt.Errorf("error replay must have an error replay verdict")
		}
	case "inconclusive":
		if result.ReplayVerdict.Status != "inconclusive" {
			return fmt.Errorf("inconclusive replay must have an inconclusive replay verdict")
		}
	}
	return nil
}

// LoadReplayReport loads a saved replay report and validates its JSON shape
// and cross-field semantics. Replay reports are diagnostic artifacts, so the
// loader accepts readable JSON rather than requiring canonical whitespace.
func LoadReplayReport(path string) (ReplayResult, error) {
	if strings.TrimSpace(path) == "" {
		return ReplayResult{}, fmt.Errorf("replay report path must not be empty")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return ReplayResult{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var result ReplayResult
	if err := decoder.Decode(&result); err != nil {
		return ReplayResult{}, fmt.Errorf("parse replay report %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return ReplayResult{}, fmt.Errorf("parse replay report %s: multiple JSON values are not supported", path)
		}
		return ReplayResult{}, fmt.Errorf("parse replay report %s: %w", path, err)
	}
	if err := ValidateReplayResult(result); err != nil {
		return ReplayResult{}, fmt.Errorf("validate replay report %s: %w", path, err)
	}
	return result, nil
}

func validReplayDigest(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validReplayVerdictStatus(value string) bool {
	switch value {
	case "pass", "fail", "error", "inconclusive":
		return true
	default:
		return false
	}
}

func validObservationStatus(value string) bool {
	switch value {
	case "same", "changed", "unavailable":
		return true
	default:
		return false
	}
}

func validRequestStatus(value string) bool {
	switch value {
	case "same", "changed", "unavailable":
		return true
	default:
		return false
	}
}
