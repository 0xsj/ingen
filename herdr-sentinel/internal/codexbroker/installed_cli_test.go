package codexbroker_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"ingen/herdr-sentinel/internal/agentlaunch"
	"ingen/herdr-sentinel/internal/codexbroker"
	"ingen/sorna/execution"
	"ingen/sorna/policy"
)

type installedCLITransport func(*http.Request) (*http.Response, error)

func (f installedCLITransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// This wire check deliberately runs outside Sorna. It uses private state and
// synthetic authentication, but grants no containment assurance. The separate
// contained runtime gate below must also pass before claiming runtime support.
func TestInstalledCodexBrokerProtocol(t *testing.T) {
	binary := os.Getenv("INGEN_CODEX_INTEGRATION_BINARY")
	if binary == "" {
		t.Skip("set INGEN_CODEX_INTEGRATION_BINARY to opt into wire compatibility")
	}
	if !filepath.IsAbs(binary) {
		t.Fatal("installed CLI binary must be an explicit absolute path")
	}
	const key, model = "synthetic-wire-only-key", "codex-fixture-model"
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var mu sync.Mutex
	var request map[string]any
	broker, err := codexbroker.Start(ctx, codexbroker.Config{APIKey: key, Model: model, MaxRequests: 1, MaxOutputTokens: 64, Timeout: 30 * time.Second, Transport: installedCLITransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api.openai.com/v1/responses" || r.Header.Get("Authorization") != "Bearer "+key {
			return nil, fmt.Errorf("unexpected upstream request")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, fmt.Errorf("invalid upstream JSON")
		}
		mu.Lock()
		request = body
		mu.Unlock()
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(installedCLISSE(model))), Request: r}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	token := broker.Token()
	root := t.TempDir()
	home, codexHome := filepath.Join(root, "home"), filepath.Join(root, "codex")
	for _, path := range []string{home, codexHome} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	_, args, err := agentlaunch.BuildCodex(agentlaunch.CodexRequest{ExecutionID: "wire-fixture", Root: root, WorkingDir: root, HomePath: home, CodexHome: codexHome, PromptSourcePath: "prompt.txt", PromptSourceSHA256: strings.Repeat("a", 64), PromptSnapshotPath: "snapshot.txt", PromptSnapshotSHA256: strings.Repeat("a", 64), PromptBytes: 12, Model: model, Broker: &agentlaunch.BrokerContext{Provider: "openai-responses", Endpoint: broker.Endpoint(), CredentialSource: "environment", CredentialEnv: "OPENAI_API_KEY", MaxRequests: 1, MaxOutputTokens: 64, TimeoutSeconds: 30}})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = root
	// The wire-only fixture invokes no tools. Exclude Git from PATH so CLI
	// marketplace refresh cannot spawn a remote catalog clone in this probe.
	cmd.Env = []string{"PATH=/bin:/usr/sbin:/sbin", "HOME=" + home, "CODEX_HOME=" + codexHome, "TMPDIR=" + root, "INGEN_CODEX_BROKER_TOKEN=" + token}
	cmd.Stdin = strings.NewReader("Say fixture-broker-ok. Do not invoke tools.\n")
	prepareInstalledProcess(cmd)
	output, err := cmd.CombinedOutput()
	cleanupInstalledProcess(cmd)
	clean := strings.ReplaceAll(strings.ReplaceAll(string(output), key, "[redacted]"), token, "[redacted]")
	if err != nil {
		t.Fatalf("installed CLI wire check failed: %v\n%s", err, clean)
	}
	if !strings.Contains(clean, "fixture-broker-ok") || !strings.Contains(clean, `"type":"turn.completed"`) {
		t.Fatalf("CLI did not complete synthetic response: %s", clean)
	}
	if strings.Contains(string(output), key) || strings.Contains(string(output), token) {
		t.Fatal("wire output exposed synthetic credential or scoped token")
	}
	if err := broker.Close(); err != nil {
		t.Fatal(err)
	}
	stats := broker.Snapshot()
	if stats.RequestsForwarded != 1 || stats.RequestsRejected != 0 || stats.UpstreamFailures != 0 || stats.InFlight != 0 {
		t.Fatalf("unexpected broker statistics: %+v", stats)
	}
	mu.Lock()
	body := request
	mu.Unlock()
	if body["model"] != model || body["store"] != false || body["max_output_tokens"] != float64(64) || body["stream"] != true {
		t.Fatal("wire request does not match constrained model/storage/token/stream settings")
	}
	for _, field := range []string{"client_metadata", "prompt_cache_key", "previous_response_id", "conversation"} {
		if _, present := body[field]; present {
			t.Fatalf("forbidden field %s reached mock upstream", field)
		}
	}
	t.Log("installed CLI completed one synthetic Responses stream through the production broker; wire compatibility only, no containment or provider inference claim")
}

