package nativesession

import (
	"path/filepath"
	"testing"

	"ingen/core/ciresult"
	"ingen/herdr-sentinel/evidence"
	"ingen/herdr-sentinel/internal/agentlaunch"
	"ingen/herdr-sentinel/internal/nativejournal"
	"ingen/herdr-sentinel/internal/roleexec"
)

func TestParseRoleExecutionIntentStopsAtFirstCommandDelimiter(t *testing.T) {
	intent := nativejournal.Intent{
		SentinelExecutable: "/usr/local/bin/sentinel",
		ReceiptPath:        ".ingen/artifacts/sentinel-run.json",
		RoleID:             "implementation",
		SessionID:          "session-1",
		Argv: []string{
			"/usr/local/bin/sentinel", "role", "execute",
			"--root", "/tmp/project",
			"--workspace", ".ingen/workspace.yaml",
			"--receipt", ".ingen/artifacts/sentinel-run.json",
			"--role", "implementation",
			"--execution-id", "execution-1",
			"--policy-path", ".ingen/artifacts/role-executions/execution-1.policy.json",
			"--expected-manifest-sha256", testRoleDigest("manifest"),
			"--expected-policy-sha256", testRoleDigest("policy"),
			"--expected-executable-sha256", testRoleDigest("executable"),
			"--", "/bin/sh", "-c", "printf -- --help",
		},
	}
	binding, recognized, err := parseRoleExecutionIntent(intent)
	if err != nil || !recognized || binding.ExecutionID != "execution-1" {
		t.Fatalf("parseRoleExecutionIntent() = %+v, %v, %v", binding, recognized, err)
	}
	originalArgv := append([]string(nil), intent.Argv...)
	delimiter := -1
	for index, arg := range intent.Argv[3:] {
		if arg == "--" {
			delimiter = index
			break
		}
	}
	intent.Argv = append(intent.Argv[:3+delimiter+1], "")
	if _, _, err := parseRoleExecutionIntent(intent); err == nil {
		t.Fatal("empty contained command was accepted")
	}
	intent.Argv = append(originalArgv[:3+delimiter], "--unsupported", "x", "--", "/bin/true")
	if _, _, err := parseRoleExecutionIntent(intent); err == nil {
		t.Fatal("unsupported Sentinel option was accepted")
	}
}

func TestParseTypedCodexRoleExecutionIntentHasNoCommandEscapeHatch(t *testing.T) {
	intent := nativejournal.Intent{
		SentinelExecutable: "/usr/local/bin/sentinel", ReceiptPath: ".ingen/receipt.json", RoleID: "implementation", SessionID: "session-typed", RoleKind: "implementation",
		Argv: []string{
			"/usr/local/bin/sentinel", "role", "execute",
			"--root", "/tmp/project", "--workspace", ".ingen/workspace.yaml", "--receipt", ".ingen/receipt.json",
			"--role", "implementation", "--execution-id", "execution-typed", "--policy-path", ".ingen/artifacts/role-executions/execution-typed.policy.json",
			"--expected-manifest-sha256", testRoleDigest("manifest"), "--expected-policy-sha256", testRoleDigest("policy"),
			"--expected-executable-sha256", testRoleDigest("codex"), "--agent", "codex", "--agent-executable", "/opt/codex/bin/codex",
			"--prompt-file", ".ingen/prompts/task.txt", "--expected-prompt-sha256", testRoleDigest("prompt"), "--agent-model", "codex-test",
		},
	}
	binding, recognized, err := parseRoleExecutionIntent(intent)
	if err != nil || !recognized || binding.Agent != "codex" || binding.PromptPath != ".ingen/prompts/task.txt" || binding.AgentModel != "codex-test" {
		t.Fatalf("typed Codex argv parse = %+v, %v, %v", binding, recognized, err)
	}
	intent.Argv = append(intent.Argv, "--", "/bin/true")
	if _, _, err := parseRoleExecutionIntent(intent); err == nil {
		t.Fatal("typed Codex argv accepted an arbitrary command delimiter and command")
	}
}

