package attestation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"ingen/lockwood/internal/artifact"
)

// MarshalCanonical returns the compact local JSON encoding for an attestation
// envelope. It uses the declared struct field order and has no trailing
// newline; this is the byte representation intended for immutable storage.
func MarshalCanonical(envelope Envelope) ([]byte, error) {
	if err := envelope.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(envelope)
}

// UnmarshalCanonical decodes one strictly validated, canonical attestation
// envelope. Unknown fields, multiple JSON values, and formatting changes are
// rejected so the stored bytes have one local representation.
func UnmarshalCanonical(data []byte) (Envelope, error) {
	var envelope Envelope
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return Envelope{}, fmt.Errorf("decode attestation envelope: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Envelope{}, fmt.Errorf("attestation envelope contains multiple JSON values")
		}
		return Envelope{}, fmt.Errorf("decode attestation envelope: %w", err)
	}
	canonical, err := MarshalCanonical(envelope)
	if err != nil {
		return Envelope{}, err
	}
	if !bytes.Equal(data, canonical) {
		return Envelope{}, fmt.Errorf("attestation envelope is not canonical JSON")
	}
	return envelope, nil
}

// CanonicalDigest returns the SHA-256 digest of an attestation envelope's
// canonical JSON bytes. This identifies the detached envelope artifact; it is
// distinct from envelope.Target.Digest, which identifies the custody record.
func CanonicalDigest(envelope Envelope) (string, error) {
	encoded, err := MarshalCanonical(envelope)
	if err != nil {
		return "", err
	}
	return artifact.DigestBytes(encoded), nil
}
