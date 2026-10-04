package evidence

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v3"
	"ingen/herdr-sentinel/internal/agentlaunch"
	"ingen/herdr-sentinel/internal/project"
	"ingen/herdr-sentinel/internal/roleexec"
	"ingen/herdr-sentinel/internal/workflowgate"
	"ingen/herdr-sentinel/internal/workspace"
	"ingen/sorna/contract"
	"ingen/sorna/oracle"
	"ingen/sorna/policy"
)

func TestVerifyAndBuildCIResultPreservesExactEvidence(t *testing.T) {
	root, reportPath, reportBytes := evidenceFixture(t)
	verified, err := Verify(root, reportPath, hash(reportBytes))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(verified.ReportBytes, reportBytes) || verified.ReportSHA256 != hash(reportBytes) {
		t.Fatal("verifier did not preserve exact report bytes and digest")
	}
	artifact, err := verified.BuildCIResult(root)
	if err != nil {
		t.Fatal(err)
	}
	var report map[string]any
	if err := json.Unmarshal(artifact.Report, &report); err != nil {
		t.Fatalf("producer report is not embedded as JSON: %v", err)
	}
	if report["execution_id"] != "evidence-case" || len(artifact.Inputs) != 5 {
		t.Fatalf("unexpected CI report or input refs: report=%v inputs=%v", report["execution_id"], artifact.Inputs)
	}
	second, err := verified.BuildCIResult(root)
	if err != nil || !bytes.Equal(mustJSON(t, artifact), mustJSON(t, second)) {
		t.Fatalf("CI producer result is not deterministic: %v", err)
	}
	verified.Report.Status = "failed"
	if _, err := verified.BuildCIResult(root); err == nil {
		t.Fatal("builder accepted a caller-mutated verified report")
	}
}

