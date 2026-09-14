package amber

import (
	"context"
	"fmt"
)

const MaxIncomingJSONBytes = 16 * 1024

// IncomingPolicy controls how an adapter handles malformed incoming data.
type IncomingPolicy string

const (
	IncomingReject IncomingPolicy = "reject"
	IncomingIgnore IncomingPolicy = "ignore"
)

func validateIncomingPolicy(policy IncomingPolicy) error {
	if policy != IncomingReject && policy != IncomingIgnore {
		return fmt.Errorf("%w: unsupported incoming policy %q", ErrInvalidTransition, policy)
	}
	return nil
}

// InspectIncomingJSON validates an incoming JSON value without installing it
// into any context. Empty input is absent; non-empty invalid input is either
// rejected or treated as absent according to policy.
func InspectIncomingJSON(data []byte, policy IncomingPolicy) (Provenance, bool, error) {
	if err := validateIncomingPolicy(policy); err != nil {
		return Provenance{}, false, err
	}
	if len(data) == 0 {
		return Provenance{}, false, nil
	}
	if len(data) > MaxIncomingJSONBytes {
		return incomingFailure(policy, fmt.Errorf("incoming JSON exceeds %d bytes", MaxIncomingJSONBytes))
	}

	provenance, err := FromJSON(data)
	if err != nil {
		return incomingFailure(policy, err)
	}
	return provenance, true, nil
}

// WithIncomingJSON inspects and, when accepted, installs an incoming value in
// a derived context. Absent or ignored input returns the original context.
func WithIncomingJSON(ctx context.Context, data []byte, policy IncomingPolicy) (context.Context, bool, error) {
	if ctx == nil {
		return nil, false, fmt.Errorf("%w: context cannot be nil", ErrInvalidTransition)
	}
	provenance, present, err := InspectIncomingJSON(data, policy)
	if err != nil || !present {
		return ctx, present, err
	}
	installed, err := WithProvenance(ctx, provenance)
	if err != nil {
		return ctx, false, err
	}
	return installed, true, nil
}

func incomingFailure(policy IncomingPolicy, err error) (Provenance, bool, error) {
	if policy == IncomingIgnore {
		return Provenance{}, false, nil
	}
	return Provenance{}, false, fmt.Errorf("%w: %v", ErrInvalidProvenance, err)
}
