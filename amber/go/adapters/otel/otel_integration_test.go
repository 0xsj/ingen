package amberotel

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	amber "github.com/0xsj/ingen/amber"
)

func TestSDKRecordsAmberAttributesOnExistingSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(recorder),
	)
	defer func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Errorf("shut down tracer provider: %v", err)
		}
	}()

	ctx, span := provider.Tracer("amber/otel-integration").Start(context.Background(), "process-order")
	provenance, err := amber.Start()
	if err != nil {
		t.Fatal(err)
	}
	if err := SetCurrentSpanAttributes(ctx, provenance); err != nil {
		t.Fatalf("set current span attributes: %v", err)
	}
	span.End()

	ended := recorder.Ended()
	if len(ended) != 1 {
		t.Fatalf("recorded %d ended spans, want 1", len(ended))
	}
	attributes := attributesByKey(ended[0].Attributes())
	if got := attributes["amber.execution_id"].AsString(); got != provenance.ExecutionID().String() {
		t.Fatalf("execution attribute=%q, want %q", got, provenance.ExecutionID())
	}
	if got := attributes["amber.depth"].AsInt64(); got != 0 {
		t.Fatalf("depth attribute=%d, want 0", got)
	}
	if got := attributes["amber.mode.kind"].AsString(); got != string(amber.ModeNormal) {
		t.Fatalf("mode attribute=%q, want %q", got, amber.ModeNormal)
	}
}

func attributesByKey(values []attribute.KeyValue) map[string]attribute.Value {
	attributes := make(map[string]attribute.Value, len(values))
	for _, value := range values {
		attributes[string(value.Key)] = value.Value
	}
	return attributes
}
