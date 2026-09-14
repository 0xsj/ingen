package amber

import (
	"context"
	"encoding/json"
	"testing"
)

func TestIncomingInspectionPolicies(t *testing.T) {
	root, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}

	accepted, present, err := InspectIncomingJSON(data, IncomingReject)
	if err != nil || !present || accepted.ExecutionID() != root.ExecutionID() {
		t.Fatalf("valid incoming value was not accepted: present=%v err=%v", present, err)
	}
	if _, present, err := InspectIncomingJSON([]byte(`{"version":1}`), IncomingIgnore); err != nil || present {
		t.Fatalf("invalid ignored input should be absent: present=%v err=%v", present, err)
	}
	if _, present, err := InspectIncomingJSON([]byte(`{"version":1}`), IncomingReject); err == nil || present {
		t.Fatalf("invalid rejected input should return an error: present=%v err=%v", present, err)
	}
	oversized := make([]byte, MaxIncomingJSONBytes+1)
	if _, present, err := InspectIncomingJSON(oversized, IncomingIgnore); err != nil || present {
		t.Fatalf("oversized ignored input should be absent: present=%v err=%v", present, err)
	}
	if _, present, err := InspectIncomingJSON(oversized, IncomingReject); err == nil || present {
		t.Fatalf("oversized rejected input should return an error: present=%v err=%v", present, err)
	}
}

func TestWithIncomingJSONDoesNotOverwriteOnAbsentInput(t *testing.T) {
	root, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	base := context.Background()
	installed, present, err := WithIncomingJSON(base, data, IncomingReject)
	if err != nil || !present {
		t.Fatalf("valid incoming context was not installed: present=%v err=%v", present, err)
	}
	unchanged, present, err := WithIncomingJSON(installed, nil, IncomingReject)
	if err != nil || present || unchanged != installed {
		t.Fatalf("absent input must preserve the original context: present=%v err=%v", present, err)
	}
	current, ok := ProvenanceFromContext(unchanged)
	if !ok || current.ExecutionID() != root.ExecutionID() {
		t.Fatal("existing context was overwritten or lost")
	}
}
