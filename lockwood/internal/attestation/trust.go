package attestation

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/custody"
)

const (
	TrustRegistrySchema = "lockwood.attestation-trust/v1"
	KeyActive           = "active"
	KeyRevoked          = "revoked"
)

// TrustedKey is one public key entry in a caller-supplied trust registry.
// Key IDs are immutable governance identifiers; rotation should add a new ID
// and retain the old entry as revoked rather than reusing an ID.
type TrustedKey struct {
	KeyID     string `json:"key_id"`
	Algorithm string `json:"algorithm"`
	PublicKey string `json:"public_key"`
	Status    string `json:"status"`
	NotBefore string `json:"not_before,omitempty"`
	NotAfter  string `json:"not_after,omitempty"`
}

// TrustRegistry is a versioned, canonical description of public keys trusted
// for Lockwood detached record and handling-event signature verification. It
// is not an access-control list and does not identify a human or authorize
// unrelated actions.
type TrustRegistry struct {
	Schema string       `json:"schema"`
	Keys   []TrustedKey `json:"keys"`
}

func (registry TrustRegistry) Validate() error {
	if registry.Schema != TrustRegistrySchema {
		return fmt.Errorf("unexpected attestation trust registry schema %q", registry.Schema)
	}
	if registry.Keys == nil {
		return fmt.Errorf("attestation trust registry keys must be an array")
	}
	seen := make(map[string]struct{}, len(registry.Keys))
	for _, key := range registry.Keys {
		if !keyIDPattern.MatchString(key.KeyID) {
			return fmt.Errorf("invalid attestation trust key id %q", key.KeyID)
		}
		if _, ok := seen[key.KeyID]; ok {
			return fmt.Errorf("duplicate attestation trust key id %q", key.KeyID)
		}
		seen[key.KeyID] = struct{}{}
		if key.Algorithm != Algorithm {
			return fmt.Errorf("unsupported attestation trust algorithm %q", key.Algorithm)
		}
		publicKey, err := decodeTrustedPublicKey(key.PublicKey)
		if err != nil {
			return fmt.Errorf("trust key %q: %w", key.KeyID, err)
		}
		if len(publicKey) != ed25519.PublicKeySize {
			return fmt.Errorf("trust key %q has size %d, want %d", key.KeyID, len(publicKey), ed25519.PublicKeySize)
		}
		switch key.Status {
		case KeyActive, KeyRevoked:
		default:
			return fmt.Errorf("trust key %q has invalid status %q", key.KeyID, key.Status)
		}
		notBefore, err := parseRegistryTime("not_before", key.NotBefore)
		if err != nil {
			return fmt.Errorf("trust key %q: %w", key.KeyID, err)
		}
		notAfter, err := parseRegistryTime("not_after", key.NotAfter)
		if err != nil {
			return fmt.Errorf("trust key %q: %w", key.KeyID, err)
		}
		if !notBefore.IsZero() && !notAfter.IsZero() && !notAfter.After(notBefore) {
			return fmt.Errorf("trust key %q has non-increasing validity window", key.KeyID)
		}
	}
	return nil
}