func TestVerifyBindsTypedFreshCodexPromptAndInvocation(t *testing.T) {
	root, reportPath, raw := evidenceFixture(t)
	var report Report
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	oldScratch, oldWritable := report.DerivedReadRoots[0], report.DerivedWriteRoots[0]
	promptPath := ".ingen/brief.md"
	if err := os.WriteFile(filepath.Join(root, promptPath), []byte("Review this exact task input.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	prompt, err := os.ReadFile(filepath.Join(root, promptPath))
	if err != nil {
		t.Fatal(err)
	}
	promptSHA := hash(prompt)
	context, args, err := agentlaunch.BuildCodex(agentlaunch.CodexRequest{
		ExecutionID: report.ExecutionID, Root: root,
		WorkingDir:       filepath.Join(root, ".ingen/artifacts/role-executions/.runtime", report.ExecutionID),
		HomePath:         filepath.Join(root, ".ingen/artifacts/role-executions/.runtime", report.ExecutionID, "rw/home"),
		CodexHome:        filepath.Join(root, ".ingen/artifacts/role-executions/.runtime", report.ExecutionID, "rw/codex-home"),
		PromptSourcePath: promptPath, PromptSourceSHA256: promptSHA,
		PromptSnapshotPath: ".ingen/artifacts/role-executions/evidence-case.prompt", PromptSnapshotSHA256: promptSHA,
		PromptBytes: len(prompt), Model: "codex-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	context.Args = args
	report.AgentContext = &context
	report.ExecutablePath = "/opt/codex/bin/codex"
	report.Limitations = []string{"Codex was requested in a fresh ephemeral CLI mode; Sentinel cannot attest provider-side model context or retention", "provider credentials are not inherited and network access is disabled; remote inference is not available in this profile", "executable digest is a prelaunch byte check, not running-image identity", "Darwin runtime bootstrap and ancestor metadata are available", "no process namespace", "no host attestation", "noninteractive execution only"}
	policyPath := filepath.Join(root, filepath.FromSlash(report.PolicyPath))
	policyBytes, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := policy.LoadBytes(report.PolicyPath, policyBytes)
	if err != nil {
		t.Fatal(err)
	}
	filesystem := sealed.Policy["filesystem"].(map[string]any)
	sealed.Policy["status"] = "draft"
	filesystem["read"] = replacePolicyRoot(filesystem["read"], oldScratch, context.WorkingDirectory)
	filesystem["write"] = replacePolicyRoot(filesystem["write"], oldWritable, filepath.Join(context.WorkingDirectory, "rw"))
	sealedExecutionPolicy, err := policy.Seal(policy.Document{Policy: sealed.Policy})
	if err != nil {
		t.Fatal(err)
	}
	policyBytes, err = policy.CanonicalJSON(sealedExecutionPolicy.Document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policyPath, policyBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	report.PolicySHA256 = hash(policyBytes)
	report.DerivedReadRoots = []string{context.WorkingDirectory}
	report.DerivedWriteRoots = []string{filepath.Join(context.WorkingDirectory, "rw")}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(context.PromptSnapshotPath)), prompt, 0o600); err != nil {
		t.Fatal(err)
	}
	reportBytes, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	schema, err := compiler.Compile("../spec/role-execution-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schemaDocument any
	if err := json.Unmarshal(reportBytes, &schemaDocument); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(schemaDocument); err != nil {
		t.Fatalf("typed role-execution report rejected by published schema: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(reportPath)), reportBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	verified, err := Verify(root, reportPath, "")
	if err != nil {
		t.Fatalf("valid typed context rejected: %v", err)
	}
	if _, ok := fileKind(verified.Files, FileAgentPromptSource); !ok {
		t.Fatal("verified input omits prompt source bytes")
	}
	if _, ok := fileKind(verified.Files, FileAgentPrompt); !ok {
		t.Fatal("verified input omits exact prompt snapshot bytes")
	}
	var nullFields map[string]any
	if err := json.Unmarshal(reportBytes, &nullFields); err != nil {
		t.Fatal(err)
	}
	nullFields["agent_context"] = nil
	nullReport, err := json.Marshal(nullFields)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(reportPath)), nullReport, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(root, reportPath, ""); err == nil {
		t.Fatal("accepted explicit null agent context")
	}

	context.Args = append(context.Args, "--resume")
	report.AgentContext = &context
	changed, _ := json.Marshal(report)
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(reportPath)), changed, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(root, reportPath, ""); err == nil {
		t.Fatal("accepted arbitrary Codex resume argument in recorded context")
	}
}

func TestVerifyBrokerProfilePolicyInvocationAndSecretFreeStats(t *testing.T) {
	t.Run("zero broker requests are a lifecycle fact, not inference success", func(t *testing.T) {
		root, reportPath, _ := brokerEvidenceFixture(t)
		verified, err := Verify(root, reportPath, "")
		if err != nil {
			t.Fatalf("valid local broker profile rejected: %v", err)
		}
		if verified.Report.BrokerExecution.Stats.RequestsForwarded != 0 || verified.Report.BrokerExecution.Status != "closed" {
			t.Fatalf("unexpected broker observation: %+v", verified.Report.BrokerExecution)
		}
		artifact, err := verified.BuildCIResult(root)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(string(artifact.Explanation)), "inference succeeded") || strings.Contains(strings.ToLower(string(artifact.Explanation)), "provider success") {
			t.Fatalf("zero broker requests were described as provider success: %s", artifact.Explanation)
		}
	})
	t.Run("unknown provider", func(t *testing.T) {
		root, reportPath, report := brokerEvidenceFixture(t)
		report.AgentContext.Broker.Provider = "other-provider"
		report.BrokerExecution.Provider = "other-provider"
		writeEvidenceReport(t, root, reportPath, report)
		if _, err := Verify(root, reportPath, ""); err == nil {
			t.Fatal("accepted unsupported broker provider")
		}
	})
	t.Run("redirect endpoint", func(t *testing.T) {
		root, reportPath, report := brokerEvidenceFixture(t)
		report.AgentContext.Broker.Endpoint = "http://localhost:34567/v1"
		report.BrokerExecution.Endpoint = report.AgentContext.Broker.Endpoint
		writeEvidenceReport(t, root, reportPath, report)
		if _, err := Verify(root, reportPath, ""); err == nil {
			t.Fatal("accepted redirectable or nonliteral broker endpoint")
		}
	})
	t.Run("extra network allowance", func(t *testing.T) {
		root, reportPath, report := brokerEvidenceFixture(t)
		policyPath := filepath.Join(root, filepath.FromSlash(report.PolicyPath))
		policyBytes, err := os.ReadFile(policyPath)
		if err != nil {
			t.Fatal(err)
		}
		document, err := policy.LoadBytes(report.PolicyPath, policyBytes)
		if err != nil {
			t.Fatal(err)
		}
		document.Policy["status"] = "draft"
		network := document.Policy["network"].(map[string]any)
		allow := network["allow"].([]any)
		allow = append(allow, map[string]any{"host": "api.example.invalid", "ports": []any{443}, "direction": "outbound", "purpose": "unauthorized extra egress"})
		network["allow"] = allow
		sealed, err := policy.Seal(policy.Document{Policy: document.Policy})
		if err != nil {
			t.Fatal(err)
		}
		policyBytes, err = policy.CanonicalJSON(sealed.Document)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(policyPath, policyBytes, 0o600); err != nil {
			t.Fatal(err)
		}
		report.PolicySHA256 = hash(policyBytes)
		writeEvidenceReport(t, root, reportPath, report)
		if _, err := Verify(root, reportPath, ""); err == nil {
			t.Fatal("accepted extra outbound network allowance")
		}
	})
	t.Run("forged Codex arguments", func(t *testing.T) {
		root, reportPath, report := brokerEvidenceFixture(t)
		report.AgentContext.Args = append(report.AgentContext.Args, "--resume")
		writeEvidenceReport(t, root, reportPath, report)
		if _, err := Verify(root, reportPath, ""); err == nil {
			t.Fatal("accepted arbitrary brokered Codex arguments")
		}
	})
	t.Run("mismatched stats", func(t *testing.T) {
		root, reportPath, report := brokerEvidenceFixture(t)
		report.BrokerExecution.Stats.RequestsForwarded = 1
		report.BrokerExecution.Stats.RequestsReceived = 1
		report.BrokerExecution.Stats.RequestsRejected = 1
		writeEvidenceReport(t, root, reportPath, report)
		if _, err := Verify(root, reportPath, ""); err == nil {
			t.Fatal("accepted a closed broker snapshot with an unclassified forwarded request")
		}
	})
	t.Run("null required counter", func(t *testing.T) {
		root, reportPath, _ := brokerEvidenceFixture(t)
		path := filepath.Join(root, filepath.FromSlash(reportPath))
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		changed := bytes.Replace(contents, []byte(`"requests_received":0`), []byte(`"requests_received":null`), 1)
		if bytes.Equal(changed, contents) {
			t.Fatal("broker fixture did not contain a zero requests_received counter")
		}
		if err := os.WriteFile(path, changed, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Verify(root, reportPath, ""); err == nil {
			t.Fatal("accepted null in a required broker counter")
		}
	})
}

func brokerEvidenceFixture(t *testing.T) (string, string, Report) {
	t.Helper()
	root, reportPath, raw := evidenceFixture(t)
	var report Report
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	oldScratch, oldWritable := report.DerivedReadRoots[0], report.DerivedWriteRoots[0]
	promptPath := ".ingen/brief.md"
	prompt := []byte("Review this exact task input.\n")
	if err := os.WriteFile(filepath.Join(root, promptPath), prompt, 0o600); err != nil {
		t.Fatal(err)
	}
	promptSHA := hash(prompt)
	broker := &agentlaunch.BrokerContext{
		Provider: "openai-responses", Endpoint: "http://127.0.0.1:34567/v1", CredentialSource: "environment", CredentialEnv: "OPENAI_API_KEY",
		MaxRequests: 16, MaxOutputTokens: 4096, TimeoutSeconds: 300,
	}
	context, args, err := agentlaunch.BuildCodex(agentlaunch.CodexRequest{
		ExecutionID: report.ExecutionID, Root: root,
		WorkingDir:       filepath.Join(root, ".ingen/artifacts/role-executions/.runtime", report.ExecutionID),
		HomePath:         filepath.Join(root, ".ingen/artifacts/role-executions/.runtime", report.ExecutionID, "rw/home"),
		CodexHome:        filepath.Join(root, ".ingen/artifacts/role-executions/.runtime", report.ExecutionID, "rw/codex-home"),
		PromptSourcePath: promptPath, PromptSourceSHA256: promptSHA,
		PromptSnapshotPath: ".ingen/artifacts/role-executions/evidence-case.prompt", PromptSnapshotSHA256: promptSHA,
		PromptBytes: len(prompt), Model: "gpt-test", Broker: broker,
	})
	if err != nil {
		t.Fatal(err)
	}
	context.Args = args
	report.AgentContext = &context
	report.ExecutablePath = "/opt/codex/bin/codex"
	report.NetworkMode = "allowlist"
	report.Limitations = []string{"provider-side model context and retention are not attested", "upstream credentials are read by Sentinel and are not inherited by the Codex child", "broker counters are local request lifecycle observations and do not attest provider inference or retention", "macOS Seatbelt localhost network permission includes local host addresses at the pinned TCP port; it is not an IPv4-only grant", "executable digest is a prelaunch byte check, not running-image identity", "Darwin runtime bootstrap and ancestor metadata are available", "no process namespace", "no host attestation", "noninteractive execution only"}
	started := time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano)
	finished := time.Now().UTC().Format(time.RFC3339Nano)
	report.StartedAt, report.FinishedAt = started, finished
	report.BrokerExecution = &roleexec.BrokerExecution{
		Status: "closed", Provider: broker.Provider, Endpoint: broker.Endpoint, Model: context.Model,
		MaxRequests: broker.MaxRequests, MaxOutputTokens: broker.MaxOutputTokens, TimeoutSeconds: broker.TimeoutSeconds,
		Stats: roleexec.BrokerStats{StartedAt: started, ClosedAt: finished},
	}

	policyPath := filepath.Join(root, filepath.FromSlash(report.PolicyPath))
	policyBytes, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	document, err := policy.LoadBytes(report.PolicyPath, policyBytes)
	if err != nil {
		t.Fatal(err)
	}
	document.Policy["status"] = "draft"
	filesystem := document.Policy["filesystem"].(map[string]any)
	filesystem["read"] = replacePolicyRoot(filesystem["read"], oldScratch, context.WorkingDirectory)
	filesystem["write"] = replacePolicyRoot(filesystem["write"], oldWritable, filepath.Join(context.WorkingDirectory, "rw"))
	document.Policy["network"] = map[string]any{"mode": "allowlist", "allow": []any{map[string]any{
		"host": "localhost", "ports": []any{34567}, "direction": "outbound", "purpose": "local Codex Responses broker",
	}}}
	sealed, err := policy.Seal(policy.Document{Policy: document.Policy})
	if err != nil {
		t.Fatal(err)
	}
	policyBytes, err = policy.CanonicalJSON(sealed.Document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policyPath, policyBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	report.PolicySHA256 = hash(policyBytes)
	report.DerivedReadRoots = []string{context.WorkingDirectory}
	report.DerivedWriteRoots = []string{filepath.Join(context.WorkingDirectory, "rw")}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(context.PromptSnapshotPath)), prompt, 0o600); err != nil {
		t.Fatal(err)
	}
	writeEvidenceReport(t, root, reportPath, report)
	return root, reportPath, report
}

func writeEvidenceReport(t *testing.T, root, reportPath string, report Report) {
	t.Helper()
	contents, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(reportPath)), contents, 0o600); err != nil {
		t.Fatal(err)
	}
}

