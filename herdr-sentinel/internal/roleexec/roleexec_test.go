package roleexec

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"ingen/herdr-sentinel/internal/agentlaunch"
	"ingen/herdr-sentinel/internal/capability"
	"ingen/herdr-sentinel/internal/project"
	"ingen/herdr-sentinel/internal/workspace"
)

func TestPrepareKeepsManifestWritesWriteOnlyAndRejectsDrift(t *testing.T) {
	root := testProject(t)
	plan, err := capability.FromFileUnderRoot(root, project.WorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	request := Request{Root: root, WorkspacePath: project.WorkspacePath, RoleID: "implementation", ExecutionID: "write-only-test", Command: []string{"/usr/bin/true"}, ExpectedManifestSHA256: plan.Workspace.Manifest.SHA256}
	prepared, err := Prepare(request)
	if err != nil {
		t.Fatal(err)
	}
	reads := policyPaths(prepared.Policy.Document.Policy, "read")
	writes := policyPaths(prepared.Policy.Document.Policy, "write")
	if containsPath(reads, filepath.Join(prepared.Root, "src")) {
		t.Fatal("manifest write root was silently promoted to read capability")
	}
	if !containsPath(writes, filepath.Join(prepared.Root, "src")) {
		t.Fatal("manifest implementation write root was omitted")
	}
	if len(prepared.DerivedReadRoots) != 1 || prepared.DerivedReadRoots[0] != prepared.ScratchRoot {
		t.Fatalf("derived reads = %v, expected only execution scratch", prepared.DerivedReadRoots)
	}
	if len(prepared.DerivedWriteRoots) != 1 || !strings.HasPrefix(prepared.DerivedWriteRoots[0], prepared.ScratchRoot+string(filepath.Separator)) {
		t.Fatalf("derived writes = %v, expected private scratch subtree", prepared.DerivedWriteRoots)
	}
	if _, err := os.Stat(prepared.ScratchRoot); !os.IsNotExist(err) {
		t.Fatalf("Prepare created scratch prematurely: stat err=%v", err)
	}
	request.ExpectedManifestSHA256 = strings.Repeat("0", 64)
	if _, err := Prepare(request); err == nil {
		t.Fatal("Prepare accepted a drifted workspace manifest hash")
	}
}

func TestPrepareBuildsFreshCodexInvocationFromPinnedReadablePrompt(t *testing.T) {
	root := testProject(t)
	promptPath := ".ingen/prompts/implementation.txt"
	prompt := []byte("Inspect only the approved task. Preserve this exact line.\n")
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, promptPath)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, promptPath), prompt, 0o600); err != nil {
		t.Fatal(err)
	}
	manifestFile := filepath.Join(root, project.WorkspacePath)
	manifestBytes, err := os.ReadFile(manifestFile)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := workspace.LoadBytes(project.WorkspacePath, manifestBytes)
	if err != nil {
		t.Fatal(err)
	}
	for i := range manifest.Roles {
		if manifest.Roles[i].ID == "implementation" {
			manifest.Roles[i].ReadRoots = append(manifest.Roles[i].ReadRoots, filepath.ToSlash(filepath.Dir(promptPath)))
		}
	}
	changed, err := yaml.Marshal(workspace.Document{Workspace: manifest})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestFile, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := capability.FromFileUnderRoot(root, project.WorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	_, promptSHA, err := PromptIdentity(root, promptPath)
	if err != nil {
		t.Fatal(err)
	}
	request := Request{
		Root: root, WorkspacePath: project.WorkspacePath, RoleID: "implementation", ExecutionID: "codex-fresh-test",
		ExpectedManifestSHA256: plan.Workspace.Manifest.SHA256,
		Agent:                  &AgentRequest{Name: "codex", ExecutablePath: "/usr/bin/true", PromptPath: promptPath, ExpectedPromptSHA256: promptSHA, Model: "codex-test-model"},
	}
	prepared, err := Prepare(request)
	if err != nil {
		t.Fatal(err)
	}
	ctx := prepared.AgentContext
	if ctx == nil || ctx.Agent != "codex-cli" || ctx.Mode != "exec-ephemeral" || ctx.NetworkMode != "disabled" || ctx.PromptBytes != len(prompt) || ctx.PromptSourceSHA256 != promptSHA || ctx.PromptSnapshotSHA256 != promptSHA {
		t.Fatalf("unexpected typed Codex context: %#v", ctx)
	}
	wantScratch := filepath.Join(prepared.Root, ".ingen", "artifacts", "role-executions", ".runtime", request.ExecutionID)
	if prepared.ScratchRoot != wantScratch || !strings.HasPrefix(ctx.HomePath, wantScratch+string(filepath.Separator)) || !strings.HasPrefix(ctx.CodexHomePath, wantScratch+string(filepath.Separator)) {
		t.Fatalf("Codex scratch/home paths escape unique runtime subtree: %#v", ctx)
	}
	policyReads := policyPaths(prepared.Policy.Document.Policy, "read")
	if containsPath(policyReads, filepath.Join(prepared.Root, ".ingen", "artifacts", "role-executions", ".runtime")) || !containsPath(policyReads, wantScratch) {
		t.Fatalf("Codex policy must read only its unique runtime subtree, not sibling sessions: %v", policyReads)
	}
	want := []string{"--no-daemon", "exec", "--ephemeral", "--ignore-user-config", "--ignore-rules", "--skip-git-repo-check", "--json", "--color", "never", "-c", "memories.use_memories=false", "-c", "memories.generate_memories=false", "-c", "project_doc_max_bytes=0", "-m", "codex-test-model", "-"}
	fullCommand := append([]string{"/usr/bin/true"}, want...)
	if !equalStrings(ctx.Args, want) || len(prepared.Command.Command) < len(fullCommand) || !equalStrings(prepared.Command.Command[len(prepared.Command.Command)-len(fullCommand):], fullCommand) || prepared.Command.ExecutablePath != "/usr/bin/true" {
		t.Fatalf("typed Codex argv = %q, want exact fixed arguments %q", prepared.Command.Command, append([]string{"/usr/bin/true"}, want...))
	}
	if _, err := os.Stat(prepared.ScratchRoot); !os.IsNotExist(err) {
		t.Fatalf("Prepare created Codex runtime directory before execution: %v", err)
	}
	env := privateEnvironment(prepared)
	for _, forbidden := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "CODEX_HOME=" + ctx.CodexHomePath, "HOME=" + ctx.HomePath} {
		if strings.HasPrefix(forbidden, "OPENAI_") || strings.HasPrefix(forbidden, "ANTHROPIC_") {
			for _, item := range env {
				if strings.HasPrefix(item, forbidden+"=") {
					t.Fatalf("private agent environment inherited credential variable %q", forbidden)
				}
			}
		} else if !containsPath(env, forbidden) {
			t.Fatalf("private Codex environment omits %q: %v", forbidden, env)
		}
	}
}

