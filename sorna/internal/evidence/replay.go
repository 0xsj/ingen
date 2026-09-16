package evidence

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"ingen/sorna/internal/oracle"
	"ingen/sorna/internal/runner"
)

const replaySchema = "sorna.replay/v1"

// ReplayResult records the difference between a verified stored run and a new
// execution of the same frozen oracle. The original evidence is never
// rewritten by Replay.
type ReplayResult struct {
	Schema             string                   `json:"schema"`
	Status             string                   `json:"status"`
	EvidencePath       string                   `json:"evidence_path"`
	RecordedRunID      string                   `json:"recorded_run_id"`
	ReplayRunID        string                   `json:"replay_run_id"`
	Contract           runner.ContractReference `json:"contract"`
	Oracle             runner.OracleReference   `json:"oracle"`
	Integrity          ReplayIntegrity          `json:"integrity"`
	Behavior           ReplayBehavior           `json:"behavior"`
	RecordedSubject    runner.SubjectReference  `json:"recorded_subject"`
	ReplaySubject      runner.SubjectReference  `json:"replay_subject"`
	RecordedVerdict    runner.ContractVerdict   `json:"recorded_verdict"`
	ReplayVerdict      runner.ContractVerdict   `json:"replay_verdict"`
	Rules              []ReplayRuleResult       `json:"rules"`
	Differences        []string                 `json:"differences,omitempty"`
	ObservationChanges int                      `json:"observation_changes"`
}

type ReplayIntegrity struct {
	Status string `json:"status"`
}

type ReplayBehavior struct {
	Status            string `json:"status"`
	ObservationStatus string `json:"observation_status"`
}

// ReplayRuleResult compares contract-visible rule outcomes and reports
// observation changes separately. Observation hashes are diagnostic; a
// changed observation does not imply a changed contract outcome by itself.
type ReplayRuleResult struct {
	RuleID                    string   `json:"rule_id"`
	CaseID                    string   `json:"case_id"`
	Status                    string   `json:"status"`
	RecordedStatus            string   `json:"recorded_status,omitempty"`
	ReplayStatus              string   `json:"replay_status,omitempty"`
	RecordedObservationSHA256 string   `json:"recorded_observation_sha256,omitempty"`
	ReplayObservationSHA256   string   `json:"replay_observation_sha256,omitempty"`
	ObservationStatus         string   `json:"observation_status"`
	Differences               []string `json:"differences,omitempty"`
}

// Replay verifies an evidence bundle and re-executes its frozen oracle
// against config.BaseURL. The contract source is intentionally not an input.
// The subject URL must be supplied explicitly so replay never silently sends
// requests to the original subject.
func Replay(ctx context.Context, evidenceDir string, artifact oracle.Artifact, config runner.Config) (ReplayResult, error) {
	if strings.TrimSpace(config.BaseURL) == "" {
		return ReplayResult{}, fmt.Errorf("replay base URL must be supplied explicitly")
	}
	if err := Verify(evidenceDir); err != nil {
		return ReplayResult{}, fmt.Errorf("verify replay evidence: %w", err)
	}
	manifest, recorded, err := loadRun(evidenceDir)
	if err != nil {
		return ReplayResult{}, fmt.Errorf("load replay run: %w", err)
	}
	if recorded.Mutation != nil {
		return ReplayResult{}, fmt.Errorf("replay does not support mutation evidence; replay the unmutated run")
	}
	if recorded.Oracle == nil {
		return ReplayResult{}, fmt.Errorf("recorded run does not contain an oracle reference")
	}

	oracleHash, err := oracle.Hash(artifact)
	if err != nil {
		return ReplayResult{}, fmt.Errorf("hash replay oracle: %w", err)
	}
	replayOracle := runner.OracleReference{Schema: artifact.Schema, SHA256: oracleHash}
	if *recorded.Oracle != replayOracle {
		return ReplayResult{}, fmt.Errorf("replay oracle %s (%s) does not match recorded oracle %s (%s)",
			artifact.Schema, oracleHash, recorded.Oracle.Schema, recorded.Oracle.SHA256)
	}
	replayContract := runner.ContractReference{ID: artifact.Contract.ID, Version: artifact.Contract.Version, SHA256: artifact.Contract.SHA256}
	if recorded.Contract != replayContract {
		return ReplayResult{}, fmt.Errorf("replay oracle contract does not match recorded contract")
	}
	if manifest.Policy != nil && artifact.PolicySHA256 != manifest.Policy.SHA256 {
		return ReplayResult{}, fmt.Errorf("replay oracle policy hash %q does not match recorded policy %q", artifact.PolicySHA256, manifest.Policy.SHA256)
	}

	replayed, err := runner.ExecuteOracle(ctx, artifact, config)
	if err != nil {
		return ReplayResult{}, fmt.Errorf("execute replay oracle: %w", err)
	}
	result := ReplayResult{
		Schema:          replaySchema,
		Status:          "matched",
		EvidencePath:    evidenceDir,
		RecordedRunID:   recorded.RunID,
		ReplayRunID:     replayed.RunID,
		Contract:        recorded.Contract,
		Oracle:          *recorded.Oracle,
		Integrity:       ReplayIntegrity{Status: "verified"},
		Behavior:        ReplayBehavior{Status: "matched", ObservationStatus: "same"},
		RecordedSubject: recorded.Subject,
		ReplaySubject:   replayed.Subject,
		RecordedVerdict: recorded.Verdict,
		ReplayVerdict:   replayed.Verdict,
		Rules:           make([]ReplayRuleResult, 0),
	}
	compareReplay(&result, recorded, replayed)
	return result, nil
}

