// Package ambermessaging provides provenance propagation for message metadata.
package ambermessaging

import (
	"context"
	"fmt"

	amber "github.com/0xsj/ingen/amber"
	ambertransport "github.com/0xsj/ingen/amber/adapters/transport"
)

const ProvenanceKey = ambertransport.ProvenanceField

// Metadata is a message's string-valued metadata collection.
type Metadata map[string]string

// Message is a broker-neutral message whose body is opaque to Amber. The
// middleware only owns the metadata boundary; delivery and acknowledgement
// semantics remain with the application or broker adapter.
type Message[T any] struct {
	Body     T
	Metadata Metadata
}

// Handler processes a message with its scoped provenance context and returns
// the message that should be emitted next.
type Handler[T any] func(context.Context, Message[T]) (Message[T], error)

// DecodeMetadata inspects message metadata without installing it into a
// context. Missing or empty provenance metadata is absent.
func DecodeMetadata(metadata Metadata, policy amber.IncomingPolicy) (amber.Provenance, bool, error) {
	return DecodeMetadataWithValidator(metadata, policy, nil)
}

// DecodeMetadataWithValidator inspects message metadata and applies an
// optional trust validator before returning the value.
func DecodeMetadataWithValidator(metadata Metadata, policy amber.IncomingPolicy, validator amber.IncomingValidator) (amber.Provenance, bool, error) {
	provenance, present, err := ambertransport.DecodeValue(metadata[ProvenanceKey], policy)
	if err != nil || !present || validator == nil {
		return provenance, present, err
	}
	if err := validator(provenance); err != nil {
		if policy == amber.IncomingIgnore {
			return amber.Provenance{}, false, nil
		}
		return amber.Provenance{}, false, fmt.Errorf("%w: incoming validation failed: %v", amber.ErrInvalidProvenance, err)
	}
	return provenance, true, nil
}

// WithIncomingMetadata inspects and, when accepted, installs provenance in a
// derived context. Missing or ignored metadata preserves the original context.
func WithIncomingMetadata(ctx context.Context, metadata Metadata, policy amber.IncomingPolicy) (context.Context, bool, error) {
	return WithIncomingMetadataWithValidator(ctx, metadata, policy, nil)
}

// WithIncomingMetadataWithValidator installs metadata only after the optional
// trust validator accepts the decoded provenance.
func WithIncomingMetadataWithValidator(ctx context.Context, metadata Metadata, policy amber.IncomingPolicy, validator amber.IncomingValidator) (context.Context, bool, error) {
	if ctx == nil {
		return nil, false, fmt.Errorf("%w: context cannot be nil", amber.ErrInvalidTransition)
	}
	provenance, present, err := DecodeMetadataWithValidator(metadata, policy, validator)
	if err != nil || !present {
		return ctx, present, err
	}
	installed, err := amber.WithProvenance(ctx, provenance)
	if err != nil {
		return ctx, false, err
	}
	return installed, true, nil
}

// WithOutgoingMetadata returns a cloned metadata map containing provenance.
// The input map is never mutated, including when it is nil.
func WithOutgoingMetadata(metadata Metadata, provenance amber.Provenance) (Metadata, error) {
	value, err := ambertransport.EncodeValue(provenance)
	if err != nil {
		return nil, err
	}
	clone := cloneMetadata(metadata)
	if clone == nil {
		clone = make(Metadata, 1)
	}
	clone[ProvenanceKey] = value
	return clone, nil
}

// WithIncomingMessage installs accepted metadata in a derived context and
// gives the handler a cloned metadata map so it cannot mutate the caller's
// reusable message metadata by accident.
func WithIncomingMessage[T any](ctx context.Context, message Message[T], policy amber.IncomingPolicy) (context.Context, Message[T], bool, error) {
	return WithIncomingMessageWithValidator(ctx, message, policy, nil)
}

// WithIncomingMessageWithValidator installs message provenance after the
// optional trust validator accepts it and clones metadata for the consumer.
func WithIncomingMessageWithValidator[T any](ctx context.Context, message Message[T], policy amber.IncomingPolicy, validator amber.IncomingValidator) (context.Context, Message[T], bool, error) {
	derived, present, err := WithIncomingMetadataWithValidator(ctx, message.Metadata, policy, validator)
	if err != nil {
		return ctx, message, false, err
	}
	message.Metadata = cloneMetadata(message.Metadata)
	return derived, message, present, nil
}

// WithOutgoingMessage returns a message with cloned metadata containing
// provenance. The message body is passed through unchanged.
func WithOutgoingMessage[T any](message Message[T], provenance amber.Provenance) (Message[T], error) {
	metadata, err := WithOutgoingMetadata(message.Metadata, provenance)
	if err != nil {
		return Message[T]{}, err
	}
	message.Metadata = metadata
	return message, nil
}

// Middleware creates a broker-neutral message handler with Amber context
// installation and outgoing metadata propagation. Rejected input returns an
// error and does not reach next; ignored input continues without provenance.
func Middleware[T any](next Handler[T], policy amber.IncomingPolicy) Handler[T] {
	return MiddlewareWithValidator(next, policy, nil)
}

// MiddlewareWithValidator creates a message handler with an optional trust
// validator applied before the consumer runs.
func MiddlewareWithValidator[T any](next Handler[T], policy amber.IncomingPolicy, validator amber.IncomingValidator) Handler[T] {
	if next == nil {
		panic("ambermessaging: next handler cannot be nil")
	}

	return func(ctx context.Context, message Message[T]) (Message[T], error) {
		incomingContext, incomingMessage, _, err := WithIncomingMessageWithValidator(ctx, message, policy, validator)
		if err != nil {
			return Message[T]{}, err
		}
		outgoing, err := next(incomingContext, incomingMessage)
		if err != nil {
			return Message[T]{}, err
		}

		provenance, ok := amber.ProvenanceFromContext(incomingContext)
		if !ok {
			return outgoing, nil
		}
		return WithOutgoingMessage(outgoing, provenance)
	}
}

func cloneMetadata(metadata Metadata) Metadata {
	if metadata == nil {
		return nil
	}
	clone := make(Metadata, len(metadata))
	for key, item := range metadata {
		clone[key] = item
	}
	return clone
}