func TestBrokerPolicyAndChildEnvironmentAreNarrow(t *testing.T) {
	plan := capability.Plan{Workspace: capability.WorkspaceRef{ID: "workspace-test"}}
	role := capability.Role{ID: "implementation"}
	document := makePolicy(plan, role, "exec-broker", nil, nil, nil, nil, "allowlist", 49123)
	network := document.Policy["network"].(map[string]any)
	if network["mode"] != "allowlist" {
		t.Fatalf("network mode = %#v", network)
	}
	rules := network["allow"].([]any)
	if len(rules) != 1 {
		t.Fatalf("network rules = %#v", rules)
	}
	rule := rules[0].(map[string]any)
	if rule["host"] != "localhost" || rule["direction"] != "outbound" || rule["purpose"] != "local Codex Responses broker" || !equalInts(rule["ports"], 49123) {
		t.Fatalf("unexpected broker network rule: %#v", rule)
	}
	prepared := Prepared{ScratchRoot: "/project/scratch", AgentContext: &agentlaunch.CodexContext{}}
	env := privateEnvironmentWithBrokerToken(prepared, "scoped-test-token")
	if !containsEnv(env, "INGEN_CODEX_BROKER_TOKEN=scoped-test-token") || containsEnvPrefix(env, "OPENAI_API_KEY=") || containsEnvPrefix(env, "ANTHROPIC_API_KEY=") {
		t.Fatalf("broker child environment did not expose only scoped token: %v", env)
	}
}

func equalInts(raw any, want int) bool {
	values, ok := raw.([]any)
	return ok && len(values) == 1 && values[0] == want
}

func containsEnv(env []string, value string) bool {
	for _, item := range env {
		if item == value {
			return true
		}
	}
	return false
}

func containsEnvPrefix(env []string, prefix string) bool {
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			return true
		}
	}
	return false
}

