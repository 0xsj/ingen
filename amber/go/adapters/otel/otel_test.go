package amberotel

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	amber "github.com/0xsj/ingen/amber"
)

type recordingSpan struct {
	attributes map[string]attribute.Value
}

func (span *recordingSpan) SetAttributes(values ...attribute.KeyValue) {
	if span.attributes == nil {
		span.attributes = make(map[string]attribute.Value)
	}
	for _, value := range values {
		span.attributes[string(value.Key)] = value.Value
	}
}

type recordingTraceSpan struct {
	trace.Span
	recorder *recordingSpan
}

func (span *recordingTraceSpan) SetAttributes(values ...attribute.KeyValue) {
	span.recorder.SetAttributes(values...)
}

func TestToAttributesUsesStableAmberKeys(t *testing.T) {
	root, err := amber.Start()
	if err != nil {
		t.Fatal(err)
	}
	attributes, err := ToAttributes(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(attributes) != 8 {
		t.Fatalf("root produced %d attributes, want 8", len(attributes))
	}
	for _, value := range attributes {
		if !value.Valid() {
			t.Fatalf("invalid OpenTelemetry attribute: %#v", value)
		}
	}

	span := &recordingSpan{}
	if err := SetAttributes(span, root); err != nil {
		t.Fatal(err)
	}
	if got := span.attributes["amber.execution_id"].AsString(); got != root.ExecutionID().String() {
		t.Fatalf("execution attribute=%q, want %q", got, root.ExecutionID())
	}
	if got := span.attributes["amber.depth"].AsInt64(); got != 0 {
		t.Fatalf("depth attribute=%d, want 0", got)
	}
}

func TestToAttributesProjectsRetryAndCausation(t *testing.T) {
	root, err := amber.Start()
	if err != nil {
		t.Fatal(err)
	}
	retry, err := root.Retry()
	if err != nil {
		t.Fatal(err)
	}
	attributes, err := ToAttributes(retry)
	if err != nil {
		t.Fatal(err)
	}
	span := &recordingSpan{}
	span.SetAttributes(attributes...)
	if got := span.attributes["amber.mode.kind"].AsString(); got != string(amber.ModeRetry) {
		t.Fatalf("mode attribute=%q, want %q", got, amber.ModeRetry)
	}
	if got := span.attributes["amber.causation.id"].AsString(); got != root.ExecutionID().String() {
		t.Fatalf("causation attribute=%q, want %q", got, root.ExecutionID())
	}
}

func TestSetCurrentSpanAttributesAcceptsNoopAndRejectsNilContext(t *testing.T) {
	root, err := amber.Start()
	if err != nil {
		t.Fatal(err)
	}
	if err := SetCurrentSpanAttributes(context.Background(), root); err != nil {
		t.Fatalf("no-op span should be safe: %v", err)
	}
	if err := SetCurrentSpanAttributes(nil, root); err == nil {
		t.Fatal("nil context must be rejected")
	}
}

func TestSetCurrentSpanAttributesUsesContextSpan(t *testing.T) {
	root, err := amber.Start()
	if err != nil {
		t.Fatal(err)
	}
	recorder := &recordingSpan{}
	span := &recordingTraceSpan{recorder: recorder}
	ctx := trace.ContextWithSpan(context.Background(), span)
	if err := SetCurrentSpanAttributes(ctx, root); err != nil {
		t.Fatal(err)
	}
	if got := recorder.attributes["amber.execution_id"].AsString(); got != root.ExecutionID().String() {
		t.Fatalf("context span execution attribute=%q, want %q", got, root.ExecutionID())
	}
}

func TestOTelConformanceFixture(t *testing.T) {
	data, err := os.ReadFile("../../../conformance/otel-v1.json")
	if err != nil {
		t.Fatalf("read OTel fixture: %v", err)
	}
	var fixture struct {
		Version  int             `json:"version"`
		Value    json.RawMessage `json:"value"`
		Expected map[string]any  `json:"expected"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode OTel fixture: %v", err)
	}
	if fixture.Version != amber.Version {
		t.Fatalf("fixture version = %d, want %d", fixture.Version, amber.Version)
	}
	provenance, err := amber.FromJSON(fixture.Value)
	if err != nil {
		t.Fatalf("decode provenance fixture: %v", err)
	}
	attributes, err := ToAttributes(provenance)
	if err != nil {
		t.Fatal(err)
	}
	actual := make(map[string]any, len(attributes))
	for _, value := range attributes {
		key := string(value.Key)
		switch key {
		case "amber.version", "amber.depth", "amber.attempt":
			actual[key] = value.Value.AsInt64()
		default:
			actual[key] = value.Value.AsString()
		}
	}
	actualJSON, err := json.Marshal(actual)
	if err != nil {
		t.Fatal(err)
	}
	expectedJSON, err := json.Marshal(fixture.Expected)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actualJSON, expectedJSON) {
		t.Fatalf("attributes mismatch:\ngot:  %s\nwant: %s", actualJSON, expectedJSON)
	}
}
