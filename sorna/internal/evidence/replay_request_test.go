package evidence

import (
	"testing"

	"ingen/sorna/internal/runner"
)

func TestReplayRequestFingerprintIgnoresSubjectHostAndPort(t *testing.T) {
	recorded := runner.RuleResult{
		Request: runner.Request{Method: "get", URL: "http://recorded.invalid:8080/status?format=json"},
		Setup: []runner.StepResult{{
			Request: runner.Request{Method: "POST", URL: "http://recorded.invalid:8080/session", Body: map[string]any{"role": "reader"}},
		}},
	}
	replayed := recorded
	replayed.Setup = append([]runner.StepResult(nil), recorded.Setup...)
	replayed.Request.URL = "http://replayed.invalid:9090/status?format=json"
	replayed.Setup[0].Request.URL = "http://replayed.invalid:9090/session"

	recordedFingerprint, recordedOK := replayRequestFingerprint(recorded)
	replayedFingerprint, replayedOK := replayRequestFingerprint(replayed)
	if !recordedOK || !replayedOK || recordedFingerprint != replayedFingerprint {
		t.Fatalf("fingerprints = %q, %q, want equal fingerprints across hosts", recordedFingerprint, replayedFingerprint)
	}
}

func TestReplayRequestFingerprintIncludesSequenceAndBody(t *testing.T) {
	base := runner.RuleResult{
		Request: runner.Request{Method: "POST", URL: "http://subject.invalid/documents", Body: map[string]any{"name": "one"}},
		Setup: []runner.StepResult{{
			Request: runner.Request{Method: "POST", URL: "http://subject.invalid/session", Body: map[string]any{"role": "reader"}},
		}},
	}
	baseFingerprint, ok := replayRequestFingerprint(base)
	if !ok {
		t.Fatal("base request fingerprint unavailable")
	}

	bodyChanged := base
	bodyChanged.Request.Body = map[string]any{"name": "two"}
	bodyFingerprint, ok := replayRequestFingerprint(bodyChanged)
	if !ok || bodyFingerprint == baseFingerprint {
		t.Fatalf("body fingerprints = %q, %q, want different fingerprints", baseFingerprint, bodyFingerprint)
	}

	sequenceChanged := base
	sequenceChanged.Setup = append(sequenceChanged.Setup, runner.StepResult{
		Request: runner.Request{Method: "DELETE", URL: "http://subject.invalid/session"},
	})
	sequenceFingerprint, ok := replayRequestFingerprint(sequenceChanged)
	if !ok || sequenceFingerprint == baseFingerprint {
		t.Fatalf("sequence fingerprints = %q, %q, want different fingerprints", baseFingerprint, sequenceFingerprint)
	}
}

func TestReplayRequestFingerprintIsUnavailableWithoutARequest(t *testing.T) {
	if fingerprint, ok := replayRequestFingerprint(runner.RuleResult{}); ok || fingerprint != "" {
		t.Fatalf("fingerprint = %q, available = %v, want unavailable", fingerprint, ok)
	}
}