func TestPrepareRejectsUnpinnedOrWritableCodexPrompt(t *testing.T) {
	root := testProject(t)
	promptPath := ".ingen/prompts/task.txt"
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, promptPath)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, promptPath), []byte("prompt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := capability.FromFileUnderRoot(root, project.WorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	_, promptSHA, err := PromptIdentity(root, promptPath)
	if err != nil {
		t.Fatal(err)
	}
	request := Request{Root: root, WorkspacePath: project.WorkspacePath, RoleID: "implementation", ExecutionID: "codex-no-read", ExpectedManifestSHA256: plan.Workspace.Manifest.SHA256, Agent: &AgentRequest{Name: "codex", ExecutablePath: "/usr/bin/true", PromptPath: promptPath, ExpectedPromptSHA256: promptSHA}}
	if _, err := Prepare(request); err == nil || !strings.Contains(err.Error(), "explicit role read root") {
		t.Fatalf("Prepare accepted a prompt not granted by role ReadRoots: %v", err)
	}
	request.Agent.ExpectedPromptSHA256 = strings.Repeat("0", 64)
	if _, err := Prepare(request); err == nil {
		t.Fatal("Prepare accepted a stale immutable prompt digest")
	}
	manifestPath := filepath.Join(root, project.WorkspacePath)
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := workspace.LoadBytes(project.WorkspacePath, manifestBytes)
	if err != nil {
		t.Fatal(err)
	}
	for i := range manifest.Roles {
		if manifest.Roles[i].ID == "implementation" {
			manifest.Roles[i].ReadRoots = append(manifest.Roles[i].ReadRoots, ".ingen/prompts")
			manifest.Roles[i].WriteRoots = append(manifest.Roles[i].WriteRoots, ".ingen/prompts")
		}
	}
	changed, err := yaml.Marshal(workspace.Document{Workspace: manifest})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err = capability.FromFileUnderRoot(root, project.WorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	request.ExpectedManifestSHA256 = plan.Workspace.Manifest.SHA256
	request.Agent.ExpectedPromptSHA256 = promptSHA
	if _, err := Prepare(request); err == nil || !strings.Contains(err.Error(), "must not be in a role write root") {
		t.Fatalf("Prepare accepted a role-writable prompt: %v", err)
	}
}

func TestPrepareRejectsProtectedOutputCapabilityAndUnpinnedTools(t *testing.T) {
	root := testProject(t)
	manifestPath := filepath.Join(root, project.WorkspacePath)
	contents, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := workspace.LoadBytes(project.WorkspacePath, contents)
	if err != nil {
		t.Fatal(err)
	}
	for index := range manifest.Roles {
		if manifest.Roles[index].ID == "implementation" {
			manifest.Roles[index].WriteRoots = []string{".ingen/artifacts/role-executions"}
		}
	}
	changed, err := yaml.Marshal(workspace.Document{Workspace: manifest})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := capability.FromFileUnderRoot(root, project.WorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(Request{Root: root, WorkspacePath: project.WorkspacePath, RoleID: "implementation", ExecutionID: "protected-path", Command: []string{"/usr/bin/true"}, ExpectedManifestSHA256: plan.Workspace.Manifest.SHA256}); err == nil {
		t.Fatal("role capability exposing report output was accepted")
	}

	root = testProject(t)
	plan, err = capability.FromFileUnderRoot(root, project.WorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	tool, digest, err := ToolIdentity("/usr/bin/true")
	if err != nil {
		t.Fatal(err)
	}
	request := Request{Root: root, WorkspacePath: project.WorkspacePath, RoleID: "implementation", ExecutionID: "tool-pin", Command: []string{"/usr/bin/true"}, AllowedTools: []string{tool}, ExpectedToolSHA256: []string{strings.Repeat("f", 64)}, ExpectedManifestSHA256: plan.Workspace.Manifest.SHA256}
	if _, err := Prepare(request); err == nil {
		t.Fatal("Prepare accepted a mismatched allowed-tool digest")
	}
	request.ExpectedToolSHA256 = []string{digest}
	if _, err := Prepare(request); err != nil {
		t.Fatalf("Prepare rejected correctly pinned tool: %v", err)
	}
	denyRoot := filepath.Join(root, ".git")
	if err := os.MkdirAll(denyRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	deniedExecutable := filepath.Join(denyRoot, "true")
	copyExecutable(t, "/usr/bin/true", deniedExecutable)
	request = Request{Root: root, WorkspacePath: project.WorkspacePath, RoleID: "implementation", ExecutionID: "denied-command", Command: []string{deniedExecutable}, ExpectedManifestSHA256: plan.Workspace.Manifest.SHA256}
	if _, err := Prepare(request); err == nil {
		t.Fatal("command executable beneath a manifest deny root was accepted")
	}
	toolPath := filepath.Join(denyRoot, "tool")
	copyExecutable(t, "/usr/bin/true", toolPath)
	toolDigest, err := hashFile(toolPath)
	if err != nil {
		t.Fatal(err)
	}
	request = Request{Root: root, WorkspacePath: project.WorkspacePath, RoleID: "implementation", ExecutionID: "denied-tool", Command: []string{"/usr/bin/true"}, AllowedTools: []string{toolPath}, ExpectedToolSHA256: []string{toolDigest}, ExpectedManifestSHA256: plan.Workspace.Manifest.SHA256}
	if _, err := Prepare(request); err == nil {
		t.Fatal("allowed tool beneath a manifest deny root was accepted")
	}
}

func TestExecuteHostPolicyDeniesUndeclaredReadWriteToolAndNetwork(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Sorna role containment is available only on macOS")
	}
	root := testProject(t)
	secret := filepath.Join(root, "src", "private.txt")
	if err := os.WriteFile(secret, []byte("source-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	outsideWrite := filepath.Join(root, "docs", "roleexec-denied-write.txt")
	if err := os.WriteFile(outsideWrite, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	listener, listenErr := net.Listen("tcp", "127.0.0.1:0")
	if listenErr == nil {
		defer listener.Close()
		go func() {
			for {
				conn, acceptErr := listener.Accept()
				if acceptErr != nil {
					return
				}
				_, _ = fmt.Fprint(conn, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok")
				_ = conn.Close()
			}
		}()
	}
	t.Setenv("API_TOKEN", "must-not-leak")
	tool, toolHash, err := ToolIdentity("/usr/bin/curl")
	if err != nil {
		t.Fatal(err)
	}
	script := `if IFS= read -r line < "$1"; then printf 'READ:%s\\n' "$line"; else echo READ_DENIED; fi
if printf hacked > "$2"; then echo WRITE_OK; else echo WRITE_DENIED; fi
if id >/dev/null 2>&1; then echo TOOL_OK; else echo TOOL_DENIED; fi
printf 'TOKEN=%s\\nHOME=%s\\n' "${API_TOKEN-unset}" "$HOME"`
	args := []string{"/bin/bash", "-c", script, "bash", secret, outsideWrite}
	if listener != nil {
		script += `
if /usr/bin/curl -fsS --max-time 2 "http://$3/" >/dev/null 2>&1; then echo NETWORK_OK; else echo NETWORK_DENIED; fi`
		args[2] = script
		args = append(args, listener.Addr().String())
	}
	request, policyPath := preparedRequest(t, root, args, []string{tool}, []string{toolHash})
	report, _, err := Execute(context.Background(), request)
	if err != nil {
		stderr, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(report.StderrPath)))
		if strings.Contains(string(stderr), "sandbox_apply: Operation not permitted") {
			t.Skip("test runner disallows nested macOS Seatbelt; host execution proof runs in the external acceptance environment")
		}
		t.Fatalf("role shell execution: %v (report=%+v) stderr=%q", err, report, stderr)
	}
	if report.Backend != "macos-seatbelt" || report.Enforcement != "host-enforced" || report.Assurance != "unverified" {
		t.Fatalf("execution assurance metadata = %+v", report)
	}
	stdout, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(report.StdoutPath)))
	if err != nil {
		t.Fatal(err)
	}
	text := string(stdout)
	expectedFields := []string{"READ_DENIED", "WRITE_DENIED", "TOOL_DENIED", "TOKEN=unset", "HOME="}
	if listener != nil {
		expectedFields = append(expectedFields, "NETWORK_DENIED")
	}
	for _, expected := range expectedFields {
		if !strings.Contains(text, expected) {
			t.Errorf("stdout missing %q: %s", expected, text)
		}
	}
	if strings.Contains(text, "source-secret") || strings.Contains(text, "must-not-leak") {
		t.Fatalf("child observed undeclared source or parent environment: %s", text)
	}
	if contents, err := os.ReadFile(outsideWrite); err != nil || string(contents) != "keep\n" {
		t.Fatalf("read-only declared path changed: %q, %v", contents, err)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(policyPath))); err != nil {
		t.Fatalf("sealed policy artifact missing: %v", err)
	}
}

func TestExecuteRecordsActualNonzeroChildExit(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Sorna role containment is available only on macOS")
	}
	root := testProject(t)
	request, _ := preparedRequest(t, root, []string{"/usr/bin/false"}, nil, nil)
	report, _, err := Execute(context.Background(), request)
	if err == nil {
		t.Fatal("nonzero child unexpectedly succeeded")
	}
	if report.Status != "failed" || report.ExitCode == nil || *report.ExitCode != 1 {
		stderr, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(report.StderrPath)))
		if strings.Contains(string(stderr), "sandbox_apply: Operation not permitted") {
			t.Skip("test runner disallows nested macOS Seatbelt; host execution proof runs in the external acceptance environment")
		}
		t.Fatalf("nonzero process result was not preserved: %+v stderr=%q", report, stderr)
	}
}

