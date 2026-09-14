package evidence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"ingen/core/ciresult"
	"ingen/sorna/internal/lifecycle"
	"ingen/sorna/internal/mutation"
	"ingen/sorna/internal/oracle"
	"ingen/sorna/internal/policy"
	"ingen/sorna/internal/runner"
)

func TestWriteBundleAndVerify(t *testing.T) {
	directory := t.TempDir()
	now := time.Date(2026, 9, 13, 20, 0, 0, 0, time.UTC)
	record := runner.RunRecord{
		Schema:    "ingen.run/v1",
		RunID:     "run-evidence-test",
		CreatedAt: now,
		Contract:  runner.ContractReference{ID: "document-pipeline", Version: 1, SHA256: strings.Repeat("a", 64)},
		Oracle:    &runner.OracleReference{Schema: "ingen.oracle/v1", SHA256: strings.Repeat("c", 64)},
		Subject:   runner.SubjectReference{BaseURL: "http://subject.invalid", Adapter: "http-json-v1"},
		Lifecycle: &lifecycle.Record{Mode: "managed-process", Outcome: "stopped", Events: []lifecycle.Event{{
			Sequence:  1,
			Timestamp: now,
			Kind:      "subject.process.started",
			Detail:    "command launched",
		}}},
	}
	bundle, err := WriteBundle(directory, record, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{bundle.RunPath, bundle.ManifestPath, bundle.LifecyclePath, bundle.ChecksumsPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
	}

	manifestBytes, err := os.ReadFile(bundle.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Schema != "sorna.evidence/v1" || manifest.RunID != record.RunID {
		t.Fatalf("manifest = %+v, want evidence schema and run ID", manifest)
	}
	if manifest.Oracle == nil || *manifest.Oracle != *record.Oracle {
		t.Fatalf("manifest oracle = %+v, want run oracle reference", manifest.Oracle)
	}
	if len(manifest.ArtifactsSHA256) != 2 {
		t.Fatalf("manifest artifacts = %+v, want run and lifecycle hashes", manifest.ArtifactsSHA256)
	}
	lifecycleBytes, err := os.ReadFile(bundle.LifecyclePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(lifecycleBytes), `"run_id":"run-evidence-test"`) || !strings.Contains(string(lifecycleBytes), `"actor":"sorna"`) {
		t.Fatalf("lifecycle JSONL = %s, want run identity and actor", lifecycleBytes)
	}
	if err := Verify(directory); err != nil {
		t.Fatalf("Verify() = %v, want valid bundle", err)
	}

	if err := os.WriteFile(filepath.Join(directory, "run.json"), []byte("tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Verify(directory); err == nil || !strings.Contains(err.Error(), "checksum mismatch for run.json") {
		t.Fatalf("Verify() after tamper = %v, want run checksum mismatch", err)
	}
}

func TestWriteBundleEmbedsSealedPolicyAndHashesIt(t *testing.T) {
	directory := t.TempDir()
	now := time.Date(2026, 9, 13, 20, 0, 0, 0, time.UTC)
	sealed, err := policy.Seal(testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	record := runner.RunRecord{
		Schema:    "ingen.run/v1",
		RunID:     "run-policy-evidence-test",
		CreatedAt: time.Date(2026, 9, 13, 20, 0, 0, 0, time.UTC),
		Contract:  runner.ContractReference{ID: "contract-test", Version: 1, SHA256: strings.Repeat("b", 64)},
		Subject:   runner.SubjectReference{BaseURL: "http://subject.invalid", Adapter: "http-json-v1"},
		Lifecycle: &lifecycle.Record{
			Mode:    "managed-process",
			Outcome: "stopped",
			Access: &lifecycle.AccessTelemetry{
				Status:                       "captured",
				Source:                       "test",
				ProcessID:                    42,
				EventCount:                   1,
				ExecutableSampleCount:        1,
				ExecutableSamplingIntervalMS: 25,
				ExecutableSamplingStartedAt:  now,
				ExecutableSamplingStoppedAt:  now.Add(time.Millisecond),
				ExecutableObservationCount:   1,
			},
			AccessEvents: []lifecycle.AccessEvent{{
				Timestamp: now,
				Process:   "subject",
				PID:       42,
				Decision:  "deny",
				Operation: "file-read-data",
				Resource:  "/private/secret",
			}},
			ExecutableObservations: []lifecycle.ExecutableObservation{{
				Timestamp: now,
				PID:       42,
				Path:      "/bin/subject",
				SHA256:    strings.Repeat("d", 64),
			}},
		},
	}
	bundle, err := WriteBundleWithPolicies(directory, record, &sealed, &sealed)
	if err != nil {
		t.Fatal(err)
	}
	manifestBytes, err := os.ReadFile(bundle.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Policy == nil || manifest.Policy.SHA256 != sealed.SHA256 {
		t.Fatalf("manifest policy = %+v, want sealed policy reference", manifest.Policy)
	}
	if manifest.SubjectPolicy == nil || manifest.SubjectPolicy.SHA256 != sealed.SHA256 {
		t.Fatalf("manifest subject policy = %+v, want separate subject policy reference", manifest.SubjectPolicy)
	}
	for _, relative := range []string{"policy/canonical.json", "policy/hash.txt", "policy/subject/canonical.json", "policy/subject/hash.txt", "events/subject-access.jsonl", "events/subject-executables.jsonl"} {
		if _, ok := manifest.ArtifactsSHA256[relative]; !ok {
			t.Fatalf("manifest artifacts = %+v, want %q", manifest.ArtifactsSHA256, relative)
		}
	}
	if bundle.SubjectAccessPath == "" {
		t.Fatal("bundle subject access path is empty")
	}
	if bundle.SubjectExecutablePath == "" {
		t.Fatal("bundle subject executable path is empty")
	}
	if err := Verify(directory); err != nil {
		t.Fatalf("Verify() = %v, want valid policy bundle", err)
	}
}

func TestWriteOracleBundleAndVerify(t *testing.T) {
	directory := t.TempDir()
	sealedPolicy, err := policy.Seal(testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	artifact := oracle.Artifact{
		Schema: "ingen.oracle/v1",
		Status: "frozen",
		Contract: oracle.ContractReference{
			ID:      "contract-test",
			Version: 1,
			SHA256:  strings.Repeat("a", 64),
		},
		PolicySHA256: sealedPolicy.SHA256,
		Cases: []oracle.Case{{
			CaseID: "case-0001", RuleID: "rule.test", Strength: "must", Subject: "GET /test",
		}},
	}
	oraclePath := filepath.Join(directory, "oracle.json")
	if _, err := oracle.WriteFile(oraclePath, artifact); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 14, 5, 0, 0, 0, time.UTC)
	execution := OracleExecution{
		ExecutionID:              "oracle-evidence-test",
		Mode:                     "sandboxed-process",
		Command:                  []string{"sorna", "oracle", "generate"},
		WorkingDir:               directory,
		Backend:                  "test-backend",
		Enforcement:              "host-enforced",
		PolicySHA256:             sealedPolicy.SHA256,
		SubjectID:                "contract-test",
		ExecutablePath:           "/bin/sh",
		ExecutableSHA256:         strings.Repeat("b", 64),
		ObservedExecutablePath:   "/bin/sh",
		ObservedExecutableSHA256: strings.Repeat("b", 64),
		ExecutableObservedAt:     now,
		StartedAt:                now,
		CompletedAt:              now.Add(time.Second),
		Outcome:                  "completed",
		Access: AccessTelemetry{
			Status:                       "captured",
			Source:                       "test",
			ProcessID:                    42,
			ExecutableSampleCount:        1,
			ExecutableSamplingIntervalMS: 25,
			ExecutableSamplingStartedAt:  now,
			ExecutableSamplingStoppedAt:  now.Add(time.Second),
			ExecutableObservationCount:   1,
		},
		ExecutableObservations: []OracleExecutableObservation{{
			EventID:     "executable-0001",
			ExecutionID: "oracle-evidence-test",
			Sequence:    1,
			Timestamp:   now,
			PID:         42,
			Path:        "/bin/sh",
			SHA256:      strings.Repeat("b", 64),
		}},
		Events: []OracleExecutionEvent{{
			EventID: "evt-0001", Sequence: 1, Timestamp: now, Kind: "oracle.process.completed",
		}},
	}
	bundle, err := WriteOracleBundle(directory, artifact, execution, sealedPolicy)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{bundle.OraclePath, bundle.ManifestPath, bundle.LifecyclePath, bundle.AccessPath, bundle.ExecutablePath, bundle.ChecksumsPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
	}
	manifestBytes, err := os.ReadFile(bundle.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest OracleManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Schema != "sorna.oracle-evidence/v1" || manifest.Oracle.SHA256 == "" || manifest.Contract != artifact.Contract || manifest.Assurance.Status != "host-enforced-observed" || manifest.Assurance.ObservationCoverage != "periodic-best-effort" {
		t.Fatalf("manifest = %+v, want oracle evidence identity", manifest)
	}
	if _, ok := manifest.ArtifactsSHA256["events/access.jsonl"]; !ok {
		t.Fatalf("manifest artifacts = %+v, want access event hash", manifest.ArtifactsSHA256)
	}
	if _, ok := manifest.ArtifactsSHA256["events/executables.jsonl"]; !ok {
		t.Fatalf("manifest artifacts = %+v, want executable observation hash", manifest.ArtifactsSHA256)
	}
	if err := Verify(directory); err != nil {
		t.Fatalf("Verify() = %v, want valid oracle bundle", err)
	}
	gateResult, err := EvaluateGate(directory, GatePolicy{MinimumObservationCoverage: "periodic-best-effort"})
	if err != nil {
		t.Fatal(err)
	}
	if gateResult.Status != "passed" || gateResult.ExitCode != 0 || gateResult.OracleOutcome != "completed" || gateResult.ObservationCoverage != "periodic-best-effort" {
		t.Fatalf("oracle gate result = %+v; want passing completed oracle gate", gateResult)
	}
	ciArtifact, err := BuildCIResult(directory, gateResult)
	if err != nil {
		t.Fatal(err)
	}
	if err := ciArtifact.Validate(); err != nil {
		t.Fatalf("shared CI result validation = %v", err)
	}
	if ciArtifact.Tool != "sorna" || ciArtifact.Kind != "behavioral-verification" || ciArtifact.Status != "passed" || ciArtifact.ExitCode != 0 {
		t.Fatalf("shared CI result = %+v, want passing Sorna envelope", ciArtifact)
	}
	if ciArtifact.Policy == nil || ciArtifact.Policy.Path != "policy/canonical.json" {
		t.Fatalf("shared CI policy = %+v, want canonical policy reference", ciArtifact.Policy)
	}
	for _, input := range []string{"manifest", "oracle", "oracle_events", "access_events", "executable_events"} {
		if _, ok := ciArtifact.Inputs[input]; !ok {
			t.Fatalf("shared CI inputs = %+v, want %q", ciArtifact.Inputs, input)
		}
	}
	var report GateResult
	if err := json.Unmarshal(ciArtifact.Report, &report); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(report, gateResult) {
		t.Fatalf("shared CI report = %+v, want unchanged gate result %+v", report, gateResult)
	}
	if ciArtifact.Schema != ciresult.Schema {
		t.Fatalf("shared CI schema = %q, want %q", ciArtifact.Schema, ciresult.Schema)
	}
	executableStreamPath := filepath.Join(directory, "events", "executables.jsonl")
	executableStream, err := os.ReadFile(executableStreamPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executableStreamPath, append(executableStream, executableStream...), 0o644); err != nil {
		t.Fatal(err)
	}
	checksumsPath := filepath.Join(directory, "checksums.sha256")
	checksums, err := os.ReadFile(checksumsPath)
	if err != nil {
		t.Fatal(err)
	}
	updatedExecutableHash, err := hashFile(executableStreamPath)
	if err != nil {
		t.Fatal(err)
	}
	checksumsText := strings.Replace(string(checksums), hashBytes(executableStream), updatedExecutableHash, 1)
	if err := os.WriteFile(checksumsPath, []byte(checksumsText), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Verify(directory); err == nil || !strings.Contains(err.Error(), "contains 2 records, expected 1") {
		t.Fatalf("Verify() after executable stream duplication = %v, want semantic count error", err)
	}
	execution.ObservedExecutableSHA256 = strings.Repeat("c", 64)
	if _, err := WriteOracleBundle(directory, artifact, execution, sealedPolicy); err == nil || !strings.Contains(err.Error(), "does not match prepared identity") {
		t.Fatalf("WriteOracleBundle() = %v, want observed identity mismatch", err)
	}
}

func TestValidateExecutableSamplingRejectsInconsistentMetadata(t *testing.T) {
	now := time.Date(2026, 9, 14, 5, 0, 0, 0, time.UTC)
	tests := []struct {
		name              string
		sampleCount       int
		intervalMS        int
		startedAt         time.Time
		stoppedAt         time.Time
		observationCount  int
		observationErrors int
	}{
		{name: "observations without attempts", sampleCount: 0, observationCount: 1},
		{name: "missing window", sampleCount: 1, intervalMS: 25, startedAt: now},
		{name: "non-positive interval", sampleCount: 1, startedAt: now, stoppedAt: now},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateExecutableSampling(
				test.sampleCount,
				test.intervalMS,
				test.startedAt,
				test.stoppedAt,
				test.observationCount,
				test.observationErrors,
			)
			if err == nil {
				t.Fatal("validateExecutableSampling() = nil, want metadata error")
			}
		})
	}
}

func TestVerifyExecutableObservationStreamRejectsOutsideWindow(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "executables.jsonl")
	if err := os.WriteFile(path, []byte(`{"timestamp":"2026-09-14T04:59:59Z"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 9, 14, 5, 0, 0, 0, time.UTC)
	stop := start.Add(time.Second)
	err := verifyExecutableObservationStream(path, true, 1, start, stop, func([]byte) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "outside the sampling window") {
		t.Fatalf("verifyExecutableObservationStream() = %v, want sampling-window error", err)
	}
}

func TestVerifyRejectsUnsafeChecksumPath(t *testing.T) {
	directory := t.TempDir()
	checksums := strings.Repeat("a", 64) + "  ../outside\n"
	if err := os.WriteFile(filepath.Join(directory, "checksums.sha256"), []byte(checksums), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Verify(directory); err == nil || !strings.Contains(err.Error(), "inside the evidence bundle") {
		t.Fatalf("Verify() = %v, want unsafe path error", err)
	}
}

func TestEvaluateGateKeepsCoverageReportOnlyByDefault(t *testing.T) {
	directory := t.TempDir()
	record := runner.RunRecord{
		Schema:    "ingen.run/v1",
		RunID:     "run-gate-report-only",
		CreatedAt: time.Now().UTC(),
		Assurance: runner.Assurance{
			Level:               0,
			Status:              "host-enforced-subject",
			ObservationCoverage: "periodic-best-effort-with-gaps",
		},
		Contract: runner.ContractReference{ID: "contract-test", Version: 1, SHA256: strings.Repeat("a", 64)},
		Subject:  runner.SubjectReference{BaseURL: "http://subject.invalid", Adapter: "http-json-v1"},
		Verdict:  runner.ContractVerdict{Status: "pass", Reason: "all rules passed"},
	}
	if _, err := WriteBundle(directory, record, nil); err != nil {
		t.Fatal(err)
	}
	result, err := EvaluateGate(directory, GatePolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "passed" || result.ExitCode != 0 || len(result.Warnings) != 1 || len(result.Reasons) != 0 {
		t.Fatalf("report-only result = %+v; want passing gate with one warning", result)
	}
	strict, err := EvaluateGate(directory, GatePolicy{MinimumObservationCoverage: "periodic-best-effort"})
	if err != nil {
		t.Fatal(err)
	}
	if strict.Status != "failed" || strict.ExitCode != 1 || len(strict.Reasons) != 1 {
		t.Fatalf("strict result = %+v; want coverage gate failure", strict)
	}
	record.Verdict = runner.ContractVerdict{Status: "fail", Reason: "ordinary contract failure"}
	failedDirectory := t.TempDir()
	if _, err := WriteBundle(failedDirectory, record, nil); err != nil {
		t.Fatal(err)
	}
	failed, err := EvaluateGate(failedDirectory, GatePolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != "failed" || failed.ExitCode != 1 || len(failed.Reasons) != 1 || !strings.Contains(failed.Reasons[0], "contract verdict") {
		t.Fatalf("ordinary failure result = %+v; want blocking contract gate", failed)
	}
}

func TestEvaluateGateTreatsKilledMutationAsPassing(t *testing.T) {
	directory := t.TempDir()
	record := runner.RunRecord{
		Schema:    "ingen.run/v1",
		RunID:     "run-gate-killed",
		CreatedAt: time.Now().UTC(),
		Assurance: runner.Assurance{ObservationCoverage: "periodic-best-effort"},
		Contract:  runner.ContractReference{ID: "contract-test", Version: 1, SHA256: strings.Repeat("a", 64)},
		Subject:   runner.SubjectReference{BaseURL: "http://subject.invalid", Adapter: "http-json-v1"},
		Verdict:   runner.ContractVerdict{Status: "fail", Reason: "mutation was detected"},
		Mutation:  &mutation.Result{Outcome: "killed"},
	}
	if _, err := WriteBundle(directory, record, nil); err != nil {
		t.Fatal(err)
	}
	result, err := EvaluateGate(directory, GatePolicy{MinimumObservationCoverage: "periodic-best-effort"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "passed" || result.ExitCode != 0 || result.MutationOutcome != "killed" {
		t.Fatalf("killed mutation result = %+v; want passing gate", result)
	}
}

func TestEvaluateGateRejectsUnknownCoveragePolicy(t *testing.T) {
	if _, err := EvaluateGate(t.TempDir(), GatePolicy{MinimumObservationCoverage: "continuous-attested"}); err == nil || !strings.Contains(err.Error(), "minimum observation coverage") {
		t.Fatalf("EvaluateGate() = %v, want invalid coverage policy error", err)
	}
}

func TestBuildCIErrorResultPreservesVerificationFailure(t *testing.T) {
	artifact, err := BuildCIErrorResult(".artifacts/missing", fmt.Errorf("checksums.sha256 is missing"))
	if err != nil {
		t.Fatal(err)
	}
	if err := artifact.Validate(); err != nil {
		t.Fatalf("shared CI error validation = %v", err)
	}
	if artifact.Status != "error" || artifact.ExitCode != 2 || artifact.Error != "checksums.sha256 is missing" {
		t.Fatalf("shared CI error = %+v, want preserved verification failure", artifact)
	}
}

func TestValidateBaselineRequiresMatchingPassingRun(t *testing.T) {
	sealedPolicy, err := policy.Seal(testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	contract := runner.ContractReference{ID: "contract-test", Version: 1, SHA256: strings.Repeat("a", 64)}
	oracleReference := &runner.OracleReference{Schema: "ingen.oracle/v1", SHA256: strings.Repeat("b", 64)}
	record := runner.RunRecord{
		Schema:    "ingen.run/v1",
		RunID:     "run-baseline-valid",
		CreatedAt: time.Now().UTC(),
		Contract:  contract,
		Oracle:    oracleReference,
		Subject:   runner.SubjectReference{BaseURL: "http://subject.invalid", Adapter: "http-json-v1", Variant: "clean-baseline"},
		Verdict:   runner.ContractVerdict{Status: "pass", Reason: "all rules passed"},
	}
	directory := t.TempDir()
	if _, err := WriteBundleWithPolicies(directory, record, &sealedPolicy, &sealedPolicy); err != nil {
		t.Fatal(err)
	}
	baseline, err := ValidateBaseline(directory, BaselineRequirements{
		Contract:            contract,
		Oracle:              oracleReference,
		PolicySHA256:        sealedPolicy.SHA256,
		SubjectPolicySHA256: sealedPolicy.SHA256,
	})
	if err != nil {
		t.Fatal(err)
	}
	if baseline.EvidencePath != directory || baseline.RunID != record.RunID || baseline.Contract != contract || !sameOracle(baseline.Oracle, oracleReference) {
		t.Fatalf("baseline = %+v, want matching baseline reference", baseline)
	}
}

func TestValidateBaselineRejectsInvalidComparisonInputs(t *testing.T) {
	sealedPolicy, err := policy.Seal(testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	contract := runner.ContractReference{ID: "contract-test", Version: 1, SHA256: strings.Repeat("a", 64)}
	oracleReference := &runner.OracleReference{Schema: "ingen.oracle/v1", SHA256: strings.Repeat("b", 64)}
	tests := []struct {
		name        string
		verdict     string
		mutation    *mutation.Result
		requirement BaselineRequirements
		want        string
	}{
		{name: "contract failure", verdict: "fail", want: "contract verdict"},
		{name: "mutation baseline", verdict: "pass", mutation: &mutation.Result{Spec: mutation.Spec{ID: "already-mutated"}}, want: "contains mutation"},
		{name: "contract mismatch", verdict: "pass", requirement: BaselineRequirements{Contract: runner.ContractReference{ID: "other", Version: 1, SHA256: strings.Repeat("a", 64)}}, want: "does not match candidate"},
		{name: "policy mismatch", verdict: "pass", requirement: BaselineRequirements{Contract: contract, Oracle: oracleReference, PolicySHA256: strings.Repeat("f", 64), SubjectPolicySHA256: sealedPolicy.SHA256}, want: "oracle policy hash"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := runner.RunRecord{
				Schema:    "ingen.run/v1",
				RunID:     "run-baseline-" + strings.ReplaceAll(test.name, " ", "-"),
				CreatedAt: time.Now().UTC(),
				Contract:  contract,
				Oracle:    oracleReference,
				Subject:   runner.SubjectReference{BaseURL: "http://subject.invalid", Adapter: "http-json-v1"},
				Verdict:   runner.ContractVerdict{Status: test.verdict},
				Mutation:  test.mutation,
			}
			directory := t.TempDir()
			if _, err := WriteBundleWithPolicies(directory, record, &sealedPolicy, &sealedPolicy); err != nil {
				t.Fatal(err)
			}
			requirement := test.requirement
			if requirement.Contract == (runner.ContractReference{}) {
				requirement.Contract = contract
			}
			if requirement.Oracle == nil {
				requirement.Oracle = oracleReference
			}
			if requirement.PolicySHA256 == "" {
				requirement.PolicySHA256 = sealedPolicy.SHA256
			}
			if requirement.SubjectPolicySHA256 == "" {
				requirement.SubjectPolicySHA256 = sealedPolicy.SHA256
			}
			if _, err := ValidateBaseline(directory, requirement); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateBaseline() = %v, want %q", err, test.want)
			}
		})
	}
}

func testPolicy() policy.Document {
	return policy.Document{Policy: map[string]any{
		"schema":      "ingen.policy/v1",
		"id":          "policy-evidence-test",
		"version":     int64(1),
		"status":      "draft",
		"purpose":     "test",
		"enforcement": "declared-only",
		"filesystem": map[string]any{
			"read":  []any{map[string]any{"path": "contract", "reason": "input"}},
			"write": []any{map[string]any{"path": "output", "reason": "output"}},
			"deny":  []any{map[string]any{"path": "implementation", "reason": "blocked"}},
		},
		"network": map[string]any{"mode": "disabled"},
		"process": map[string]any{"subject_id": "contract-test", "can_invoke_subject": false},
	}}
}
