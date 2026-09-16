package attestation

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/custody"
)

const (
	Schema     = "lockwood.attestation/v1"
	Algorithm  = "ed25519"
	TargetKind = "custody-record"
)

var keyIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,255}$`)

type Target struct {
	Kind   string `json:"kind"`
	Digest string `json:"digest"`
}

// Envelope is a detached signature over a canonical custody-record identity.
// Key trust and authorization are deliberately supplied by the caller.
type Envelope struct {
	Schema    string `json:"schema"`
	Target    Target `json:"target"`
	Algorithm string `json:"algorithm"`
	KeyID     string `json:"key_id"`
	Signature string `json:"signature"`
}

// PublicKeySet is supplied by the caller that owns key trust and authorization.
// Lockwood only uses it for exact key_id lookup.
type PublicKeySet map[string]ed25519.PublicKey

func (envelope Envelope) Validate() error {
	if envelope.Schema != Schema {
		return fmt.Errorf("unexpected attestation schema %q", envelope.Schema)
	}
	if envelope.Target.Kind != TargetKind {
		return fmt.Errorf("unexpected attestation target kind %q", envelope.Target.Kind)
	}
	if err := artifact.ValidateDigest(envelope.Target.Digest); err != nil {
		return fmt.Errorf("invalid attestation target: %w", err)
	}
	if envelope.Algorithm != Algorithm {
		return fmt.Errorf("unsupported attestation algorithm %q", envelope.Algorithm)
	}
	if !keyIDPattern.MatchString(envelope.KeyID) {
		return fmt.Errorf("invalid attestation key id %q", envelope.KeyID)
	}
	signature, err := base64.StdEncoding.DecodeString(envelope.Signature)
	if err != nil {
		return fmt.Errorf("decode attestation signature: %w", err)
	}
	if len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("attestation signature has size %d, want %d", len(signature), ed25519.SignatureSize)
	}
	return nil
}

// SigningMessage returns the domain-separated bytes signed for a record and
// key identifier. The final newline is part of the signing payload.
func SigningMessage(record custody.Record, keyID string) ([]byte, error) {
	digest, err := custody.CanonicalDigest(record)
	if err != nil {
		return nil, err
	}
	return signingMessage(digest, keyID)
}

func Sign(record custody.Record, keyID string, privateKey ed25519.PrivateKey) (Envelope, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return Envelope{}, fmt.Errorf("attestation private key has size %d, want %d", len(privateKey), ed25519.PrivateKeySize)
	}
	digest, err := custody.CanonicalDigest(record)
	if err != nil {
		return Envelope{}, err
	}
	message, err := signingMessage(digest, keyID)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{
		Schema:    Schema,
		Target:    Target{Kind: TargetKind, Digest: digest},
		Algorithm: Algorithm,
		KeyID:     keyID,
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, message)),
	}, nil
}

func Verify(record custody.Record, envelope Envelope, publicKey ed25519.PublicKey) error {
	if err := envelope.Validate(); err != nil {
		return err
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("attestation public key has size %d, want %d", len(publicKey), ed25519.PublicKeySize)
	}
	canonicalDigest, err := custody.CanonicalDigest(record)
	if err != nil {
		return err
	}
	if envelope.Target.Digest != canonicalDigest {
		return fmt.Errorf("attestation target digest mismatch: got %s, want %s", envelope.Target.Digest, canonicalDigest)
	}
	message, err := signingMessage(canonicalDigest, envelope.KeyID)
	if err != nil {
		return err
	}
	signature, err := base64.StdEncoding.DecodeString(envelope.Signature)
	if err != nil {
		return fmt.Errorf("decode attestation signature: %w", err)
	}
	if !ed25519.Verify(publicKey, message, signature) {
		return fmt.Errorf("attestation signature verification failed")
	}
	return nil
}

// VerifyWithKeySet resolves the envelope key_id against a caller-supplied
// public-key set and then performs cryptographic verification. The key set is
// not a Lockwood trust registry; callers remain responsible for authorization,
// rotation, and revocation policy.
func VerifyWithKeySet(record custody.Record, envelope Envelope, keys PublicKeySet) error {
	if err := envelope.Validate(); err != nil {
		return err
	}
	publicKey, ok := keys[envelope.KeyID]
	if !ok {
		return fmt.Errorf("attestation key id %q not found in supplied key set", envelope.KeyID)
	}
	return Verify(record, envelope, publicKey)
}

func signingMessage(digest, keyID string) ([]byte, error) {
	if !keyIDPattern.MatchString(keyID) {
		return nil, fmt.Errorf("invalid attestation key id %q", keyID)
	}
	message := strings.Join([]string{Schema, TargetKind, keyID, digest, ""}, "\n")
	return []byte(message), nil
}