func TestExecuteExecutionIDCannotBeClaimedTwice(t *testing.T) {
	root := testProject(t)
	request, _ := preparedRequest(t, root, []string{"/usr/bin/true"}, nil, nil)
	_, _, firstErr := Execute(context.Background(), request)
	claimPath := filepath.Join(root, ".ingen", "artifacts", "role-executions", request.ExecutionID+".claim")
	if _, err := os.Stat(claimPath); err != nil {
		t.Fatalf("execution claim missing after first invocation (run error %v): %v", firstErr, err)
	}
	claimBefore, err := os.ReadFile(claimPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Execute(context.Background(), request); err == nil {
		t.Fatal("second invocation reused an already claimed execution ID")
	}
	claimAfter, err := os.ReadFile(claimPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(claimAfter) != string(claimBefore) {
		t.Fatal("duplicate invocation changed the immutable execution claim")
	}
}

func TestPersistPolicyRejectsSymlinkedOutputDirectory(t *testing.T) {
	root := testProject(t)
	plan, err := capability.FromFileUnderRoot(root, project.WorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := Prepare(Request{Root: root, WorkspacePath: project.WorkspacePath, RoleID: "implementation", ExecutionID: "symlink-policy", Command: []string{"/usr/bin/true"}, ExpectedManifestSHA256: plan.Workspace.Manifest.SHA256})
	if err != nil {
		t.Fatal(err)
	}
	artifactRoot := filepath.Join(root, ".ingen", "artifacts")
	if err := os.MkdirAll(artifactRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(artifactRoot, "role-executions")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	policyPath := filepath.ToSlash(filepath.Join(".ingen", "artifacts", "role-executions", prepared.ExecutionID+".policy.json"))
	if err := PersistPolicy(root, policyPath, prepared.Policy); err == nil {
		t.Fatal("policy persistence followed a symlinked artifact directory")
	}
	if _, err := os.Stat(filepath.Join(outside, prepared.ExecutionID+".policy.json")); !os.IsNotExist(err) {
		t.Fatalf("policy was written outside project root: %v", err)
	}
}

func TestPrepareRejectsInRootSymlinkAliasToProtectedState(t *testing.T) {
	root := testProject(t)
	protected := filepath.Join(root, ".ingen", "artifacts", "native-sessions")
	if err := os.MkdirAll(protected, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, ".ingen", "public")
	if err := os.Symlink(filepath.Join("artifacts", "native-sessions"), alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	manifestPath := filepath.Join(root, project.WorkspacePath)
	contents, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := workspace.LoadBytes(project.WorkspacePath, contents)
	if err != nil {
		t.Fatal(err)
	}
	for i := range manifest.Roles {
		if manifest.Roles[i].ID == "implementation" {
			manifest.Roles[i].ReadRoots = append(manifest.Roles[i].ReadRoots, ".ingen/public")
		}
	}
	changed, err := yaml.Marshal(workspace.Document{Workspace: manifest})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := capability.FromFileUnderRoot(root, project.WorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	request := Request{Root: root, WorkspacePath: project.WorkspacePath, RoleID: "implementation", ExecutionID: "alias-protected", Command: []string{"/usr/bin/true"}, ExpectedManifestSHA256: plan.Workspace.Manifest.SHA256}
	if _, err := Prepare(request); err == nil {
		t.Fatal("role read capability through an in-root symlink to native session state was accepted")
	}

	// Explicit protected inputs are subject to the same alias rejection even
	// when the manifest itself grants no such capability.
	for i := range manifest.Roles {
		if manifest.Roles[i].ID == "implementation" {
			manifest.Roles[i].ReadRoots = nil
		}
	}
	changed, err = yaml.Marshal(workspace.Document{Workspace: manifest})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err = capability.FromFileUnderRoot(root, project.WorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	request.ExpectedManifestSHA256 = plan.Workspace.Manifest.SHA256
	request.ProtectedPaths = []string{".ingen/public/approval.json"}
	if _, err := Prepare(request); err == nil {
		t.Fatal("protected input through an in-root symlink was accepted")
	}
}

func TestPrepareRejectsProjectRootReadAndWriteCapabilities(t *testing.T) {
	for _, test := range []struct {
		name string
		set  func(*workspace.Role)
	}{
		{"read", func(role *workspace.Role) { role.ReadRoots = append(role.ReadRoots, ".") }},
		{"write", func(role *workspace.Role) { role.WriteRoots = append(role.WriteRoots, ".") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := testProject(t)
			manifestPath := filepath.Join(root, project.WorkspacePath)
			contents, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatal(err)
			}
			manifest, err := workspace.LoadBytes(project.WorkspacePath, contents)
			if err != nil {
				t.Fatal(err)
			}
			for i := range manifest.Roles {
				if manifest.Roles[i].ID == "implementation" {
					test.set(&manifest.Roles[i])
				}
			}
			changed, err := yaml.Marshal(workspace.Document{Workspace: manifest})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(manifestPath, changed, 0o600); err != nil {
				t.Fatal(err)
			}
			plan, err := capability.FromFileUnderRoot(root, project.WorkspacePath)
			if err != nil {
				t.Fatal(err)
			}
			executionID := "root-capability-" + test.name
			request := Request{Root: root, WorkspacePath: project.WorkspacePath, RoleID: "implementation", ExecutionID: executionID, Command: []string{"/usr/bin/true"}, ExpectedManifestSHA256: plan.Workspace.Manifest.SHA256}
			if _, err := Prepare(request); err == nil || !strings.Contains(err.Error(), "project root") {
				t.Fatalf("Prepare error = %v, want project root capability rejection", err)
			}
			for _, path := range []string{
				filepath.Join(root, ".ingen", "artifacts", "role-executions", executionID+".policy.json"),
				filepath.Join(root, ".ingen", "sessions", "implementation", ".sentinel-roleexec", executionID),
			} {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatalf("rejected capability created output %s (stat error %v)", path, err)
				}
			}
		})
	}
}

func preparedRequest(t *testing.T, root string, command, tools, toolHashes []string) (Request, string) {
	t.Helper()
	plan, err := capability.FromFileUnderRoot(root, project.WorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	id := "execute-" + strings.ReplaceAll(t.Name(), "/", "-")
	request := Request{Root: root, WorkspacePath: project.WorkspacePath, RoleID: "implementation", ExecutionID: id, Command: command, AllowedTools: tools, ExpectedToolSHA256: toolHashes, ExpectedManifestSHA256: plan.Workspace.Manifest.SHA256}
	prepared, err := Prepare(request)
	if err != nil {
		t.Fatal(err)
	}
	policyPath := filepath.ToSlash(filepath.Join(".ingen", "artifacts", "role-executions", id+".policy.json"))
	if err := PersistPolicy(root, policyPath, prepared.Policy); err != nil {
		t.Fatal(err)
	}
	request.PolicyPath, request.ExpectedPolicySHA256, request.ExpectedExecutableSHA256 = policyPath, prepared.Policy.SHA256, prepared.ExecutableSHA256
	return request, policyPath
}

func testProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if _, err := project.Initialize(project.Options{Root: root, ID: "roleexec-test"}); err != nil {
		t.Fatal(err)
	}
	return root
}

func policyPaths(document map[string]any, category string) []string {
	fs := document["filesystem"].(map[string]any)
	entries := fs[category].([]any)
	paths := make([]string, 0, len(entries))
	for _, raw := range entries {
		paths = append(paths, raw.(map[string]any)["path"].(string))
	}
	return paths
}

func containsPath(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
}

func copyExecutable(t *testing.T, source, destination string) {
	t.Helper()
	contents, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, contents, 0o700); err != nil {
		t.Fatal(err)
	}
}
