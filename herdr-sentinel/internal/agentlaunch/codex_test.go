package agentlaunch

import (
	"strings"
	"testing"
)

func TestBuildCodexCreatesOnlyFreshEphemeralArgv(t *testing.T) {
	request := CodexRequest{
		ExecutionID: "execution-1", Root: "/project", WorkingDir: "/project/.ingen/artifacts/role-executions/.runtime/execution-1",
		HomePath:         "/project/.ingen/artifacts/role-executions/.runtime/execution-1/rw/home",
		CodexHome:        "/project/.ingen/artifacts/role-executions/.runtime/execution-1/rw/codex-home",
		PromptSourcePath: ".ingen/prompts/implementation.txt", PromptSourceSHA256: strings.Repeat("a", 64),
		PromptSnapshotPath: ".ingen/artifacts/role-executions/execution-1.prompt", PromptSnapshotSHA256: strings.Repeat("a", 64),
		PromptBytes: 24, Model: "gpt-5.1-codex",
	}
	context, args, err := BuildCodex(request)
	if err != nil {
		t.Fatal(err)
	}
	wantPrefix := []string{
		"--no-daemon", "exec", "--ephemeral", "--ignore-user-config", "--ignore-rules",
		"--skip-git-repo-check", "--json", "--color", "never",
		"-c", "memories.use_memories=false", "-c", "memories.generate_memories=false", "-c", "project_doc_max_bytes=0",
		"-m", "gpt-5.1-codex", "-",
	}
	if len(args) != len(wantPrefix) {
		t.Fatalf("Codex argv = %q, want exact fixed profile %q", args, wantPrefix)
	}
	for index := range wantPrefix {
		if args[index] != wantPrefix[index] {
			t.Fatalf("Codex argv[%d] = %q, want %q", index, args[index], wantPrefix[index])
		}
	}
	if context.Agent != "codex-cli" || context.Mode != "exec-ephemeral" || context.Stdin != StdinPromptSource || context.NetworkMode != "disabled" || context.PromptSourceSHA256 != context.PromptSnapshotSHA256 {
		t.Fatalf("Codex context = %+v", context)
	}
	for _, arg := range args {
		if arg == "resume" || arg == "fork" || arg == "--resume" || arg == "--profile" || arg == "--worktree" {
			t.Fatalf("fresh profile contains context adoption option %q", arg)
		}
	}
}

func TestValidatePromptPreservesExactBoundedUTF8(t *testing.T) {
	if err := ValidatePrompt([]byte("Implement the requested behavior.\n\nKeep evidence intact. \n")); err != nil {
		t.Fatalf("valid newline-preserving prompt rejected: %v", err)
	}
	for _, prompt := range [][]byte{nil, []byte("  \n"), []byte("bad\x00prompt"), {0xff}, make([]byte, MaxPromptBytes+1)} {
		if err := ValidatePrompt(prompt); err == nil {
			t.Fatalf("accepted invalid prompt of %d bytes", len(prompt))
		}
	}
}

func TestBuildCodexRejectsMutableOrUnsafeLaunchInputs(t *testing.T) {
	base := CodexRequest{
		ExecutionID: "execution-1", Root: "/project", WorkingDir: "/project/scratch",
		HomePath: "/project/scratch/home", CodexHome: "/project/scratch/codex",
		PromptSourcePath: "prompt.txt", PromptSourceSHA256: strings.Repeat("a", 64),
		PromptSnapshotPath: ".ingen/artifacts/role-executions/execution-1.prompt", PromptSnapshotSHA256: strings.Repeat("a", 64), PromptBytes: 10,
	}
	cases := []struct {
		name   string
		mutate func(*CodexRequest)
	}{
		{name: "digest drift", mutate: func(request *CodexRequest) { request.PromptSnapshotSHA256 = strings.Repeat("b", 64) }},
		{name: "path traversal", mutate: func(request *CodexRequest) { request.PromptSourcePath = "../prompt.txt" }},
		{name: "option injection model", mutate: func(request *CodexRequest) { request.Model = "--profile=old" }},
		{name: "invalid runtime root", mutate: func(request *CodexRequest) { request.CodexHome = "/project/../old-home" }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			request := base
			test.mutate(&request)
			if _, _, err := BuildCodex(request); err == nil {
				t.Fatal("accepted unsafe or unbound Codex launch input")
			}
		})
	}
}

func TestBuildCodexBrokerProfilePinsOnlyLocalResponsesEndpoint(t *testing.T) {
	base := CodexRequest{
		ExecutionID: "execution-1", Root: "/project", WorkingDir: "/project/scratch",
		HomePath: "/project/scratch/home", CodexHome: "/project/scratch/codex",
		PromptSourcePath: "prompt.txt", PromptSourceSHA256: strings.Repeat("a", 64),
		PromptSnapshotPath: ".ingen/artifacts/role-executions/execution-1.prompt", PromptSnapshotSHA256: strings.Repeat("a", 64), PromptBytes: 10,
		Model: "gpt-5-codex", Broker: &BrokerContext{Provider: "openai-responses", Endpoint: "http://127.0.0.1:49123/v1", CredentialSource: "environment", CredentialEnv: "OPENAI_API_KEY", MaxRequests: 16, MaxOutputTokens: 4096, TimeoutSeconds: 300},
	}
	context, args, err := BuildCodex(base)
	if err != nil {
		t.Fatal(err)
	}
	if context.NetworkMode != "allowlist" || context.Broker == nil || context.Broker.Endpoint != base.Broker.Endpoint {
		t.Fatalf("broker context = %+v", context)
	}
	joined := strings.Join(args, "\n")
	for _, want := range []string{"model_provider=ingen_broker", "model_providers.ingen_broker.env_key=INGEN_CODEX_BROKER_TOKEN", "model_providers.ingen_broker.wire_api=responses", "model_providers.ingen_broker.request_max_retries=0", "web_search=disabled", "approval_policy=never", "sandbox_mode=danger-full-access", "-"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("broker argv missing %q: %q", want, args)
		}
	}
	for _, secret := range []string{"secret-value", "OPENAI_API_KEY="} {
		if strings.Contains(joined, secret) {
			t.Fatalf("broker argv contains credential material %q", secret)
		}
	}
	for _, endpoint := range []string{"http://localhost:49123/v1", "http://127.0.0.1:0/v1", "http://127.0.0.1:49123/v1/evil"} {
		request := base
		broker := *base.Broker
		broker.Endpoint = endpoint
		request.Broker = &broker
		if _, _, err := BuildCodex(request); err == nil {
			t.Fatalf("accepted endpoint %q", endpoint)
		}
	}
}
