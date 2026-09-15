// Package amberotel provides an optional OpenTelemetry span adapter.
package amberotel

import (
	"context"
	"fmt"
	"math"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	amber "github.com/0xsj/ingen/amber"
)

// Span is the part of an OpenTelemetry span needed by Amber. The interface
// keeps the adapter usable with recording, test, and no-op spans.
type Span interface {
	SetAttributes(...attribute.KeyValue)
}

// ToAttributes converts Amber's stable tracing projection into OpenTelemetry
// attributes. Depth and attempt remain numeric and therefore must fit the
// OpenTelemetry int64 attribute representation.
func ToAttributes(provenance amber.Provenance) ([]attribute.KeyValue, error) {
	if err := provenance.Validate(); err != nil {
		return nil, err
	}
	depth, err := int64Attribute("depth", provenance.Depth())
	if err != nil {
		return nil, err
	}
	attempt, err := int64Attribute("attempt", provenance.Attempt())
	if err != nil {
		return nil, err
	}

	attributes := []attribute.KeyValue{
		attribute.Int("amber.version", provenance.Version()),
		attribute.String("amber.work_id", provenance.WorkID().String()),
		attribute.String("amber.execution_id", provenance.ExecutionID().String()),
		attribute.String("amber.correlation_id", provenance.CorrelationID().String()),
		attribute.String("amber.origin", string(provenance.Origin())),
		attribute.Int64("amber.depth", depth),
		attribute.Int64("amber.attempt", attempt),
		attribute.String("amber.mode.kind", string(provenance.Mode().Kind)),
	}

	mode := provenance.Mode()
	if mode.OfExecutionID != "" {
		attributes = append(attributes, attribute.String("amber.mode.of_execution_id", mode.OfExecutionID.String()))
	}
	if mode.ReplayID != "" {
		attributes = append(attributes, attribute.String("amber.mode.replay_id", mode.ReplayID.String()))
	}
	if causation, ok := provenance.Causation(); ok {
		attributes = append(attributes,
			attribute.String("amber.causation.kind", causation.Kind),
			attribute.String("amber.causation.id", causation.ID.String()),
		)
		if causation.Type != "" {
			attributes = append(attributes, attribute.String("amber.causation.type", causation.Type))
		}
	}
	return attributes, nil
}

// SetAttributes adds Amber attributes to an existing span. It never starts,
// ends, or replaces a span.
func SetAttributes(span Span, provenance amber.Provenance) error {
	if span == nil {
		return fmt.Errorf("%w: span cannot be nil", amber.ErrInvalidTransition)
	}
	attributes, err := ToAttributes(provenance)
	if err != nil {
		return err
	}
	span.SetAttributes(attributes...)
	return nil
}

// SetCurrentSpanAttributes adds Amber attributes to the span explicitly
// stored in ctx. A context without a span uses OpenTelemetry's no-op span and
// is therefore safe to call in applications where tracing is disabled.
func SetCurrentSpanAttributes(ctx context.Context, provenance amber.Provenance) error {
	if ctx == nil {
		return fmt.Errorf("%w: context cannot be nil", amber.ErrInvalidTransition)
	}
	return SetAttributes(trace.SpanFromContext(ctx), provenance)
}

func int64Attribute(name string, value uint64) (int64, error) {
	if value > math.MaxInt64 {
		return 0, fmt.Errorf("%w: %s %d exceeds OpenTelemetry int64 range", amber.ErrInvalidProvenance, name, value)
	}
	return int64(value), nil
}
