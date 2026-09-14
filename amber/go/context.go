package amber

import (
	"context"
	"fmt"
)

// provenanceContextKey is private so unrelated context users cannot collide
// with Amber's value.
type provenanceContextKey struct{}

// WithProvenance returns a derived context containing provenance. The input
// value is validated before it is installed. Keeping the parent context gives
// callers an exact, immutable restoration point.
func WithProvenance(ctx context.Context, provenance Provenance) (context.Context, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: context cannot be nil", ErrInvalidTransition)
	}
	if err := provenance.Validate(); err != nil {
		return nil, err
	}
	return context.WithValue(ctx, provenanceContextKey{}, provenance), nil
}

// ProvenanceFromContext inspects a context without changing it. Invalid or
// absent values are treated as absent.
func ProvenanceFromContext(ctx context.Context) (Provenance, bool) {
	if ctx == nil {
		return Provenance{}, false
	}
	value, ok := ctx.Value(provenanceContextKey{}).(Provenance)
	if !ok || value.Validate() != nil {
		return Provenance{}, false
	}
	return value, true
}
