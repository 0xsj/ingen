// Package agentprobe checks a selected Codex CLI's local startup and synthetic
// Responses protocol under Sorna. A supported result is limited to that
// synthetic round trip; it says nothing about provider inference or retention.
package agentprobe

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"ingen/herdr-sentinel/internal/agentlaunch"
	"ingen/herdr-sentinel/internal/codexbroker"
	"ingen/sorna/execution"
	"ingen/sorna/policy"
)

const (
	ReportSchema = "ingen.sentinel-codex-readiness/v1"
	Scope        = "local-synthetic-codex-startup-and-protocol"
	fixtureModel = "sentinel-offline-readiness"
	fixtureKey   = "sentinel-offline-synthetic-credential"
	fixtureReply = "sentinel-offline-readiness-ok"
	maxTimeout   = 120 * time.Second
)

var fixturePrompt = []byte("This is an offline diagnostic. Reply exactly: " + fixtureReply + "\n")

type Request struct {
	ExecutablePath string
	Timeout        time.Duration
}

type Report struct {
	Schema           string            `json:"schema"`
	Scope            string            `json:"scope"`
	Status           string            `json:"status"`
	CheckedAt        string            `json:"checked_at"`
	StartedAt        string            `json:"started_at"`
	FinishedAt       string            `json:"finished_at"`
	ExecutablePath   string            `json:"executable_path"`
	ExecutableSHA256 string            `json:"executable_sha256,omitempty"`
	PolicySHA256     string            `json:"policy_sha256,omitempty"`
	Backend          string            `json:"backend"`
	Enforcement      string            `json:"enforcement"`
	Assurance        string            `json:"assurance"`
	NetworkMode      string            `json:"network_mode"`
	Model            string            `json:"model"`
	Synthetic        bool              `json:"synthetic"`
	ProcessStarted   bool              `json:"process_started"`
	ExitCode         *int              `json:"exit_code,omitempty"`
	MockRoundTrips   int               `json:"mock_round_trips"`
	ReplyObserved    bool              `json:"reply_observed"`
	OutputTruncated  bool              `json:"output_truncated"`
	CaptureComplete  bool              `json:"capture_complete"`
	BrokerStatus     string            `json:"broker_status"`
	Broker           codexbroker.Stats `json:"broker"`
	ReasonCode       string            `json:"reason_code,omitempty"`
	Reason           string            `json:"reason,omitempty"`
	Limitations      []string          `json:"limitations"`
}

type processResult struct {
	Started           bool
	ExitCode          *int
	Canceled          bool
	TimedOut          bool
	StartErr          bool
	Output            []byte
	Truncated         bool
	CaptureIncomplete bool
}

type runner interface {
	Prepare(command []string, root string, sealed policy.Sealed) (execution.Prepared, error)
	Run(ctx context.Context, prepared execution.Prepared, workingDirectory string, env []string, stdin []byte) processResult
}

func (systemRunner) Prepare(command []string, root string, sealed policy.Sealed) (execution.Prepared, error) {
	return execution.Prepare(command, root, sealed)
}

type dependencies struct {
	runner    runner
	transport http.RoundTripper
	now       func() time.Time
	tempDir   string
	goos      string
}

type byteReader struct{ data []byte }

func bytesReader(data []byte) io.Reader { return &byteReader{data: data} }

