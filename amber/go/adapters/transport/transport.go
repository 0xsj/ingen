// Package ambertransport contains transport-neutral provenance value encoding.
package ambertransport

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	amber "github.com/0xsj/ingen/amber"
)

const (
	ProvenanceField      = "Amber-Provenance"
	MaxEncodedValueBytes = 4 * ((amber.MaxIncomingJSONBytes + 2) / 3)
)

// EncodeValue serializes provenance as canonical UTF-8 JSON and encodes it as
// unpadded base64url for transport metadata.
func EncodeValue(provenance amber.Provenance) (string, error) {
	data, err := json.Marshal(provenance)
	if err != nil {
		return "", err
	}
	if len(data) > amber.MaxIncomingJSONBytes {
		return "", fmt.Errorf("%w: encoded JSON exceeds %d bytes", amber.ErrInvalidProvenance, amber.MaxIncomingJSONBytes)
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

// DecodeValue decodes and validates transport metadata without installing it
// into a context. Empty input is absent; malformed input follows policy.
func DecodeValue(value string, policy amber.IncomingPolicy) (amber.Provenance, bool, error) {
	if value == "" {
		if err := validatePolicy(policy); err != nil {
			return amber.Provenance{}, false, err
		}
		return amber.Provenance{}, false, nil
	}
	if len(value) > MaxEncodedValueBytes {
		return decodeFailure(policy, fmt.Errorf("value exceeds %d encoded bytes", MaxEncodedValueBytes))
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return decodeFailure(policy, fmt.Errorf("invalid base64url value: %v", err))
	}
	return amber.InspectIncomingJSON(data, policy)
}

func validatePolicy(policy amber.IncomingPolicy) error {
	if policy != amber.IncomingReject && policy != amber.IncomingIgnore {
		return fmt.Errorf("%w: unsupported incoming policy %q", amber.ErrInvalidTransition, policy)
	}
	return nil
}

func decodeFailure(policy amber.IncomingPolicy, err error) (amber.Provenance, bool, error) {
	if policy == amber.IncomingIgnore {
		return amber.Provenance{}, false, nil
	}
	if err := validatePolicy(policy); err != nil {
		return amber.Provenance{}, false, err
	}
	return amber.Provenance{}, false, fmt.Errorf("%w: transport value: %v", amber.ErrInvalidProvenance, err)
}
