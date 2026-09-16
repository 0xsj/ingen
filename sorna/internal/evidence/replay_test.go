package evidence

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"ingen/sorna/internal/oracle"
	"ingen/sorna/internal/runner"
)

func TestReplayMatchesFrozenRunWithoutChangingEvidence(t *testing.T) {
	artifact := replayArtifact()
	recorded, err := runner.ExecuteOracle(context.Background(), artifact, runner.Config{
		BaseURL: "http://recorded.invalid",
		Client:  replayClient(200, `{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if _, err := WriteBundle(directory, recorded, nil); err != nil {
		t.Fatal(err)
	}
	checksumsBeforeBytes, err := os.ReadFile(directory + "/checksums.sha256")
	if err != nil {
		t.Fatal(err)
	}

	result, err := Replay(context.Background(), directory, artifact, runner.Config{
		BaseURL: "http://equivalent.invalid",
		Client:  replayClient(200, `{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "matched" || result.Integrity.Status != "verified" || result.Behavior.Status != "matched" {
		t.Fatalf("replay result = %+v, want verified matching replay", result)
	}
	if result.Behavior.ObservationStatus != "same" || result.ObservationChanges != 0 {
		t.Fatalf("observation result = %+v, want unchanged observations", result)
	}
	if len(result.Rules) != 1 || result.Rules[0].Status != "match" {
		t.Fatalf("rule replay result = %+v, want one matching rule", result.Rules)
	}
	checksumsAfter, err := os.ReadFile(directory + "/checksums.sha256")
	if err != nil {
		t.Fatal(err)
	}
	if string(checksumsBeforeBytes) != string(checksumsAfter) {
		t.Fatal("replay changed the evidence bundle")
	}
}

func TestReplayReportsOutcomeDrift(t *testing.T) {
	artifact := replayArtifact()
	recorded, err := runner.ExecuteOracle(context.Background(), artifact, runner.Config{
		BaseURL: "http://recorded.invalid",
		Client:  replayClient(200, `{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if _, err := WriteBundle(directory, recorded, nil); err != nil {
		t.Fatal(err)
	}

	result, err := Replay(context.Background(), directory, artifact, runner.Config{
		BaseURL: "http://changed.invalid",
		Client:  replayClient(500, `{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "drifted" || result.Behavior.Status != "drifted" {
		t.Fatalf("replay result = %+v, want outcome drift", result)
	}
	if len(result.Rules) != 1 || result.Rules[0].Status != "outcome-drift" || result.Rules[0].RecordedStatus != "pass" || result.Rules[0].ReplayStatus != "fail" {
		t.Fatalf("rule replay result = %+v, want pass-to-fail drift", result.Rules)
	}
}

func TestReplaySeparatesObservationChangeFromOutcomeDrift(t *testing.T) {
	artifact := replayArtifact()
	recorded, err := runner.ExecuteOracle(context.Background(), artifact, runner.Config{
		BaseURL: "http://recorded.invalid",
		Client:  replayClient(200, `{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if _, err := WriteBundle(directory, recorded, nil); err != nil {
		t.Fatal(err)
	}

	result, err := Replay(context.Background(), directory, artifact, runner.Config{
		BaseURL: "http://changed-body.invalid",
		Client:  replayClient(200, `{"message":"different"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "matched" || result.Behavior.Status != "matched" || result.Behavior.ObservationStatus != "changed" || result.ObservationChanges != 1 {
		t.Fatalf("replay result = %+v, want matching outcome with changed observation", result)
	}
	if result.Rules[0].Status != "match" || result.Rules[0].ObservationStatus != "changed" {
		t.Fatalf("rule replay result = %+v, want matching outcome with changed observation", result.Rules[0])
	}
}

func TestReplayReportsExecutionErrorSeparatelyFromDrift(t *testing.T) {
	artifact := replayArtifact()
	recorded, err := runner.ExecuteOracle(context.Background(), artifact, runner.Config{
		BaseURL: "http://recorded.invalid",
		Client:  replayClient(200, `{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if _, err := WriteBundle(directory, recorded, nil); err != nil {
		t.Fatal(err)
	}

	result, err := Replay(context.Background(), directory, artifact, runner.Config{
		BaseURL: "http://unreachable.invalid",
		Client: &http.Client{Transport: replayRoundTripper(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("subject unavailable")
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "error" || result.Behavior.Status != "error" || result.Behavior.ObservationStatus != "unavailable" {
		t.Fatalf("replay result = %+v, want execution error", result)
	}
	if result.Rules[0].ObservationStatus != "unavailable" || result.ObservationChanges != 0 {
		t.Fatalf("observation result = %+v, want unavailable observation", result)
	}
}

func replayArtifact() oracle.Artifact {
	return oracle.Artifact{
		Schema:       oracle.Schema,
		Status:       "frozen",
		Contract:     oracle.ContractReference{ID: "replay-contract", Version: 1, SHA256: strings.Repeat("a", 64)},
		PolicySHA256: strings.Repeat("b", 64),
		Cases: []oracle.Case{{
			CaseID: "case-0001", RuleID: "replay.status", Strength: "must", Subject: "GET /status",
			Expect: map[string]any{"status": int64(200)},
		}},
	}
}

func replayClient(status int, body string) *http.Client {
	return &http.Client{Transport: replayRoundTripper(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})}
}

type replayRoundTripper func(*http.Request) (*http.Response, error)

func (roundTripper replayRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTripper(request)
}
