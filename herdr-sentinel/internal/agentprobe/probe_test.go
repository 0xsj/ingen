package agentprobe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ingen/sorna/execution"
	"ingen/sorna/policy"
)

type fakeRunner struct {
	mode     string
	requests int
}

func (r *fakeRunner) Prepare(command []string, _ string, sealed policy.Sealed) (execution.Prepared, error) {
	if len(command) < 2 || sealed.SHA256 == "" {
		return execution.Prepared{}, os.ErrInvalid
	}
	digest, err := hashExecutable(command[0])
	if err != nil {
		return execution.Prepared{}, err
	}
	return execution.Prepared{
		Command: command, Backend: "macos-seatbelt", Enforcement: "host-enforced", NetworkMode: "allowlist",
		ExecutablePath: command[0], ExecutableSHA256: digest,
	}, nil
}

func (r *fakeRunner) Run(_ context.Context, prepared execution.Prepared, _ string, env []string, _ []byte) processResult {
	values := make(map[string]string)
	for _, item := range env {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			values[key] = value
		}
	}
	if values["OPENAI_API_KEY"] != "" || values["OPENAI_BASE_URL"] != "" || values["AWS_SECRET_ACCESS_KEY"] != "" {
		return processResult{Started: true, ExitCode: intPointer(70)}
	}
	if r.mode == "cancel" {
		return processResult{Started: true, Canceled: true}
	}
	if r.mode == "startup-failure" {
		return processResult{Started: true, ExitCode: intPointer(65), Output: []byte("Failed to synchronize managed preferences")}
	}
	endpoint := ""
	for _, arg := range prepared.Command {
		if strings.HasPrefix(arg, "model_providers.ingen_broker.base_url=") {
			endpoint = strings.TrimPrefix(arg, "model_providers.ingen_broker.base_url=")
		}
	}
	requestBody, _ := json.Marshal(map[string]any{
		"model": fixtureModel, "input": "synthetic probe", "max_output_tokens": 64, "store": false, "stream": true,
	})
	request, err := http.NewRequest(http.MethodPost, endpoint+"/responses", strings.NewReader(string(requestBody)))
	if err != nil {
		return processResult{Started: true, ExitCode: intPointer(71)}
	}
	request.Header.Set("Authorization", "Bearer "+values["INGEN_CODEX_BROKER_TOKEN"])
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return processResult{Started: true, ExitCode: intPointer(72)}
	}
	_, readErr := io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if readErr != nil || response.StatusCode != http.StatusOK {
		return processResult{Started: true, ExitCode: intPointer(73)}
	}
	r.requests++
	if r.mode == "echo-only" {
		return processResult{Started: true, ExitCode: intPointer(0), Output: []byte(`{"type":"turn.completed","text":"` + fixtureReply + `"}`)}
	}
	output := "{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"" + fixtureReply + "\"}}\n{\"type\":\"turn.completed\"}\n"
	return processResult{Started: true, ExitCode: intPointer(0), Output: []byte(output)}
}

func TestDiagnoseReportsOnlySyntheticRoundTripAndFiltersCredentials(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "real-secret-value-must-not-cross-boundary")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "also-private")
	path := executableFixture(t)
	runner := &fakeRunner{}
	report, err := diagnose(context.Background(), Request{ExecutablePath: path, Timeout: 5 * time.Second}, dependencies{
		runner: runner, transport: fixtureTransport{}, now: time.Now, tempDir: t.TempDir(), goos: "darwin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "supported" || report.Scope != Scope || !report.Synthetic || !report.ProcessStarted || report.ExitCode == nil || *report.ExitCode != 0 {
		t.Fatalf("unexpected readiness result: %#v", report)
	}
	if report.MockRoundTrips != 1 || report.Broker.RequestsForwarded != 1 || report.Broker.ClosedAt == nil || !report.ReplyObserved || !report.CaptureComplete {
		t.Fatalf("synthetic broker/reply evidence was not complete: %#v", report)
	}
	if runner.requests != 1 {
		t.Fatalf("mock requests = %d, want 1", runner.requests)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"real-secret-value-must-not-cross-boundary", "also-private", fixtureKey} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("readiness report exposed a credential value")
		}
	}
}