func (r *byteReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

// Diagnose runs a fixed fresh Codex invocation with a synthetic credential
// and an in-process mock transport. It never consults caller credentials,
// forwards a real provider request, or changes a project/Herdr workspace.
func Diagnose(ctx context.Context, request Request) (Report, error) {
	return diagnose(ctx, request, dependencies{runner: systemRunner{}, transport: fixtureTransport{}, now: time.Now})
}

func diagnose(ctx context.Context, request Request, deps dependencies) (Report, error) {
	if ctx == nil {
		return Report{}, errors.New("diagnostic context is required")
	}
	if request.Timeout == 0 {
		request.Timeout = 45 * time.Second
	}
	if request.Timeout < time.Second || request.Timeout > maxTimeout {
		return Report{}, fmt.Errorf("diagnostic timeout must be between 1 and %s", maxTimeout)
	}
	if request.ExecutablePath == "" || !filepath.IsAbs(request.ExecutablePath) || filepath.Clean(request.ExecutablePath) != request.ExecutablePath {
		return Report{}, errors.New("Codex executable must be an explicit clean absolute path")
	}
	if deps.runner == nil {
		deps.runner = systemRunner{}
	}
	if deps.transport == nil {
		deps.transport = fixtureTransport{}
	}
	if deps.now == nil {
		deps.now = time.Now
	}
	if deps.goos == "" {
		deps.goos = runtime.GOOS
	}
	startedAt := deps.now().UTC()
	report := Report{
		Schema: ReportSchema, Scope: Scope, Status: "indeterminate", StartedAt: startedAt.Format(time.RFC3339Nano),
		ExecutablePath: request.ExecutablePath, Backend: "unavailable", Enforcement: "unavailable", Assurance: "unverified",
		NetworkMode: "allowlist", Model: fixtureModel, Synthetic: true, BrokerStatus: "unavailable",
		Limitations: []string{
			"supported means only that the selected CLI completed one local synthetic Responses round trip under the recorded Sorna backend",
			"the mock transport makes no provider request and provides no evidence of real inference, provider behavior, retention, or credential acceptance",
			"the executable digest is a prelaunch byte check, not an observed running-image identity or host attestation",
			"macOS Seatbelt localhost network permission includes local host addresses at the pinned TCP port; it is not an IPv4-only grant",
			"this probe uses private temporary state and does not mutate a Sentinel, native Herdr, or project workspace",
		},
	}
	var broker *codexbroker.Server
	brokerClosed := false
	var brokerCloseErr error
	closeBroker := func() {
		if broker != nil && !brokerClosed {
			brokerCloseErr = broker.Close()
			brokerClosed = true
			report.Broker = broker.Snapshot()
			report.MockRoundTrips = report.Broker.RequestsForwarded
			report.BrokerStatus = "closed"
			if brokerCloseErr != nil {
				report.BrokerStatus = "indeterminate"
			}
		}
	}
	finish := func() Report {
		closeBroker()
		if brokerCloseErr != nil {
			report.Status = "indeterminate"
			report.ReasonCode = "mock-broker-close-failed"
			report.Reason = "the local synthetic broker did not close cleanly"
		}
		report.FinishedAt = deps.now().UTC().Format(time.RFC3339Nano)
		report.CheckedAt = report.FinishedAt
		return report
	}

	executable, digest, err := resolveAndHashExecutable(request.ExecutablePath)
	if err != nil {
		return Report{}, err
	}
	report.ExecutablePath = executable
	report.ExecutableSHA256 = digest
	if deps.goos != "darwin" {
		report.ReasonCode = "host-enforcement-unavailable"
		report.Reason = "Sorna macOS Seatbelt enforcement is unavailable on this platform"
		return finish(), nil
	}
	if err := ctx.Err(); err != nil {
		report.ReasonCode = "canceled-before-start"
		report.Reason = "diagnostic was canceled before launch"
		return finish(), nil
	}

	root, err := os.MkdirTemp(deps.tempDir, "ingen-codex-readiness-")
	if err != nil {
		return Report{}, fmt.Errorf("create private diagnostic context: %w", err)
	}
	defer os.RemoveAll(root)
	if err := os.Chmod(root, 0700); err != nil {
		return Report{}, fmt.Errorf("secure private diagnostic context: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return Report{}, fmt.Errorf("canonicalize private diagnostic context: %w", err)
	}
	ro := filepath.Join(root, "input")
	rw := filepath.Join(root, "runtime")
	home := filepath.Join(rw, "home")
	codexHome := filepath.Join(rw, "codex")
	tmp := filepath.Join(rw, "tmp")
	for _, directory := range []string{ro, rw, home, codexHome, tmp} {
		if err := os.Mkdir(directory, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return Report{}, fmt.Errorf("create private diagnostic directory: %w", err)
		}
	}
	sourcePath := filepath.Join(ro, "prompt.txt")
	snapshotPath := filepath.Join(ro, "prompt.snapshot")
	if err := os.WriteFile(sourcePath, fixturePrompt, 0400); err != nil {
		return Report{}, fmt.Errorf("write synthetic prompt source: %w", err)
	}
	if err := os.WriteFile(snapshotPath, fixturePrompt, 0400); err != nil {
		return Report{}, fmt.Errorf("write synthetic prompt snapshot: %w", err)
	}
	promptDigest := sha256.Sum256(fixturePrompt)
	promptSHA := hex.EncodeToString(promptDigest[:])

	runContext, cancel := context.WithTimeout(ctx, request.Timeout)
	defer cancel()
	broker, err = codexbroker.Start(runContext, codexbroker.Config{
		APIKey: fixtureKey, Model: fixtureModel, MaxRequests: 1, MaxOutputTokens: 64,
		Timeout: request.Timeout, Transport: deps.transport,
	})
	if err != nil {
		report.ReasonCode = "mock-broker-start-failed"
		report.Reason = "could not start the local synthetic Responses broker"
		return finish(), nil
	}
	report.BrokerStatus = "running"

	brokerContext := &agentlaunch.BrokerContext{
		Provider: "openai-responses", Endpoint: broker.Endpoint(), CredentialSource: "environment", CredentialEnv: "OPENAI_API_KEY",
		MaxRequests: 1, MaxOutputTokens: 64, TimeoutSeconds: max(1, int(request.Timeout.Seconds())),
	}
	_, args, err := agentlaunch.BuildCodex(agentlaunch.CodexRequest{
		ExecutionID: "offline-readiness", Root: root, WorkingDir: rw, HomePath: home, CodexHome: codexHome,
		PromptSourcePath: "input/prompt.txt", PromptSourceSHA256: promptSHA,
		PromptSnapshotPath: "input/prompt.snapshot", PromptSnapshotSHA256: promptSHA, PromptBytes: len(fixturePrompt),
		Model: fixtureModel, Broker: brokerContext,
	})
	if err != nil {
		report.ReasonCode = "fixed-invocation-invalid"
		report.Reason = "the fixed Codex readiness invocation could not be constructed"
		return finish(), nil
	}
	sealed, err := readinessPolicy(root, ro, rw, broker.Endpoint())
	if err != nil {
		report.ReasonCode = "policy-invalid"
		report.Reason = "could not seal the private diagnostic policy"
		return finish(), nil
	}
	prepared, err := deps.runner.Prepare(append([]string{executable}, args...), root, sealed)
	if err != nil {
		report.ReasonCode = "host-enforcement-unavailable"
		report.Reason = "Sorna could not prepare the diagnostic under host enforcement"
		return finish(), nil
	}
	report.Backend = prepared.Backend
	report.Enforcement = prepared.Enforcement
	report.PolicySHA256 = sealed.SHA256
	if prepared.Backend != "macos-seatbelt" || prepared.Enforcement != "host-enforced" || prepared.NetworkMode != "allowlist" || prepared.ExecutableSHA256 != digest {
		report.ReasonCode = "host-enforcement-mismatch"
		report.Reason = "prepared command does not match the required Sorna backend, network scope, or executable digest"
		return finish(), nil
	}
	currentDigest, err := hashExecutable(executable)
	if err != nil || currentDigest != digest {
		report.ReasonCode = "executable-changed-before-launch"
		report.Reason = "selected Codex executable changed after preparation"
		return finish(), nil
	}
	if err := runContext.Err(); err != nil {
		report.ReasonCode = "canceled-before-start"
		report.Reason = "diagnostic was canceled before launch"
		return finish(), nil
	}
	env := []string{
		"PATH=/usr/bin:/bin:/usr/sbin:/sbin",
		"HOME=" + home,
		"CODEX_HOME=" + codexHome,
		"TMPDIR=" + tmp,
		"INGEN_CODEX_BROKER_TOKEN=" + broker.Token(),
	}
	result := deps.runner.Run(runContext, prepared, rw, env, bytes.Clone(fixturePrompt))
	report.ProcessStarted = result.Started
	report.ExitCode = result.ExitCode
	report.OutputTruncated = result.Truncated
	report.CaptureComplete = !result.CaptureIncomplete
	report.ReplyObserved = !result.CaptureIncomplete && observeAssistantReply(result.Output)
	if result.TimedOut || result.Canceled || runContext.Err() != nil {
		report.Status = "indeterminate"
		report.ReasonCode = "execution-canceled-or-timed-out"
		report.Reason = "Codex readiness execution did not finish within its bounded run"
	} else if !result.Started || result.StartErr || result.ExitCode == nil {
		report.Status = "indeterminate"
		report.ReasonCode = "process-start-unknown"
		report.Reason = "Codex process start or exit could not be observed"
	} else if *result.ExitCode != 0 {
		report.Status = "unsupported"
		report.ReasonCode = "codex-startup-failed"
		report.Reason = "Codex exited before completing the synthetic readiness round trip"
		if bytes.Contains(result.Output, []byte("Failed to synchronize managed preferences")) {
			report.ReasonCode = "managed-preferences-unavailable-under-policy"
			report.Reason = "Codex could not synchronize managed preferences under the enforced Sorna policy"
		}
	} else {
		report.Status = "supported"
		report.ReasonCode = "synthetic-round-trip-completed"
		report.Reason = "Codex completed the local synthetic startup and protocol check"
	}
	if result.CaptureIncomplete && report.Status == "supported" {
		report.Status = "indeterminate"
		report.ReasonCode = "process-output-capture-incomplete"
		report.Reason = "process output capture could not be fully joined after child cleanup"
	}
	if result.Truncated && report.Status == "supported" {
		report.Status = "indeterminate"
		report.ReasonCode = "process-output-truncated"
		report.Reason = "process output exceeded the bounded diagnostic capture"
	}
	closeBroker()
	if report.Status == "supported" && (report.MockRoundTrips != 1 || report.Broker.RequestsRejected != 0 || report.Broker.UpstreamFailures != 0 || report.Broker.InFlight != 0 || !report.ReplyObserved) {
		report.Status = "unsupported"
		report.ReasonCode = "synthetic-round-trip-incomplete"
		report.Reason = "Codex did not complete exactly one expected local synthetic round trip"
	}
	return finish(), nil
}

func readinessPolicy(_root, ro, rw, endpoint string) (policy.Sealed, error) {
	port, err := endpointPort(endpoint)
	if err != nil {
		return policy.Sealed{}, err
	}
	document := policy.Document{Policy: map[string]any{
		"schema": policy.Schema, "id": "sentinel-offline-codex-readiness", "version": 1,
		"status": "draft", "purpose": "synthetic Codex startup and protocol diagnostic", "enforcement": "host-enforced",
		"filesystem": map[string]any{
			"read":  []any{map[string]any{"path": ro, "reason": "private synthetic prompt"}},
			"write": []any{map[string]any{"path": rw, "reason": "private diagnostic runtime state"}},
			"deny":  []any{},
		},
		"network": map[string]any{
			"mode":  "allowlist",
			"allow": []any{map[string]any{"host": "localhost", "ports": []any{port}, "direction": "outbound", "purpose": "offline synthetic Responses broker"}},
		},
		"process": map[string]any{"subject_id": "sentinel-offline-readiness", "can_invoke_subject": false, "allowed_tools": []any{}},
	}}
	sealed, err := policy.Seal(document)
	if err != nil {
		return policy.Sealed{}, fmt.Errorf("seal diagnostic policy: %w", err)
	}
	return sealed, nil
}

func endpointPort(endpoint string) (int, error) {
	const prefix, suffix = "http://127.0.0.1:", "/v1"
	if !strings.HasPrefix(endpoint, prefix) || !strings.HasSuffix(endpoint, suffix) {
		return 0, errors.New("synthetic broker endpoint is not the expected loopback URL")
	}
	var port int
	if _, err := fmt.Sscanf(strings.TrimSuffix(strings.TrimPrefix(endpoint, prefix), suffix), "%d", &port); err != nil || port < 1 || port > 65535 {
		return 0, errors.New("synthetic broker endpoint has an invalid port")
	}
	return port, nil
}

func resolveAndHashExecutable(path string) (string, string, error) {
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", "", fmt.Errorf("resolve Codex executable: %w", err)
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return "", "", fmt.Errorf("resolve Codex executable: %w", err)
	}
	digest, err := hashExecutable(canonical)
	if err != nil {
		return "", "", err
	}
	return canonical, digest, nil
}

func hashExecutable(path string) (string, error) {
	file, err := openExecutable(path)
	if err != nil {
		return "", fmt.Errorf("open Codex executable: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("Codex executable must be a regular file")
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", fmt.Errorf("hash Codex executable: %w", err)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

type fixtureTransport struct{}

func (fixtureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Method != http.MethodPost || request.URL.String() != "https://api.openai.com/v1/responses" || request.Header.Get("Authorization") != "Bearer "+fixtureKey {
		return nil, errors.New("synthetic broker received an unexpected request")
	}
	var body map[string]json.RawMessage
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		return nil, errors.New("synthetic broker received invalid request JSON")
	}
	var model string
	var outputTokens int
	var store, stream bool
	if json.Unmarshal(body["model"], &model) != nil || model != fixtureModel || json.Unmarshal(body["max_output_tokens"], &outputTokens) != nil || outputTokens != 64 || json.Unmarshal(body["store"], &store) != nil || store || json.Unmarshal(body["stream"], &stream) != nil || !stream {
		return nil, errors.New("synthetic broker received a request outside the diagnostic bounds")
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(fixtureSSE())),
		Request:    request,
	}, nil
}

func fixtureSSE() string {
	message := map[string]any{"id": "msg_readiness", "type": "message", "role": "assistant", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": fixtureReply, "annotations": []any{}}}}
	response := map[string]any{"id": "resp_readiness", "object": "response", "created_at": 1, "status": "completed", "model": fixtureModel, "output": []any{message}, "usage": map[string]any{"input_tokens": 1, "output_tokens": 2, "total_tokens": 3}}
	events := []map[string]any{
		{"type": "response.created", "response": map[string]any{"id": "resp_readiness", "object": "response", "status": "in_progress", "output": []any{}}},
		{"type": "response.output_item.added", "output_index": 0, "item": map[string]any{"id": "msg_readiness", "type": "message", "role": "assistant", "status": "in_progress", "content": []any{}}},
		{"type": "response.output_text.delta", "item_id": "msg_readiness", "output_index": 0, "content_index": 0, "delta": fixtureReply},
		{"type": "response.output_item.done", "output_index": 0, "item": message},
		{"type": "response.completed", "response": response},
	}
	var output strings.Builder
	for _, event := range events {
		data, _ := json.Marshal(event)
		fmt.Fprintf(&output, "event: %s\ndata: %s\n\n", event["type"], data)
	}
	return output.String()
}

func observeAssistantReply(output []byte) bool {
	var sawCompletion, sawReply bool
	for len(output) > 0 {
		line := output
		if index := bytes.IndexByte(output, '\n'); index >= 0 {
			line, output = output[:index], output[index+1:]
		} else {
			output = nil
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var event struct {
			Type string          `json:"type"`
			Item json.RawMessage `json:"item"`
		}
		if json.Unmarshal(line, &event) != nil {
			continue
		}
		if event.Type == "turn.completed" {
			sawCompletion = true
		}
		if event.Type != "item.completed" || len(event.Item) == 0 {
			continue
		}
		var item struct {
			Type    string `json:"type"`
			Text    string `json:"text"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if json.Unmarshal(event.Item, &item) != nil || item.Type != "agent_message" {
			continue
		}
		if strings.Contains(item.Text, fixtureReply) {
			sawReply = true
		}
		for _, part := range item.Content {
			if part.Type == "output_text" && strings.Contains(part.Text, fixtureReply) {
				sawReply = true
			}
		}
	}
	return sawCompletion && sawReply
}
