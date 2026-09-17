package governance

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"strings"
)

const AuthoritySignatureAlgorithmEd25519 = "ed25519"

// AuthoritySignatureSigner creates an issuer signature over a canonical
// authority or membership payload. Key custody and rotation remain caller-owned.
type AuthoritySignatureSigner interface {
	Sign(payload []byte) (AuthoritySignature, error)
}

// Ed25519AuthoritySignatureSigner is a small caller-owned signing adapter.
// Hammond retains no copy of the private key beyond this value's lifetime.
type Ed25519AuthoritySignatureSigner struct {
	KeyID      string
	PrivateKey ed25519.PrivateKey
}

func (signer Ed25519AuthoritySignatureSigner) Sign(payload []byte) (AuthoritySignature, error) {
	if strings.TrimSpace(signer.KeyID) == "" {
		return AuthoritySignature{}, fmt.Errorf("authority signing key_id is required")
	}
	if len(signer.PrivateKey) != ed25519.PrivateKeySize {
		return AuthoritySignature{}, fmt.Errorf("authority signing private key has invalid length")
	}
	return AuthoritySignature{
		Algorithm: AuthoritySignatureAlgorithmEd25519,
		KeyID:     signer.KeyID,
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(signer.PrivateKey, payload)),
	}, nil
}

// AuthoritySignatureVerifier verifies an authority artifact's issuer
// signature against a caller-owned trust set.
type AuthoritySignatureVerifier interface {
	VerifySignature(keyID string, payload, signature []byte) error
}

// Ed25519AuthoritySignatureVerifier is a small trust-store adapter. Key
// distribution, rotation, and revocation remain the caller's responsibility.
type Ed25519AuthoritySignatureVerifier struct {
	Keys map[string]ed25519.PublicKey
}

func (verifier Ed25519AuthoritySignatureVerifier) VerifySignature(keyID string, payload, signature []byte) error {
	key, exists := verifier.Keys[keyID]
	if !exists {
		return fmt.Errorf("trusted authority key %q is not configured", keyID)
	}
	if len(key) != ed25519.PublicKeySize {
		return fmt.Errorf("trusted authority key %q has invalid length", keyID)
	}
	if !ed25519.Verify(key, payload, signature) {
		return fmt.Errorf("authority signature is invalid")
	}
	return nil
}

func decodeAuthoritySignature(value string) ([]byte, error) {
	signature, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("authority signature is not valid base64: %w", err)
	}
	if len(signature) != ed25519.SignatureSize {
		return nil, fmt.Errorf("authority signature has invalid length")
	}
	return signature, nil
}

func validateAuthoritySignature(signature *AuthoritySignature) error {
	if signature == nil {
		return nil
	}
	if signature.Algorithm != AuthoritySignatureAlgorithmEd25519 {
		return fmt.Errorf("authority signature algorithm must be %s", AuthoritySignatureAlgorithmEd25519)
	}
	if strings.TrimSpace(signature.KeyID) == "" {
		return fmt.Errorf("authority signature key_id is required")
	}
	_, err := decodeAuthoritySignature(signature.Signature)
	return err
}
