package amber

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestStartCreatesValidRoot(t *testing.T) {
	root, err := Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := root.Validate(); err != nil {
		t.Fatalf("root validation error = %v", err)
	}
	if root.WorkID() == root.ExecutionID() {
		t.Fatal("work_id and execution_id must differ")
	}
	if root.Origin() != OriginLocal || root.Mode().Kind != ModeNormal {
		t.Fatalf("unexpected root origin/mode: %q/%q", root.Origin(), root.Mode().Kind)
	}
	if root.Depth() != 0 || root.Attempt() != 1 {
		t.Fatalf("unexpected root depth/attempt: %d/%d", root.Depth(), root.Attempt())
	}
}

func TestChildCreatesNewWorkAndPreservesFlow(t *testing.T) {
	root, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	child, err := root.Child()
	if err != nil {
		t.Fatalf("Child() error = %v", err)
	}
	if child.WorkID() == root.WorkID() || child.ExecutionID() == root.ExecutionID() {
		t.Fatal("child must have fresh work and execution IDs")
	}
	if child.CorrelationID() != root.CorrelationID() {
		t.Fatal("child must preserve correlation_id")
	}
	if child.Depth() != 1 || child.Attempt() != 1 {
		t.Fatalf("unexpected child depth/attempt: %d/%d", child.Depth(), child.Attempt())
	}
	cause, ok := child.Causation()
	if !ok || cause.Kind != "execution" || cause.ID != root.ExecutionID() {
		t.Fatalf("unexpected child causation: %#v", cause)
	}
}

func TestRetryAndReplayPreserveWorkButCreateExecutions(t *testing.T) {
	root, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	retry, err := root.Retry()
	if err != nil {
		t.Fatalf("Retry() error = %v", err)
	}
	if retry.WorkID() != root.WorkID() || retry.ExecutionID() == root.ExecutionID() {
		t.Fatal("retry must preserve work_id and create execution_id")
	}
	if retry.Attempt() != 2 || retry.Mode().Kind != ModeRetry || retry.Origin() != OriginRetry {
		t.Fatalf("unexpected retry state: %#v", retry)
	}

	replay, err := root.Replay()
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if replay.WorkID() != root.WorkID() || replay.ExecutionID() == root.ExecutionID() {
		t.Fatal("replay must preserve work_id and create execution_id")
	}
	if replay.Attempt() != root.Attempt() || replay.Mode().Kind != ModeReplay || replay.Origin() != OriginReplay {
		t.Fatalf("unexpected replay state: %#v", replay)
	}
	if replay.Mode().ReplayID == "" || replay.Mode().ReplayID == replay.ExecutionID() {
		t.Fatal("replay must have a fresh replay_id")
	}
}

func TestJSONRoundTrip(t *testing.T) {
	original, err := Start(StartOptions{
		Attribution: &Attribution{
			InitiatedBy: &Actor{ID: "user-42", Type: "user"},
			ExecutedBy:  &Actor{ID: "worker", Type: "service"},
			TenantID:    "tenant-7",
		},
		References: []Reference{{Type: "invoice", ID: "invoice-123", Relation: "subject"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("decode emitted JSON: %v", err)
	}
	attribution, ok := wire["attribution"].(map[string]any)
	if !ok || attribution["initiated_by"] == nil || attribution["executed_by"] == nil {
		t.Fatalf("JSON must use canonical attribution field names: %s", data)
	}
	decoded, err := FromJSON(data)
	if err != nil {
		t.Fatalf("FromJSON() error = %v", err)
	}
	decodedData, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("second MarshalJSON() error = %v", err)
	}
	if string(data) != string(decodedData) {
		t.Fatalf("round trip changed JSON:\n%s\n%s", data, decodedData)
	}
}

func TestInvalidJSONIsRejected(t *testing.T) {
	_, err := FromJSON([]byte(`{"version":1,"work_id":"bad"}`))
	if err == nil {
		t.Fatal("expected malformed provenance to be rejected")
	}
}

func TestUnknownJSONFieldsAreAcceptedAndOmittedOnReencode(t *testing.T) {
	root, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	wire["future_field"] = "accepted-and-ignored"
	mode := wire["mode"].(map[string]any)
	mode["future_mode_field"] = true
	withUnknown, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := FromJSON(withUnknown)
	if err != nil {
		t.Fatalf("unknown fields should be accepted: %v", err)
	}
	reencoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(reencoded, []byte("future_field")) || bytes.Contains(reencoded, []byte("future_mode_field")) {
		t.Fatalf("unknown fields should be omitted by the non-lossless SDK: %s", reencoded)
	}
}