func replacePolicyRoot(value any, oldPath, newPath string) []any {
	entries, _ := value.([]any)
	result := make([]any, 0, len(entries))
	for _, item := range entries {
		entry, _ := item.(map[string]any)
		if entry["path"] == oldPath {
			entry = map[string]any{"path": newPath, "reason": entry["reason"]}
		}
		result = append(result, entry)
	}
	return result
}

func TestVerifyRejectsTamperedReferencesAndStrictJSON(t *testing.T) {
	t.Run("expected report digest", func(t *testing.T) {
		root, reportPath, _ := evidenceFixture(t)
		if _, err := Verify(root, reportPath, strings.Repeat("0", 64)); err == nil {
			t.Fatal("accepted wrong expected report digest")
		}
	})
	t.Run("capture tampering", func(t *testing.T) {
		root, reportPath, _ := evidenceFixture(t)
		if err := os.WriteFile(filepath.Join(root, ".ingen/artifacts/role-executions/evidence-case.stdout"), []byte("changed"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Verify(root, reportPath, ""); err == nil {
			t.Fatal("accepted changed stdout bytes")
		}
	})
	t.Run("duplicate report key", func(t *testing.T) {
		root, reportPath, reportBytes := evidenceFixture(t)
		duplicated := append([]byte("{\"schema\":\"ingen.sentinel-role-execution/v1\","), reportBytes[1:]...)
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(reportPath)), duplicated, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Verify(root, reportPath, ""); err == nil {
			t.Fatal("accepted duplicate JSON object key")
		}
	})
	t.Run("trailing JSON", func(t *testing.T) {
		root, reportPath, reportBytes := evidenceFixture(t)
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(reportPath)), append(reportBytes, []byte(" {}")...), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Verify(root, reportPath, ""); err == nil {
			t.Fatal("accepted a trailing JSON value")
		}
	})
	t.Run("null required array", func(t *testing.T) {
		root, reportPath, reportBytes := evidenceFixture(t)
		changed := bytes.Replace(reportBytes, []byte("\"allowed_tools\":[]"), []byte("\"allowed_tools\":null"), 1)
		if bytes.Equal(changed, reportBytes) {
			t.Fatal("fixture did not contain expected array")
		}
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(reportPath)), changed, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Verify(root, reportPath, ""); err == nil {
			t.Fatal("accepted null where a required report array is expected")
		}
	})
}

