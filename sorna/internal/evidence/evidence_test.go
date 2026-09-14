package evidence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ingen/sorna/internal/lifecycle"
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
			Access:  &lifecycle.AccessTelemetry{Status: "captured", Source: "test", ProcessID: 42, EventCount: 1},
			AccessEvents: []lifecycle.AccessEvent{{
				Timestamp: now,
				Process:   "subject",
				PID:       42,
				Decision:  "deny",
				Operation: "file-read-data",
				Resource:  "/private/secret",
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
	for _, relative := range []string{"policy/canonical.json", "policy/hash.txt", "policy/subject/canonical.json", "policy/subject/hash.txt", "events/subject-access.jsonl"} {
		if _, ok := manifest.ArtifactsSHA256[relative]; !ok {
			t.Fatalf("manifest artifacts = %+v, want %q", manifest.ArtifactsSHA256, relative)
		}
	}
	if bundle.SubjectAccessPath == "" {
		t.Fatal("bundle subject access path is empty")
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
		ExecutionID:      "oracle-evidence-test",
		Mode:             "sandboxed-process",
		Command:          []string{"sorna", "oracle", "generate"},
		WorkingDir:       directory,
		Backend:          "test-backend",
		Enforcement:      "host-enforced",
		PolicySHA256:     sealedPolicy.SHA256,
		SubjectID:        "contract-test",
		ExecutablePath:   "/bin/sh",
		ExecutableSHA256: strings.Repeat("b", 64),
		StartedAt:        now,
		CompletedAt:      now.Add(time.Second),
		Outcome:          "completed",
		Events: []OracleExecutionEvent{{
			EventID: "evt-0001", Sequence: 1, Timestamp: now, Kind: "oracle.process.completed",
		}},
	}
	bundle, err := WriteOracleBundle(directory, artifact, execution, sealedPolicy)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{bundle.OraclePath, bundle.ManifestPath, bundle.LifecyclePath, bundle.AccessPath, bundle.ChecksumsPath} {
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
	if manifest.Schema != "sorna.oracle-evidence/v1" || manifest.Oracle.SHA256 == "" || manifest.Contract != artifact.Contract || manifest.Assurance.Status != "telemetry-unavailable" {
		t.Fatalf("manifest = %+v, want oracle evidence identity", manifest)
	}
	if _, ok := manifest.ArtifactsSHA256["events/access.jsonl"]; !ok {
		t.Fatalf("manifest artifacts = %+v, want access event hash", manifest.ArtifactsSHA256)
	}
	if err := Verify(directory); err != nil {
		t.Fatalf("Verify() = %v, want valid oracle bundle", err)
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
