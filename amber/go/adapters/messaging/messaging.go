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

// DecodeMetadata inspects message metadata without installing it into a
// context. Missing or empty provenance metadata is absent.
func DecodeMetadata(metadata Metadata, policy amber.IncomingPolicy) (amber.Provenance, bool, error) {
	return ambertransport.DecodeValue(metadata[ProvenanceKey], policy)
}

// WithIncomingMetadata inspects and, when accepted, installs provenance in a
// derived context. Missing or ignored metadata preserves the original context.
func WithIncomingMetadata(ctx context.Context, metadata Metadata, policy amber.IncomingPolicy) (context.Context, bool, error) {
	if ctx == nil {
		return nil, false, fmt.Errorf("%w: context cannot be nil", amber.ErrInvalidTransition)
	}
	provenance, present, err := DecodeMetadata(metadata, policy)
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
	clone := make(Metadata, len(metadata)+1)
	for key, item := range metadata {
		clone[key] = item
	}
	clone[ProvenanceKey] = value
	return clone, nil
}