func TestParseTypedCodexBrokerIntentRequiresFixedProviderAndBoundedUniqueProfile(t *testing.T) {
	args := []string{
		"/usr/local/bin/sentinel", "role", "execute",
		"--root", "/tmp/project", "--workspace", ".ingen/workspace.yaml", "--receipt", ".ingen/receipt.json",
		"--role", "implementation", "--execution-id", "execution-broker", "--policy-path", ".ingen/artifacts/role-executions/execution-broker.policy.json",
		"--expected-manifest-sha256", testRoleDigest("manifest"), "--expected-policy-sha256", testRoleDigest("policy"),
		"--expected-executable-sha256", testRoleDigest("codex"), "--agent", "codex", "--agent-executable", "/opt/codex/bin/codex",
		"--prompt-file", ".ingen/prompts/task.txt", "--expected-prompt-sha256", testRoleDigest("prompt"), "--agent-model", "gpt-test",
		"--agent-provider", "openai-broker", "--agent-credential-env", "OPENAI_API_KEY", "--agent-broker-port", "34567",
		"--agent-max-requests", "16", "--agent-max-output-tokens", "4096", "--agent-timeout-seconds", "300",
	}
	intent := nativejournal.Intent{SentinelExecutable: args[0], ReceiptPath: ".ingen/receipt.json", RoleID: "implementation", RoleKind: "implementation", Argv: append([]string(nil), args...)}
	binding, recognized, err := parseRoleExecutionIntent(intent)
	if err != nil || !recognized || !binding.AgentBrokerProfile || binding.AgentProvider != "openai-broker" || binding.AgentBrokerPort != 34567 || binding.AgentMaxRequests != 16 || binding.AgentMaxOutputTokens != 4096 || binding.AgentTimeoutSeconds != 300 || binding.AgentCredentialEnv != "OPENAI_API_KEY" {
		t.Fatalf("broker argv parse = %+v, %v, %v", binding, recognized, err)
	}
	for name, replacement := range map[string]string{
		"--agent-provider":          "openai-completions",
		"--agent-credential-env":    "INGEN_CODEX_BROKER_TOKEN",
		"--agent-broker-port":       "034567",
		"--agent-max-requests":      "65",
		"--agent-max-output-tokens": "8193",
		"--agent-timeout-seconds":   "1801",
	} {
		mutated := append([]string(nil), args...)
		for index, arg := range mutated[:len(mutated)-1] {
			if arg == name {
				mutated[index+1] = replacement
				break
			}
		}
		intent.Argv = mutated
		if _, _, err := parseRoleExecutionIntent(intent); err == nil {
			t.Errorf("accepted %s=%q", name, replacement)
		}
	}
	intent.Argv = append(append([]string(nil), args...), "--agent-provider", "openai-broker")
	if _, _, err := parseRoleExecutionIntent(intent); err == nil {
		t.Fatal("accepted duplicated broker provider option")
	}
	intent.Argv = append([]string(nil), args[:len(args)-2]...)
	if _, _, err := parseRoleExecutionIntent(intent); err == nil {
		t.Fatal("accepted incomplete broker profile")
	}
}