func TestVerifyRejectsEmptyRootAndSymlinkEscape(t *testing.T) {
	if _, err := Verify("", ".ingen/report.json", ""); err == nil {
		t.Fatal("empty root implicitly selected the process working directory")
	}
	root, reportPath, _ := evidenceFixture(t)
	external := t.TempDir()
	stdoutPath := filepath.Join(root, ".ingen/artifacts/role-executions/evidence-case.stdout")
	if err := os.Remove(stdoutPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(external, "outside"), stdoutPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := Verify(root, reportPath, ""); err == nil {
		t.Fatal("accepted capture symlink escaping project root")
	}
}

func TestCanceledExecutionBuildsErrorResultWithActualExitCode(t *testing.T) {
	root, reportPath, reportBytes := evidenceFixture(t)
	var report Report
	if err := json.Unmarshal(reportBytes, &report); err != nil {
		t.Fatal(err)
	}
	report.Status = "canceled"
	report.Reason = "interrupt requested"
	changed, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(reportPath)), changed, 0o600); err != nil {
		t.Fatal(err)
	}
	verified, err := Verify(root, reportPath, "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := verified.BuildCIResult(root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "error" || result.ExitCode != 2 || result.Error != "interrupt requested" {
		t.Fatalf("canceled process was not mapped to an error result: %+v", result)
	}
}

