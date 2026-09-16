package governance

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"strings"
)

const AuthoritySignatureAlgorithmEd25519 = "ed25519"

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