func TestDiagnoseClassifiesKnownManagedPreferencesFailureWithoutPublishingOutput(t *testing.T) {
	path := executableFixture(t)
	report, err := diagnose(context.Background(), Request{ExecutablePath: path, Timeout: 5 * time.Second}, dependencies{
		runner: &fakeRunner{mode: "startup-failure"}, transport: fixtureTransport{}, now: time.Now, tempDir: t.TempDir(), goos: "darwin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "unsupported" || report.ReasonCode != "managed-preferences-unavailable-under-policy" || report.ExitCode == nil || *report.ExitCode != 65 {
		t.Fatalf("known startup failure was not classified truthfully: %#v", report)
	}
	if report.MockRoundTrips != 0 || report.Broker.RequestsForwarded != 0 || report.ReplyObserved || strings.Contains(report.Reason, "Failed to synchronize") {
		t.Fatalf("failure report leaked output or claimed a round trip: %#v", report)
	}
}

func TestDiagnoseCancellationIsIndeterminateAndPromptEchoIsNotReply(t *testing.T) {
	path := executableFixture(t)
	for _, mode := range []string{"cancel", "echo-only"} {
		t.Run(mode, func(t *testing.T) {
			report, err := diagnose(context.Background(), Request{ExecutablePath: path, Timeout: 5 * time.Second}, dependencies{
				runner: &fakeRunner{mode: mode}, transport: fixtureTransport{}, now: time.Now, tempDir: t.TempDir(), goos: "darwin",
			})
			if err != nil {
				t.Fatal(err)
			}
			if mode == "cancel" {
				if report.Status != "indeterminate" || report.ExitCode != nil {
					t.Fatalf("cancellation status/exit is not truthful: %#v", report)
				}
				return
			}
			if report.Status == "supported" || report.ReplyObserved {
				t.Fatalf("prompt echo was accepted as an assistant reply: %#v", report)
			}
		})
	}
}

func TestDiagnoseCaptureUncertaintyDowngradesSyntheticSuccess(t *testing.T) {
	path := executableFixture(t)
	runner := &captureIssueRunner{fakeRunner: &fakeRunner{}, result: processResult{
		Started: true, ExitCode: intPointer(0), Output: []byte(`{"type":"item.completed","item":{"type":"agent_message","text":"` + fixtureReply + `"}}\n{"type":"turn.completed"}`),
		CaptureIncomplete: true,
	}}
	report, err := diagnose(context.Background(), Request{ExecutablePath: path, Timeout: 5 * time.Second}, dependencies{
		runner: runner, transport: fixtureTransport{}, now: time.Now, tempDir: t.TempDir(), goos: "darwin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "indeterminate" || report.ReasonCode != "process-output-capture-incomplete" || report.CaptureComplete {
		t.Fatalf("incomplete process capture was not downgraded: %#v", report)
	}
}

func TestDiagnoseTruncatedOutputCannotBeSupported(t *testing.T) {
	path := executableFixture(t)
	runner := &captureIssueRunner{fakeRunner: &fakeRunner{}, result: processResult{
		Started: true, ExitCode: intPointer(0), Output: []byte(`{"type":"item.completed","item":{"type":"agent_message","text":"` + fixtureReply + `"}}` + "\n" + `{"type":"turn.completed"}`),
		Truncated: true,
	}}
	report, err := diagnose(context.Background(), Request{ExecutablePath: path, Timeout: 5 * time.Second}, dependencies{
		runner: runner, transport: fixtureTransport{}, now: time.Now, tempDir: t.TempDir(), goos: "darwin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "indeterminate" || report.ReasonCode != "process-output-truncated" || !report.OutputTruncated {
		t.Fatalf("truncated process output was not downgraded: %#v", report)
	}
}

func TestDiagnoseRefusesExecutableChangedAfterSornaPreparation(t *testing.T) {
	path := executableFixture(t)
	runner := &mutatingExecutableRunner{fakeRunner: &fakeRunner{}, path: path}
	report, err := diagnose(context.Background(), Request{ExecutablePath: path, Timeout: 5 * time.Second}, dependencies{
		runner: runner, transport: fixtureTransport{}, now: time.Now, tempDir: t.TempDir(), goos: "darwin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "indeterminate" || report.ReasonCode != "executable-changed-before-launch" || runner.ran {
		t.Fatalf("changed executable reached process launch: report=%#v ran=%t", report, runner.ran)
	}
}

type captureIssueRunner struct {
	*fakeRunner
	result processResult
}

type mutatingExecutableRunner struct {
	*fakeRunner
	path string
	ran  bool
}

func (r *mutatingExecutableRunner) Prepare(command []string, root string, sealed policy.Sealed) (execution.Prepared, error) {
	prepared, err := r.fakeRunner.Prepare(command, root, sealed)
	if err == nil {
		err = os.WriteFile(r.path, []byte("changed after prepare"), 0700)
	}
	return prepared, err
}

func (r *mutatingExecutableRunner) Run(context.Context, execution.Prepared, string, []string, []byte) processResult {
	r.ran = true
	return processResult{Started: true, ExitCode: intPointer(0)}
}

func (r *captureIssueRunner) Run(context.Context, execution.Prepared, string, []string, []byte) processResult {
	return r.result
}

func TestObserveAssistantReplyRequiresCompletedStructuredEvents(t *testing.T) {
	for _, input := range []string{
		`{"type":"turn.completed","text":"` + fixtureReply + `"}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"` + fixtureReply + `"}}`,
	} {
		if observeAssistantReply([]byte(input)) {
			t.Fatalf("accepted incomplete or non-structured synthetic reply: %s", input)
		}
	}
	valid := `{"type":"item.completed","item":{"type":"agent_message","text":"` + fixtureReply + `"}}` + "\n" + `{"type":"turn.completed"}`
	if !observeAssistantReply([]byte(valid)) {
		t.Fatal("rejected expected completed assistant reply")
	}
}

func executableFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "codex")
	data := []byte("synthetic executable fixture")
	if err := os.WriteFile(path, data, 0700); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if got, err := hashExecutable(path); err != nil || got != hex.EncodeToString(digest[:]) {
		t.Fatalf("fixture digest mismatch: %q %v", got, err)
	}
	return path
}

func intPointer(value int) *int { return &value }