// This opt-in compatibility check runs the actual installed CLI against the
// production broker, with a transport that cannot make an upstream connection.
// The CLI and its local tool run inside the existing Sorna Seatbelt wrapper.
// This is a local boundary probe, not host attestation or provider inference.
// Codex 0.160.0 currently fails this gate at managed preferences initialization.
func TestInstalledCodexContainedBrokerProtocol(t *testing.T) {
	binary := os.Getenv("INGEN_CODEX_CONTAINED_INTEGRATION_BINARY")
	if binary == "" {
		t.Skip("set INGEN_CODEX_CONTAINED_INTEGRATION_BINARY to opt into the contained runtime release gate")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("the contained installed CLI compatibility check requires macOS Seatbelt")
	}
	if !filepath.IsAbs(binary) {
		t.Fatal("installed CLI binary must be an explicit absolute path")
	}
	const key = "synthetic-installed-cli-upstream-key"
	const model = "codex-fixture-model"
	var mu sync.Mutex
	var forwarded []map[string]any
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rw := filepath.Join(root, "rw")
	forbidden := filepath.Join(root, "forbidden")
	if err := os.MkdirAll(rw, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(forbidden, 0700); err != nil {
		t.Fatal(err)
	}
	const forbiddenMarker = "synthetic-forbidden-context-marker"
	if err := os.WriteFile(filepath.Join(forbidden, "prior.txt"), []byte(forbiddenMarker), 0600); err != nil {
		t.Fatal(err)
	}
	toolArguments, _ := json.Marshal(map[string]any{"cmd": "printf 'fixture-code-written\\n' > " + strconv.Quote(filepath.Join(rw, "result.txt")) + "; if IFS= read -r prior < " + strconv.Quote(filepath.Join(forbidden, "prior.txt")) + "; then printf 'forbidden-read-succeeded'; exit 3; fi; printf 'denied-read-checked\\n'", "shell": "/bin/sh", "login": false, "workdir": rw, "max_output_tokens": 256})
	transport := installedCLITransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api.openai.com/v1/responses" || r.Header.Get("Authorization") != "Bearer "+key {
			return nil, fmt.Errorf("unexpected upstream request")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, fmt.Errorf("invalid upstream JSON")
		}
		mu.Lock()
		forwarded = append(forwarded, body)
		turn := len(forwarded)
		mu.Unlock()
		stream := installedCLISSE(model)
		if turn == 1 {
			stream = installedCLIToolSSE(model, string(toolArguments))
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream)), Request: r}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	broker, err := codexbroker.Start(ctx, codexbroker.Config{APIKey: key, Model: model, MaxRequests: 2, MaxOutputTokens: 64, Timeout: 30 * time.Second, Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	token := broker.Token()
	home, codexHome := filepath.Join(rw, "home"), filepath.Join(rw, "codex")
	for _, path := range []string{home, codexHome} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	_, args, err := agentlaunch.BuildCodex(agentlaunch.CodexRequest{
		ExecutionID: "installed-cli-fixture", Root: root, WorkingDir: root, HomePath: home, CodexHome: codexHome,
		PromptSourcePath: "prompt.txt", PromptSourceSHA256: strings.Repeat("a", 64), PromptSnapshotPath: "snapshot.txt", PromptSnapshotSHA256: strings.Repeat("a", 64), PromptBytes: 12, Model: model,
		Broker: &agentlaunch.BrokerContext{Provider: "openai-responses", Endpoint: broker.Endpoint(), CredentialSource: "environment", CredentialEnv: "OPENAI_API_KEY", MaxRequests: 2, MaxOutputTokens: 64, TimeoutSeconds: 30},
	})
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(broker.Endpoint(), "http://127.0.0.1:"), "/v1"))
	sealed, err := policy.Seal(policy.Document{Policy: map[string]any{
		"schema": policy.Schema, "id": "installed-codex-broker-fixture", "version": 1, "status": "draft", "purpose": "synthetic installed CLI boundary probe", "enforcement": "host-enforced",
		"filesystem": map[string]any{"read": []any{map[string]any{"path": root, "reason": "private fixture state"}}, "write": []any{map[string]any{"path": rw, "reason": "private fixture state"}}, "deny": []any{map[string]any{"path": forbidden, "reason": "prior context denial probe"}}},
		"network":    map[string]any{"mode": "allowlist", "allow": []any{map[string]any{"host": "localhost", "ports": []any{port}, "direction": "outbound", "purpose": "local fixture broker"}}},
		"process":    map[string]any{"subject_id": "installed-cli-fixture", "can_invoke_subject": false, "allowed_tools": []any{map[string]any{"name": "/bin/sh", "purpose": "explicit local fixture tool"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := execution.Prepare(append([]string{binary}, args...), root, sealed)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, prepared.Command[0], prepared.Command[1:]...)
	cmd.Dir = root
	cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "HOME=" + home, "CODEX_HOME=" + codexHome, "TMPDIR=" + rw, "INGEN_CODEX_BROKER_TOKEN=" + token}
	cmd.Stdin = strings.NewReader("Execute the fixture tool call, then say fixture-broker-ok.\n")
	prepareInstalledProcess(cmd)
	output, err := cmd.CombinedOutput()
	cleanupInstalledProcess(cmd)
	cleanOutput := strings.ReplaceAll(strings.ReplaceAll(string(output), key, "[redacted]"), token, "[redacted]")
	if err != nil {
		t.Fatalf("installed CLI failed: %v\n%s", err, cleanOutput)
	}
	if !strings.Contains(cleanOutput, "fixture-broker-ok") || !strings.Contains(cleanOutput, `"type":"turn.completed"`) {
		t.Fatalf("CLI did not finish the fixture response: %s", cleanOutput)
	}
	if strings.Contains(string(output), key) || strings.Contains(string(output), token) {
		t.Fatal("CLI output exposed synthetic upstream credential or scoped token")
	}
	if err := broker.Close(); err != nil {
		t.Fatal(err)
	}
	stats := broker.Snapshot()
	if stats.RequestsForwarded != 2 || stats.RequestsRejected != 0 || stats.UpstreamFailures != 0 || stats.InFlight != 0 {
		t.Fatalf("unexpected local broker counters: %+v", stats)
	}
	mu.Lock()
	bodies := forwarded
	mu.Unlock()
	for _, body := range bodies {
		if body["model"] != model || body["store"] != false || body["max_output_tokens"] != float64(64) || body["stream"] != true {
			t.Fatal("installed CLI request was not constrained to the fixture model, storage setting, token cap, and stream")
		}
	}
	result, err := os.ReadFile(filepath.Join(rw, "result.txt"))
	if err != nil || string(result) != "fixture-code-written\n" {
		t.Fatal("installed CLI local tool did not write the exact allowed fixture file")
	}
	second, _ := json.Marshal(bodies[1])
	if !strings.Contains(string(second), "denied-read-checked") || !strings.Contains(strings.ToLower(string(second)), "operation not permitted") || strings.Contains(string(second), forbiddenMarker) {
		t.Fatal("local tool output did not prove the denied prior-context read")
	}
	t.Log("installed CLI completed two mocked Responses turns through the production broker; local tool wrote allowed bytes and received an explicit denied read under Seatbelt; no provider call")
}

func installedCLIToolSSE(model, arguments string) string {
	item := map[string]any{"id": "fc_fixture", "type": "function_call", "call_id": "call_fixture", "name": "exec_command", "arguments": arguments, "status": "completed"}
	response := map[string]any{"id": "resp_fixture_tool", "object": "response", "created_at": 1, "status": "completed", "model": model, "output": []any{item}, "usage": map[string]any{"input_tokens": 1, "output_tokens": 1, "total_tokens": 2}}
	events := []map[string]any{{"type": "response.created", "response": map[string]any{"id": "resp_fixture_tool", "object": "response", "status": "in_progress", "output": []any{}}}, {"type": "response.output_item.added", "output_index": 0, "item": item}, {"type": "response.output_item.done", "output_index": 0, "item": item}, {"type": "response.completed", "response": response}}
	var result strings.Builder
	for _, event := range events {
		encoded, _ := json.Marshal(event)
		fmt.Fprintf(&result, "event: %s\ndata: %s\n\n", event["type"], encoded)
	}
	return result.String()
}

func installedCLISSE(model string) string {
	message := map[string]any{"id": "msg_fixture", "type": "message", "role": "assistant", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": "fixture-broker-ok", "annotations": []any{}}}}
	response := map[string]any{"id": "resp_fixture", "object": "response", "created_at": 1, "status": "completed", "model": model, "output": []any{message}, "usage": map[string]any{"input_tokens": 1, "output_tokens": 1, "total_tokens": 2}}
	events := []map[string]any{
		{"type": "response.created", "response": map[string]any{"id": "resp_fixture", "object": "response", "status": "in_progress", "output": []any{}}},
		{"type": "response.output_item.added", "output_index": 0, "item": map[string]any{"id": "msg_fixture", "type": "message", "role": "assistant", "status": "in_progress", "content": []any{}}},
		{"type": "response.output_text.delta", "item_id": "msg_fixture", "output_index": 0, "content_index": 0, "delta": "fixture-broker-ok"},
		{"type": "response.output_item.done", "output_index": 0, "item": message},
		{"type": "response.completed", "response": response},
	}
	var result strings.Builder
	for _, event := range events {
		encoded, _ := json.Marshal(event)
		fmt.Fprintf(&result, "event: %s\ndata: %s\n\n", event["type"], encoded)
	}
	return result.String()
}