func TestGovernedEvidenceBindsApprovalAuthoritySourceAndFrozenOracleLimit(t *testing.T) {
	root, reportPath, _ := governedEvidenceFixture(t, false)
	verified, err := Verify(root, reportPath, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{FileContractSource, FileOraclePolicy, "governance-hammond-approval", "governance-hammond-review-policy", "governance-hammond-review-authority", "governance-sorna-contract"} {
		if _, found := fileKind(verified.Files, kind); !found {
			t.Fatalf("verified inputs omit %q", kind)
		}
	}
	result, err := verified.BuildCIResult(root)
	if err != nil {
		t.Fatal(err)
	}
	var explanation map[string]any
	if err := json.Unmarshal(result.Explanation, &explanation); err != nil {
		t.Fatal(err)
	}
	meaning, _ := explanation["meaning"].(string)
	if !strings.Contains(meaning, "without a path") || !strings.Contains(meaning, "not re-read") {
		t.Fatalf("governed explanation did not state oracle path limitation: %q", meaning)
	}

	for _, test := range []struct {
		name string
		path string
	}{
		{"approval", ".ingen/governance/approval.json"},
		{"authority", ".ingen/governance/authority.json"},
		{"contract source", ".ingen/contract/contract.json"},
	} {
		t.Run("tampered "+test.name, func(t *testing.T) {
			root, reportPath, _ := governedEvidenceFixture(t, false)
			path := filepath.Join(root, filepath.FromSlash(test.path))
			if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Verify(root, reportPath, ""); err == nil {
				t.Fatalf("accepted modified governance input %s", test.path)
			}
		})
	}
	t.Run("missing approval references", func(t *testing.T) {
		root, reportPath, raw := governedEvidenceFixture(t, false)
		var report Report
		if err := json.Unmarshal(raw, &report); err != nil {
			t.Fatal(err)
		}
		report.Governance.ApprovalArtifacts = nil
		changed, err := json.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(reportPath)), changed, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Verify(root, reportPath, ""); err == nil {
			t.Fatal("accepted governed report without approval artifact references")
		}
	})
	t.Run("approval record ID mismatch", func(t *testing.T) {
		root, reportPath, raw := governedEvidenceFixture(t, false)
		var report Report
		if err := json.Unmarshal(raw, &report); err != nil {
			t.Fatal(err)
		}
		report.Governance.ApprovalRecordID = "different-record"
		changed, err := json.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(reportPath)), changed, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Verify(root, reportPath, ""); err == nil {
			t.Fatal("accepted a report approval record ID different from the Hammond record")
		}
	})
	t.Run("sealed source shares approved canonical path", func(t *testing.T) {
		root, reportPath, _ := governedEvidenceFixture(t, true)
		verified, err := Verify(root, reportPath, "")
		if err != nil {
			t.Fatalf("same-path sealed source and approved canonical artifact: %v", err)
		}
		source, sourceOK := findFile(verified.Files, FileContractSource, ".ingen/contract/canonical.json")
		canonical, canonicalOK := findFile(verified.Files, "governance-sorna-contract", ".ingen/contract/canonical.json")
		if !sourceOK || !canonicalOK || source.SHA256 != canonical.SHA256 || !bytes.Equal(source.Bytes, canonical.Bytes) {
			t.Fatal("same-path contract source and approved canonical artifact were not safely deduplicated by identity")
		}
	})
}