func TestVerifyRoleAgentContextMatchesOnlyGeneratedFreshProfile(t *testing.T) {
	const executionID = "exec-context"
	root := "/tmp/project"
	promptPath := ".ingen/prompts/task.txt"
	promptSHA := testRoleDigest("prompt bytes")
	snapshotPath := ".ingen/artifacts/role-executions/exec-context.prompt"
	context, args, err := agentlaunch.BuildCodex(agentlaunch.CodexRequest{
		ExecutionID: executionID, Root: root, WorkingDir: filepath.Join(root, ".ingen/artifacts/role-executions/.runtime", executionID),
		HomePath:         filepath.Join(root, ".ingen/artifacts/role-executions/.runtime", executionID, "rw/home"),
		CodexHome:        filepath.Join(root, ".ingen/artifacts/role-executions/.runtime", executionID, "rw/codex-home"),
		PromptSourcePath: promptPath, PromptSourceSHA256: promptSHA, PromptSnapshotPath: snapshotPath, PromptSnapshotSHA256: promptSHA, PromptBytes: len("prompt bytes"), Model: "codex-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	context.Args = args
	binding := roleExecutionBinding{Agent: "codex", AgentExecutable: "/opt/codex/bin/codex", PromptPath: promptPath, PromptSHA256: promptSHA, AgentModel: "codex-test", ExecutionID: executionID}
	report := evidence.Report{ExecutablePath: binding.AgentExecutable, AgentContext: &context}
	if err := verifyRoleAgentContext(root, binding, report); err != nil {
		t.Fatalf("valid fixed Codex context rejected: %v", err)
	}
	context.Args = append(context.Args, "--resume")
	if err := verifyRoleAgentContext(root, binding, report); err == nil {
		t.Fatal("accepted Codex context containing an extra resume argument")
	}
}

func TestVerifyRoleAgentContextBindsBrokerReportToNativeProfile(t *testing.T) {
	const executionID = "exec-broker-context"
	root := "/tmp/project"
	broker := &agentlaunch.BrokerContext{
		Provider: "openai-responses", Endpoint: "http://127.0.0.1:34567/v1", CredentialSource: "environment", CredentialEnv: "OPENAI_API_KEY",
		MaxRequests: 16, MaxOutputTokens: 4096, TimeoutSeconds: 300,
	}
	context, args, err := agentlaunch.BuildCodex(agentlaunch.CodexRequest{
		ExecutionID: executionID, Root: root, WorkingDir: filepath.Join(root, ".ingen/runtime", executionID),
		HomePath: filepath.Join(root, ".ingen/runtime", executionID, "rw/home"), CodexHome: filepath.Join(root, ".ingen/runtime", executionID, "rw/codex-home"),
		PromptSourcePath: ".ingen/prompts/task.txt", PromptSourceSHA256: testRoleDigest("prompt"),
		PromptSnapshotPath: ".ingen/artifacts/role-executions/exec-broker-context.prompt", PromptSnapshotSHA256: testRoleDigest("prompt"),
		PromptBytes: 12, Model: "gpt-test", Broker: broker,
	})
	if err != nil {
		t.Fatal(err)
	}
	context.Args = args
	binding := roleExecutionBinding{
		ExecutionID: executionID, Agent: "codex", AgentExecutable: "/opt/codex/bin/codex", PromptPath: context.PromptSourcePath,
		PromptSHA256: context.PromptSourceSHA256, AgentModel: context.Model, AgentBrokerProfile: true,
		AgentProvider: "openai-broker", AgentCredentialEnv: "OPENAI_API_KEY", AgentBrokerPort: 34567,
		AgentMaxRequests: 16, AgentMaxOutputTokens: 4096, AgentTimeoutSeconds: 300,
	}
	report := evidence.Report{
		ExecutablePath: binding.AgentExecutable, AgentContext: &context,
		BrokerExecution: &roleexec.BrokerExecution{Status: "unavailable", Provider: broker.Provider, Endpoint: broker.Endpoint, Model: context.Model, MaxRequests: broker.MaxRequests, MaxOutputTokens: broker.MaxOutputTokens, TimeoutSeconds: broker.TimeoutSeconds},
	}
	if err := verifyRoleAgentContext(root, binding, report); err != nil {
		t.Fatalf("valid broker context rejected: %v (binding=%+v context=%+v executable=%q root=%q)", err, binding, context, report.ExecutablePath, root)
	}
	context.Broker.Endpoint = "http://localhost:34567/v1"
	report.AgentContext = &context
	if err := verifyRoleAgentContext(root, binding, report); err == nil {
		t.Fatal("accepted report broker endpoint that differs from immutable native loopback port")
	}
	context.Broker.Endpoint = broker.Endpoint
	context.Args = append(context.Args, "--resume")
	report.AgentContext = &context
	if err := verifyRoleAgentContext(root, binding, report); err == nil {
		t.Fatal("accepted forged Codex arguments in broker mode")
	}
}

func TestRoleExecutionFilesAttachesAllVerifiedNonManifestEvidence(t *testing.T) {
	manifest := ciresult.FileRef{Path: ".ingen/workspace.yaml", SHA256: testRoleDigest("manifest")}
	verified := evidence.Verified{
		Report: evidence.Report{
			ExecutionID: "execution-1", PolicyPath: ".ingen/artifacts/role-executions/execution-1.policy.json",
			StdoutPath: ".ingen/artifacts/role-executions/execution-1.stdout", StderrPath: ".ingen/artifacts/role-executions/execution-1.stderr",
			StdoutSHA256: testRoleDigest("stdout"), StderrSHA256: testRoleDigest("stderr"),
		},
		ReportSHA256: testRoleDigest("report"),
		Files: []evidence.File{
			{Kind: evidence.FileWorkspaceManifest, Path: manifest.Path, SHA256: manifest.SHA256, Bytes: []byte("manifest")},
			{Kind: evidence.FileSealedPolicy, Path: ".ingen/artifacts/role-executions/execution-1.policy.json", SHA256: testRoleDigest("policy"), Bytes: []byte("policy")},
			{Kind: evidence.FileStdout, Path: ".ingen/artifacts/role-executions/execution-1.stdout", SHA256: testRoleDigest("stdout"), Bytes: []byte("stdout")},
			{Kind: evidence.FileStderr, Path: ".ingen/artifacts/role-executions/execution-1.stderr", SHA256: testRoleDigest("stderr"), Bytes: []byte("stderr")},
			{Kind: evidence.FileContractSource, Path: ".ingen/contract/contract.yaml", SHA256: testRoleDigest("contract"), Bytes: []byte("contract")},
			{Kind: "governance-hammond-approval", Path: ".ingen/governance/approval.json", SHA256: testRoleDigest("approval"), Bytes: []byte("approval")},
		},
	}
	artifacts, err := roleExecutionFiles(verified, "session-1", manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 6 {
		t.Fatalf("attached %d artifacts, want report plus every non-manifest file", len(artifacts))
	}
	if artifacts[0].Path != filepath.ToSlash(filepath.Join(".ingen", "artifacts", "role-executions", "execution-1.json")) {
		t.Fatalf("report path = %q", artifacts[0].Path)
	}
	for _, artifact := range artifacts {
		if artifact.ID == "" || artifact.SHA256 == "" {
			t.Fatalf("invalid artifact identity: %+v", artifact)
		}
	}
}

func TestRoleExecutionFilesAllowsAbsentIndeterminateCaptures(t *testing.T) {
	manifest := ciresult.FileRef{Path: ".ingen/workspace.yaml", SHA256: testRoleDigest("manifest")}
	verified := evidence.Verified{
		Report: evidence.Report{
			ExecutionID: "execution-1", PolicyPath: ".ingen/artifacts/role-executions/execution-1.policy.json",
			StdoutPath: ".ingen/artifacts/role-executions/execution-1.stdout", StderrPath: ".ingen/artifacts/role-executions/execution-1.stderr",
		},
		ReportSHA256: testRoleDigest("report"),
		Files: []evidence.File{
			{Kind: evidence.FileWorkspaceManifest, Path: manifest.Path, SHA256: manifest.SHA256, Bytes: []byte("manifest")},
			{Kind: evidence.FileSealedPolicy, Path: ".ingen/artifacts/role-executions/execution-1.policy.json", SHA256: testRoleDigest("policy"), Bytes: []byte("policy")},
		},
	}
	if _, err := roleExecutionFiles(verified, "session-1", manifest); err != nil {
		t.Fatalf("absent capture refs rejected: %v", err)
	}
	verified.Files = append(verified.Files, evidence.File{Kind: evidence.FileStdout, Path: ".ingen/artifacts/role-executions/execution-1.stdout", SHA256: testRoleDigest("unclaimed capture"), Bytes: []byte("unclaimed capture")})
	artifacts, err := roleExecutionFiles(verified, "session-1", manifest)
	if err != nil {
		t.Fatalf("unpinned indeterminate capture rejected: %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("attached %d artifacts for an unpinned capture, want report and policy only", len(artifacts))
	}
}

func TestRoleExecutionFilesAttachesPromptSourceAndSnapshot(t *testing.T) {
	manifest := ciresult.FileRef{Path: ".ingen/workspace.yaml", SHA256: testRoleDigest("manifest")}
	context := &agentlaunch.CodexContext{
		ExecutionID: "execution-1", PromptSourcePath: ".ingen/prompts/task.txt", PromptSourceSHA256: testRoleDigest("prompt"),
		PromptSnapshotPath: ".ingen/artifacts/role-executions/execution-1.prompt", PromptSnapshotSHA256: testRoleDigest("prompt"),
	}
	verified := evidence.Verified{
		Report: evidence.Report{
			ExecutionID: "execution-1", PolicyPath: ".ingen/artifacts/role-executions/execution-1.policy.json",
			StdoutPath: ".ingen/artifacts/role-executions/execution-1.stdout", StderrPath: ".ingen/artifacts/role-executions/execution-1.stderr",
			StdoutSHA256: testRoleDigest("stdout"), StderrSHA256: testRoleDigest("stderr"), AgentContext: context,
		},
		ReportSHA256: testRoleDigest("report"),
		Files: []evidence.File{
			{Kind: evidence.FileWorkspaceManifest, Path: manifest.Path, SHA256: manifest.SHA256, Bytes: []byte("manifest")},
			{Kind: evidence.FileSealedPolicy, Path: ".ingen/artifacts/role-executions/execution-1.policy.json", SHA256: testRoleDigest("policy"), Bytes: []byte("policy")},
			{Kind: evidence.FileStdout, Path: ".ingen/artifacts/role-executions/execution-1.stdout", SHA256: testRoleDigest("stdout"), Bytes: []byte("stdout")},
			{Kind: evidence.FileStderr, Path: ".ingen/artifacts/role-executions/execution-1.stderr", SHA256: testRoleDigest("stderr"), Bytes: []byte("stderr")},
			{Kind: evidence.FileAgentPromptSource, Path: context.PromptSourcePath, SHA256: context.PromptSourceSHA256, Bytes: []byte("prompt")},
			{Kind: evidence.FileAgentPrompt, Path: context.PromptSnapshotPath, SHA256: context.PromptSnapshotSHA256, Bytes: []byte("prompt")},
		},
	}
	artifacts, err := roleExecutionFiles(verified, "session-1", manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 6 {
		t.Fatalf("attached %d artifacts, want report, policy, captures, and both prompts", len(artifacts))
	}
	verified.Files = verified.Files[:len(verified.Files)-1]
	if _, err := roleExecutionFiles(verified, "session-1", manifest); err == nil {
		t.Fatal("accepted typed Codex evidence without its prompt snapshot")
	}
}

func testRoleDigest(value string) string { return roleDigest([]byte(value)) }
