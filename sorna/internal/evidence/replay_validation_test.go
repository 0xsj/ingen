package evidence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/sorna/internal/oracle"
	"ingen/sorna/internal/runner"
)

func TestValidateReplayResultAcceptsConsistentReport(t *testing.T) {
	if err := ValidateReplayResult(validReplayReport()); err != nil {
		t.Fatalf("ValidateReplayResult() = %v, want valid report", err)
	}
}

func TestValidateReplayResultRejectsAmbiguousOrInconsistentReport(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ReplayResult)
		want   string
	}{
		{
			name: "duplicate case identity",
			mutate: func(result *ReplayResult) {
				result.Rules = append(result.Rules, result.Rules[0])
			},
			want: "case_id duplicates",
		},
		{
			name: "matched report contains drift",
			mutate: func(result *ReplayResult) {
				result.Rules[0].Status = "outcome-drift"
				result.Rules[0].Differences = []string{"tampered"}
			},
			want: "contract-visible difference",
		},
		{
			name: "observation count does not match",
			mutate: func(result *ReplayResult) {
				result.ObservationChanges = 1
			},
			want: "observation_changes",
		},
		{
			name: "matched report contains request drift",
			mutate: func(result *ReplayResult) {
				result.Rules[0].ReplayRequestSHA256 = strings.Repeat("b", 64)
				result.Rules[0].RequestStatus = "changed"
				result.Behavior.RequestStatus = "changed"
				result.RequestChanges = 1
			},
			want: "unverified request intent",
		},
		{
			name: "matched report has unavailable request fingerprint",
			mutate: func(result *ReplayResult) {
				result.Rules[0].ReplayRequestSHA256 = ""
				result.Rules[0].RequestStatus = "unavailable"
				result.Behavior.RequestStatus = "unavailable"
			},
			want: "unverified request intent",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := validReplayReport()
			test.mutate(&result)
			if err := ValidateReplayResult(result); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateReplayResult() = %v, want error containing %q", err, test.want)
			}
		})
	}
}

func TestLoadReplayReportValidatesSavedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "replay.json")
	contents, err := json.MarshalIndent(validReplayReport(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(contents, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadReplayReport(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != "matched" || len(loaded.Rules) != 1 {
		t.Fatalf("loaded report = %+v, want matching report", loaded)
	}

	tampered := validReplayReport()
	tampered.Status = "drifted"
	tamperedBytes, err := json.Marshal(tampered)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, tamperedBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadReplayReport(path); err == nil || !strings.Contains(err.Error(), "behavior status") {
		t.Fatalf("LoadReplayReport() = %v, want tampered report rejection", err)
	}
}

func validReplayReport() ReplayResult {
	digest := strings.Repeat("a", 64)
	return ReplayResult{
		Schema:          replaySchema,
		Status:          "matched",
		EvidencePath:    "evidence/run",
		RecordedRunID:   "run-recorded",
		ReplayRunID:     "run-replay",
		Contract:        runner.ContractReference{ID: "contract", Version: 1, SHA256: digest},
		Oracle:          runner.OracleReference{Schema: oracle.Schema, SHA256: digest},
		Integrity:       ReplayIntegrity{Status: "verified"},
		Behavior:        ReplayBehavior{Status: "matched", ObservationStatus: "same", RequestStatus: "same"},
		RecordedSubject: runner.SubjectReference{BaseURL: "http://recorded.invalid", Adapter: "http-json-v1"},
		ReplaySubject:   runner.SubjectReference{BaseURL: "http://replay.invalid", Adapter: "http-json-v1"},
		RecordedVerdict: runner.ContractVerdict{Status: "pass", Reason: "all rules passed"},
		ReplayVerdict:   runner.ContractVerdict{Status: "pass", Reason: "all rules passed"},
		Rules: []ReplayRuleResult{{
			RuleID:                    "rule.health",
			CaseID:                    "case-0001",
			Status:                    "match",
			RecordedStatus:            "pass",
			ReplayStatus:              "pass",
			RecordedRequestSHA256:     digest,
			ReplayRequestSHA256:       digest,
			RequestStatus:             "same",
			RecordedObservationSHA256: digest,
			ReplayObservationSHA256:   digest,
			ObservationStatus:         "same",
		}},
	}
}