func TestIndeterminateExecutionBuildsErrorResultEvenWithZeroOrMissingExit(t *testing.T) {
	for _, exit := range []*int{intPtr(0), nil} {
		root, reportPath, raw := evidenceFixture(t)
		var report Report
		if err := json.Unmarshal(raw, &report); err != nil {
			t.Fatal(err)
		}
		report.Status, report.Reason = "indeterminate", "capture publication could not be confirmed"
		report.ExitCode = exit
		report.StdoutSHA256, report.StderrSHA256 = "", ""
		changed, err := json.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(reportPath)), changed, 0o600); err != nil {
			t.Fatal(err)
		}
		verified, err := Verify(root, reportPath, "")
		if err != nil {
			t.Fatal(err)
		}
		result, err := verified.BuildCIResult(root)
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != "error" || result.ExitCode != 2 {
			t.Fatalf("indeterminate report exit=%v mapped to %+v", exit, result)
		}
	}
}

func governedEvidenceFixture(t *testing.T, sealedSourceSamePath bool) (string, string, []byte) {
	t.Helper()
	root := t.TempDir()
	if _, err := project.Initialize(project.Options{Root: root, ID: "evidence-governed"}); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := project.WorkspacePath
	manifestBytes, err := os.ReadFile(filepath.Join(root, manifestPath))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := workspace.LoadBytes(manifestPath, manifestBytes)
	if err != nil {
		t.Fatal(err)
	}
	if sealedSourceSamePath {
		manifest.Contract.Path = ".ingen/contract/canonical.json"
		for index := range manifest.Roles {
			for readIndex, readRoot := range manifest.Roles[index].ReadRoots {
				if readRoot == ".ingen/contract/contract.json" {
					manifest.Roles[index].ReadRoots[readIndex] = manifest.Contract.Path
				}
			}
		}
	}
	manifestYAML, err := yaml.Marshal(workspace.Document{Workspace: manifest})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, manifestPath), manifestYAML, 0o600); err != nil {
		t.Fatal(err)
	}
	contractDocument := contract.Document{Contract: map[string]any{
		"schema": contract.Schema, "id": manifest.ID, "version": 1, "status": "draft",
		"interface":   map[string]any{"kind": "http"},
		"rules":       []any{map[string]any{"id": "health", "strength": "must", "subject": "GET /healthz", "expect": map[string]any{"status": 200}}},
		"unspecified": []any{},
	}}
	sealedContract, err := contract.Seal(contractDocument)
	if err != nil {
		t.Fatal(err)
	}
	contractSourceBytes, err := json.Marshal(map[string]any{"contract": contractDocument.Contract})
	if err != nil {
		t.Fatal(err)
	}
	sourcePath := ".ingen/contract/contract.json"
	canonicalPath := ".ingen/contract/canonical.json"
	if sealedSourceSamePath {
		sourcePath = canonicalPath
		contractSourceBytes = sealedContract.CanonicalJSON
	}
	writeEvidenceFile(t, root, sourcePath, contractSourceBytes)
	writeEvidenceFile(t, root, canonicalPath, sealedContract.CanonicalJSON)

	oraclePolicyBytes, err := os.ReadFile(filepath.Join(root, ".ingen/policy/oracle.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	oraclePolicyDoc, err := policy.LoadBytes(".ingen/policy/oracle.yaml", oraclePolicyBytes)
	if err != nil {
		t.Fatal(err)
	}
	sealedOraclePolicy, err := policy.Seal(oraclePolicyDoc)
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := oracle.Generate(sealedContract, sealedOraclePolicy.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	frozenBytes, err := oracle.CanonicalJSON(frozen)
	if err != nil {
		t.Fatal(err)
	}
	oraclePath := ".ingen/artifacts/oracle/frozen.json"
	writeEvidenceFile(t, root, oraclePath, frozenBytes)
	writeEvidenceFile(t, root, ".ingen/artifacts/oracle/hash.txt", []byte(hash(frozenBytes)+"\n"))

	authorityPath, reviewPath, approvalPath := ".ingen/governance/authority.json", ".ingen/governance/review-policy.json", ".ingen/governance/approval.json"
	authorityBytes, err := json.Marshal(map[string]any{"schema": "ingen.hammond-authority/v1", "id": "reviewers", "version": 1, "actors": map[string][]string{"reviewer": {"product-reviewer"}}})
	if err != nil {
		t.Fatal(err)
	}
	writeEvidenceFile(t, root, authorityPath, authorityBytes)
	reviewBytes, err := json.Marshal(map[string]any{
		"schema": "ingen.hammond-review-policy/v1", "id": "review", "version": 1, "minimum_approvals": 1,
		"authority": map[string]any{"id": "reviewers", "version": 1, "schema": "ingen.hammond-authority/v1", "artifact": map[string]string{"uri": authorityPath, "sha256": hash(authorityBytes)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeEvidenceFile(t, root, reviewPath, reviewBytes)
	approvalBytes, err := json.Marshal(map[string]any{
		"schema": "ingen.hammond-governance/v1", "record_id": "approval-test",
		"contract": map[string]any{"project_id": manifest.ID, "id": manifest.ID, "version": 1, "schema": contract.Schema, "artifact": map[string]string{"uri": canonicalPath, "sha256": sealedContract.SHA256}},
		"policy":   map[string]any{"id": "review", "version": 1, "schema": "ingen.hammond-review-policy/v1", "artifact": map[string]string{"uri": reviewPath, "sha256": hash(reviewBytes)}},
		"state":    "approved", "events": []any{
			map[string]any{"id": "registered", "type": "registered", "actor": "owner", "at": "2026-10-01T00:00:00Z"},
			map[string]any{"id": "opened", "type": "review-opened", "actor": "owner", "review_cycle_id": "cycle-1", "at": "2026-10-01T00:01:00Z"},
			map[string]any{"id": "approved", "type": "approval-recorded", "actor": "reviewer", "role": "product-reviewer", "review_cycle_id": "cycle-1", "decision": "approve", "artifact_sha256": sealedContract.SHA256, "at": "2026-10-01T00:02:00Z"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeEvidenceFile(t, root, approvalPath, approvalBytes)
	gate, err := workflowgate.Check(workflowgate.Request{Root: root, WorkspacePath: manifestPath, ApprovalPath: approvalPath, ReviewPolicyPath: reviewPath, Stage: workflowgate.StageImplementation, OraclePath: oraclePath})
	if err != nil {
		t.Fatalf("test-only workflowgate approval setup failed: %v", err)
	}
	return evidenceSnapshotInRoot(t, root, "implementation", &gate)
}

func writeEvidenceFile(t *testing.T, root, relative string, contents []byte) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
}

func fileKind(files []File, kind string) (File, bool) {
	for _, file := range files {
		if file.Kind == kind {
			return file, true
		}
	}
	return File{}, false
}

func evidenceFixture(t *testing.T) (string, string, []byte) {
	return evidenceFixtureFor(t, "evidence-test", "contract-author", nil)
}

func evidenceFixtureFor(t *testing.T, projectID, roleID string, gate *workflowgate.Result) (string, string, []byte) {
	t.Helper()
	root := t.TempDir()
	if _, err := project.Initialize(project.Options{Root: root, ID: projectID}); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	return evidenceSnapshotInRoot(t, root, roleID, gate)
}

func evidenceSnapshotInRoot(t *testing.T, root, roleID string, gate *workflowgate.Result) (string, string, []byte) {
	t.Helper()
	manifestBytes, err := os.ReadFile(filepath.Join(root, project.WorkspacePath))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := workspace.LoadBytes(project.WorkspacePath, manifestBytes)
	if err != nil {
		t.Fatal(err)
	}
	var role workspace.Role
	for _, candidate := range manifest.Roles {
		if candidate.ID == roleID {
			role = candidate
			break
		}
	}
	if role.ID == "" {
		t.Fatalf("fixture role %q not found", roleID)
	}
	const executionID = "evidence-case"
	reportPath := ".ingen/artifacts/role-executions/" + executionID + ".json"
	policyPath := ".ingen/artifacts/role-executions/" + executionID + ".policy.json"
	stdoutPath := ".ingen/artifacts/role-executions/" + executionID + ".stdout"
	stderrPath := ".ingen/artifacts/role-executions/" + executionID + ".stderr"
	stdout, stderr := []byte("ok\n"), []byte{}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, reportPath)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, stdoutPath), stdout, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, stderrPath), stderr, 0o600); err != nil {
		t.Fatal(err)
	}
	scratch := filepath.Join(root, filepath.FromSlash(role.Workspace), ".sentinel-roleexec", executionID)
	reads := absRoots(root, role.ReadRoots)
	writes := absRoots(root, role.WriteRoots)
	denies := absRoots(root, role.DenyRoots)
	derivedWrites := []string{filepath.Join(scratch, "rw")}
	entries := func(paths []string, reason string) []any {
		result := make([]any, 0, len(paths))
		for _, path := range paths {
			result = append(result, map[string]any{"path": path, "reason": reason})
		}
		return result
	}
	document := policy.Document{Policy: map[string]any{
		"schema": policy.Schema, "id": "sentinel-role-" + executionID, "version": 1, "status": "draft",
		"purpose": "noninteractive Sentinel role execution", "enforcement": "host-enforced",
		"filesystem": map[string]any{"read": entries(append(reads, scratch), "manifest read capability or derived role workspace"), "write": entries(append(writes, derivedWrites...), "manifest write capability or private execution scratch"), "deny": entries(denies, "manifest deny capability")},
		"network":    map[string]any{"mode": "disabled"},
		"process":    map[string]any{"subject_id": manifest.ID + "/" + role.ID, "can_invoke_subject": false, "allowed_tools": []any{}},
	}}
	sealed, err := policy.Seal(document)
	if err != nil {
		t.Fatal(err)
	}
	policyBytes, err := policy.CanonicalJSON(sealed.Document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, policyPath), policyBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	started := time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano)
	limitations := []string{"executable digest is a prelaunch byte check, not running-image identity", "Darwin runtime bootstrap and ancestor metadata are available", "no process namespace", "no control over prior role context", "no host attestation", "noninteractive execution only"}
	report := Report{
		Schema: "ingen.sentinel-role-execution/v1", ExecutionID: executionID, WorkspaceID: manifest.ID, RoleID: role.ID, RoleKind: role.Kind,
		ManifestSHA256: hash(manifestBytes), PolicySHA256: hash(policyBytes), WorkspaceManifestPath: project.WorkspacePath, PolicyPath: policyPath,
		ExecutablePath: "/usr/bin/true", ExecutableSHA256: strings.Repeat("a", 64), AllowedTools: []string{}, AllowedToolSHA256: []string{},
		Backend: "macos-seatbelt", Enforcement: "host-enforced", Assurance: "unverified", EnforcementScope: "filesystem, tool, and network rules", Limitations: limitations, NetworkMode: "disabled",
		DeclaredReadRoots: absRoots(root, role.ReadRoots), DeclaredWriteRoots: absRoots(root, role.WriteRoots), DeclaredDenyRoots: denies,
		DerivedReadRoots: []string{scratch}, DerivedWriteRoots: derivedWrites,
		StdoutPath: stdoutPath, StdoutSHA256: hash(stdout), StderrPath: stderrPath, StderrSHA256: hash(stderr),
		StartedAt: started, FinishedAt: now, Status: "completed", ExitCode: intPtr(0),
		Governance: gate,
	}
	reportBytes, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, reportPath), reportBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	return root, reportPath, reportBytes
}

func absRoots(root string, paths []string) []string {
	result := make([]string, len(paths))
	for i, path := range paths {
		result[i] = filepath.Join(root, filepath.FromSlash(path))
	}
	return result
}

func hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func intPtr(value int) *int { return &value }

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