// Resolve returns the exact public key for keyID when it is active and within
// its validity window at evaluatedAt. Revocation is immediate: revoked keys
// are never trusted, even when evaluating an earlier timestamp. Historical
// attestations remain cryptographically verifiable through Verify.
func (registry TrustRegistry) Resolve(keyID string, evaluatedAt time.Time) (ed25519.PublicKey, error) {
	if err := registry.Validate(); err != nil {
		return nil, err
	}
	if !keyIDPattern.MatchString(keyID) {
		return nil, fmt.Errorf("invalid attestation key id %q", keyID)
	}
	if evaluatedAt.IsZero() {
		return nil, fmt.Errorf("attestation trust evaluation time is required")
	}
	evaluatedAt = evaluatedAt.UTC()
	for _, key := range registry.Keys {
		if key.KeyID != keyID {
			continue
		}
		if key.Status != KeyActive {
			return nil, fmt.Errorf("attestation key id %q is revoked", keyID)
		}
		notBefore, _ := parseRegistryTime("not_before", key.NotBefore)
		notAfter, _ := parseRegistryTime("not_after", key.NotAfter)
		if !notBefore.IsZero() && evaluatedAt.Before(notBefore) {
			return nil, fmt.Errorf("attestation key id %q is not yet valid", keyID)
		}
		if !notAfter.IsZero() && !evaluatedAt.Before(notAfter) {
			return nil, fmt.Errorf("attestation key id %q is expired", keyID)
		}
		decoded, err := decodeTrustedPublicKey(key.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("decode attestation trust key %q: %w", keyID, err)
		}
		return ed25519.PublicKey(decoded), nil
	}
	return nil, fmt.Errorf("attestation key id %q not found in trust registry", keyID)
}

// VerifyWithRegistry verifies the signature and resolves the envelope key
// through an explicit trust registry. It does not grant access or interpret
// the producer's attestation claim.
func VerifyWithRegistry(record custody.Record, envelope Envelope, registry TrustRegistry, evaluatedAt time.Time) error {
	publicKey, err := registry.Resolve(envelope.KeyID, evaluatedAt)
	if err != nil {
		return err
	}
	return Verify(record, envelope, publicKey)
}

// MarshalCanonicalTrustRegistry returns the compact canonical JSON encoding
// for a trust registry. Keys are sorted by key_id so registry identity does
// not depend on input order.
func MarshalCanonicalTrustRegistry(registry TrustRegistry) ([]byte, error) {
	keys := make([]TrustedKey, len(registry.Keys))
	copy(keys, registry.Keys)
	sort.Slice(keys, func(i, j int) bool { return keys[i].KeyID < keys[j].KeyID })
	normalized := registry
	normalized.Keys = keys
	if err := normalized.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(normalized)
}

// UnmarshalCanonicalTrustRegistry decodes one strictly validated registry.
// Unknown fields, multiple values, and noncanonical bytes are rejected.
func UnmarshalCanonicalTrustRegistry(data []byte) (TrustRegistry, error) {
	var registry TrustRegistry
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&registry); err != nil {
		return TrustRegistry{}, fmt.Errorf("decode attestation trust registry: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return TrustRegistry{}, fmt.Errorf("attestation trust registry contains multiple JSON values")
		}
		return TrustRegistry{}, fmt.Errorf("decode attestation trust registry: %w", err)
	}
	canonical, err := MarshalCanonicalTrustRegistry(registry)
	if err != nil {
		return TrustRegistry{}, err
	}
	if !bytes.Equal(data, canonical) {
		return TrustRegistry{}, fmt.Errorf("attestation trust registry is not canonical JSON")
	}
	return registry, nil
}

// TrustRegistryDigest returns the SHA-256 digest of the canonical registry
// representation. It identifies the supplied trust configuration snapshot,
// not a key or an attestation.
func TrustRegistryDigest(registry TrustRegistry) (string, error) {
	encoded, err := MarshalCanonicalTrustRegistry(registry)
	if err != nil {
		return "", err
	}
	return artifact.DigestBytes(encoded), nil
}

func decodeTrustedPublicKey(encoded string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode public key: %w", err)
	}
	if base64.StdEncoding.EncodeToString(decoded) != encoded {
		return nil, fmt.Errorf("public key is not canonical standard-base64")
	}
	if len(decoded) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("public key has size %d, want %d", len(decoded), ed25519.PublicKeySize)
	}
	return decoded, nil
}

func parseRegistryTime(field, value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid %s timestamp %q: %w", field, value, err)
	}
	if parsed.UTC().Format(time.RFC3339) != value {
		return time.Time{}, fmt.Errorf("%s timestamp %q is not canonical RFC3339 UTC", field, value)
	}
	return parsed, nil
}