func compareReplay(result *ReplayResult, recorded, replayed runner.RunRecord) {
	recordedRules := indexReplayRules(recorded.Rules)
	replayedRules := indexReplayRules(replayed.Rules)
	keys := make([]string, 0, len(recordedRules)+len(replayedRules))
	seen := make(map[string]bool, len(recordedRules)+len(replayedRules))
	for key := range recordedRules {
		keys = append(keys, key)
		seen[key] = true
	}
	for key := range replayedRules {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	for _, key := range keys {
		recordedRule, recordedOK := recordedRules[key]
		replayedRule, replayedOK := replayedRules[key]
		comparison := ReplayRuleResult{CaseID: key, Status: "match", ObservationStatus: "unavailable"}
		if recordedOK {
			comparison.RuleID = recordedRule.RuleID
			comparison.RecordedStatus = recordedRule.Status
			comparison.RecordedObservationSHA256 = recordedRule.ObservationSHA256
		}
		if replayedOK {
			if comparison.RuleID == "" {
				comparison.RuleID = replayedRule.RuleID
			}
			comparison.ReplayStatus = replayedRule.Status
			comparison.ReplayObservationSHA256 = replayedRule.ObservationSHA256
		}

		switch {
		case !recordedOK:
			comparison.Status = "added"
			comparison.Differences = []string{"rule is present only in replay"}
		case !replayedOK:
			comparison.Status = "missing"
			comparison.Differences = []string{"rule is present only in recorded evidence"}
		default:
			comparison.Differences = compareRuleOutcome(recordedRule, replayedRule)
			if len(comparison.Differences) > 0 {
				comparison.Status = "outcome-drift"
			}
			if recordedRule.ObservationSHA256 == "" || replayedRule.ObservationSHA256 == "" {
				comparison.ObservationStatus = "unavailable"
			} else if recordedRule.ObservationSHA256 != replayedRule.ObservationSHA256 {
				comparison.ObservationStatus = "changed"
			} else {
				comparison.ObservationStatus = "same"
			}
		}

		if comparison.Status != "match" {
			result.Behavior.Status = "drifted"
			result.Status = "drifted"
			for _, difference := range comparison.Differences {
				result.Differences = append(result.Differences, fmt.Sprintf("case %s: %s", key, difference))
			}
		}
		if comparison.ObservationStatus == "changed" {
			result.ObservationChanges++
			result.Behavior.ObservationStatus = "changed"
			result.Differences = append(result.Differences, fmt.Sprintf("case %s: observation hash changed", key))
		}
		result.Rules = append(result.Rules, comparison)
	}
	if recorded.Verdict.Status != replayed.Verdict.Status {
		result.Status = "drifted"
		result.Behavior.Status = "drifted"
		result.Differences = append(result.Differences, fmt.Sprintf("contract verdict changed from %q to %q", recorded.Verdict.Status, replayed.Verdict.Status))
	}
	if replayed.Summary.Errors > 0 {
		result.Status = "error"
		result.Behavior.Status = "error"
		result.Behavior.ObservationStatus = "unavailable"
		result.Differences = append(result.Differences, fmt.Sprintf("replay could not evaluate %d rule(s)", replayed.Summary.Errors))
	} else if replayed.Summary.Inconclusive > 0 || replayed.Summary.Skipped > 0 {
		result.Status = "inconclusive"
		result.Behavior.Status = "inconclusive"
		result.Behavior.ObservationStatus = "unavailable"
		result.Differences = append(result.Differences, "replay did not fully evaluate every rule")
	}
}

func indexReplayRules(rules []runner.RuleResult) map[string]runner.RuleResult {
	indexed := make(map[string]runner.RuleResult, len(rules))
	for _, rule := range rules {
		indexed[rule.CaseID] = rule
	}
	return indexed
}

func compareRuleOutcome(recorded, replayed runner.RuleResult) []string {
	differences := make([]string, 0)
	if recorded.RuleID != replayed.RuleID {
		differences = append(differences, fmt.Sprintf("rule ID changed from %q to %q", recorded.RuleID, replayed.RuleID))
	}
	if recorded.Status != replayed.Status {
		differences = append(differences, fmt.Sprintf("rule status changed from %q to %q", recorded.Status, replayed.Status))
	}
	if assertionShape(recorded.Assertions) != assertionShape(replayed.Assertions) {
		differences = append(differences, "assertion outcome shape changed")
	}
	if setupShape(recorded.Setup) != setupShape(replayed.Setup) {
		differences = append(differences, "setup outcome shape changed")
	}
	return differences
}

func assertionShape(assertions []runner.Assertion) string {
	parts := make([]string, 0, len(assertions))
	for _, assertion := range assertions {
		parts = append(parts, assertion.Path+"="+assertion.Status)
	}
	return strings.Join(parts, ";")
}

func setupShape(steps []runner.StepResult) string {
	parts := make([]string, 0, len(steps))
	for _, step := range steps {
		parts = append(parts, step.ID+"="+step.Status)
	}
	return strings.Join(parts, ";")
}
