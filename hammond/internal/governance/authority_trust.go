package governance

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	AuthorityTrustKeyActive  = "active"
	AuthorityTrustKeyRevoked = "revoked"
)

type authorityTrustKeyDocument struct {
	ID        string `json:"id"`
	Algorithm string `json:"algorithm"`
	PublicKey string `json:"public_key"`
	Status    string `json:"status"`
}

type authorityTrustDocument struct {
	Schema    string                      `json:"schema"`
	ID        string                      `json:"id"`
	Version   int                         `json:"version"`
	Keys      []authorityTrustKeyDocument `json:"keys"`
	Signature *AuthoritySignature         `json:"signature,omitempty"`
}

type authorityTrustSigningDocument struct {
	Schema  string                      `json:"schema"`
	ID      string                      `json:"id"`
	Version int                         `json:"version"`
	Keys    []authorityTrustKeyDocument `json:"keys"`
}

// AuthorityTrustStore contains only active public keys. Revoked keys are
// deliberately omitted from its verifier, so rotation fails closed for old
// signatures.
type AuthorityTrustStore struct {
	Reference AuthorityTrustReference
	Keys      map[string]ed25519.PublicKey
	Signature *AuthoritySignature
}

func (store AuthorityTrustStore) Validate() error {
	if problems := validateAuthorityTrustReference(store.Reference, "trust"); len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	if err := validateAuthoritySignature(store.Signature); err != nil {
		return err
	}
	if store.Keys == nil {
		return fmt.Errorf("authority trust keys are required")
	}
	for keyID, publicKey := range store.Keys {
		if strings.TrimSpace(keyID) == "" {
			return fmt.Errorf("authority trust key id is required")
		}
		if len(publicKey) != ed25519.PublicKeySize {
			return fmt.Errorf("authority trust key %q has invalid length", keyID)
		}
	}
	return nil
}

// SignatureVerifier returns an Ed25519 verifier backed by the active keys in
// this trust snapshot.
func (store AuthorityTrustStore) SignatureVerifier() Ed25519AuthoritySignatureVerifier {
	keys := make(map[string]ed25519.PublicKey, len(store.Keys))
	for keyID, publicKey := range store.Keys {
		keys[keyID] = append(ed25519.PublicKey(nil), publicKey...)
	}
	return Ed25519AuthoritySignatureVerifier{Keys: keys}
}

// Rotate verifies and accepts a successor trust snapshot signed by the
// caller-supplied root verifier. Trust identity must remain stable and
// versions must increase strictly, so a valid old snapshot cannot be replayed
// as a replacement.
func (store AuthorityTrustStore) Rotate(data []byte, reference AuthorityTrustReference, verifier AuthoritySignatureVerifier) (AuthorityTrustStore, error) {
	if err := store.Validate(); err != nil {
		return AuthorityTrustStore{}, fmt.Errorf("validate current Hammond authority trust: %w", err)
	}
	replacement, err := DecodeAuthorityTrustStoreWithSignatureVerifier(data, reference, verifier)
	if err != nil {
		return AuthorityTrustStore{}, err
	}
	if replacement.Reference.ID != store.Reference.ID {
		return AuthorityTrustStore{}, fmt.Errorf("authority trust rotation must retain trust id")
	}
	if replacement.Reference.Version <= store.Reference.Version {
		return AuthorityTrustStore{}, fmt.Errorf("authority trust rotation version must increase")
	}
	return replacement, nil
}

// RotateWithRootStore verifies and accepts a successor trust snapshot using
// the active keys of a validated Hammond authority root. This keeps the
// intended root-to-trust chain explicit at the call site.
func (store AuthorityTrustStore) RotateWithRootStore(data []byte, reference AuthorityTrustReference, roots AuthorityRootStore) (AuthorityTrustStore, error) {
	if err := roots.Validate(); err != nil {
		return AuthorityTrustStore{}, fmt.Errorf("validate Hammond authority root: %w", err)
	}
	return store.Rotate(data, reference, roots.SignatureVerifier())
}

// DecodeAuthorityTrustStore strictly decodes a versioned trust snapshot and
// binds its exact bytes to the supplied reference.
func DecodeAuthorityTrustStore(data []byte, reference AuthorityTrustReference) (AuthorityTrustStore, error) {
	return decodeAuthorityTrustStore(data, reference, nil)
}

// DecodeAuthorityTrustStoreWithSignatureVerifier strictly decodes a trust
// snapshot and verifies its root signature using the caller's trusted key set.
func DecodeAuthorityTrustStoreWithSignatureVerifier(data []byte, reference AuthorityTrustReference, verifier AuthoritySignatureVerifier) (AuthorityTrustStore, error) {
	if verifier == nil {
		return AuthorityTrustStore{}, fmt.Errorf("authority trust signature verifier is required")
	}
	return decodeAuthorityTrustStore(data, reference, verifier)
}

func decodeAuthorityTrustStore(data []byte, reference AuthorityTrustReference, verifier AuthoritySignatureVerifier) (AuthorityTrustStore, error) {
	var document authorityTrustDocument
	if err := decodeStrict(data, &document); err != nil {
		return AuthorityTrustStore{}, fmt.Errorf("decode Hammond authority trust: %w", err)
	}
	if document.Schema != AuthorityTrustSchema {
		return AuthorityTrustStore{}, fmt.Errorf("authority trust schema must be %s", AuthorityTrustSchema)
	}
	if document.ID != reference.ID || document.Version != reference.Version {
		return AuthorityTrustStore{}, fmt.Errorf("authority trust identity does not match its reference")
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != reference.Artifact.SHA256 {
		return AuthorityTrustStore{}, fmt.Errorf("authority trust bytes do not match reference artifact.sha256")
	}
	if len(document.Keys) == 0 {
		return AuthorityTrustStore{}, fmt.Errorf("authority trust keys are required")
	}
	if verifier != nil {
		if document.Signature == nil {
			return AuthorityTrustStore{}, fmt.Errorf("authority trust signature is required")
		}
		if err := validateAuthoritySignature(document.Signature); err != nil {
			return AuthorityTrustStore{}, err
		}
	}
	activeKeys := make(map[string]ed25519.PublicKey, len(document.Keys))
	seenIDs := make(map[string]struct{}, len(document.Keys))
	for index, key := range document.Keys {
		path := fmt.Sprintf("keys[%d]", index)
		keyID := strings.TrimSpace(key.ID)
		if keyID == "" {
			return AuthorityTrustStore{}, fmt.Errorf("%s.id is required", path)
		}
		if _, exists := seenIDs[keyID]; exists {
			return AuthorityTrustStore{}, fmt.Errorf("%s.id is duplicated", path)
		}
		seenIDs[keyID] = struct{}{}
		if key.Algorithm != AuthoritySignatureAlgorithmEd25519 {
			return AuthorityTrustStore{}, fmt.Errorf("%s.algorithm must be %s", path, AuthoritySignatureAlgorithmEd25519)
		}
		publicKey, err := decodeAuthorityPublicKey(key.PublicKey)
		if err != nil {
			return AuthorityTrustStore{}, fmt.Errorf("%s.public_key: %w", path, err)
		}
		switch key.Status {
		case AuthorityTrustKeyActive:
			activeKeys[keyID] = publicKey
		case AuthorityTrustKeyRevoked:
		default:
			return AuthorityTrustStore{}, fmt.Errorf("%s.status must be %s or %s", path, AuthorityTrustKeyActive, AuthorityTrustKeyRevoked)
		}
	}
	if verifier != nil {
		keys := append([]authorityTrustKeyDocument(nil), document.Keys...)
		sort.Slice(keys, func(left, right int) bool { return keys[left].ID < keys[right].ID })
		payload, err := json.Marshal(authorityTrustSigningDocument{
			Schema:  document.Schema,
			ID:      document.ID,
			Version: document.Version,
			Keys:    keys,
		})
		if err != nil {
			return AuthorityTrustStore{}, fmt.Errorf("canonicalize authority trust for signature: %w", err)
		}
		signature, err := decodeAuthoritySignature(document.Signature.Signature)
		if err != nil {
			return AuthorityTrustStore{}, err
		}
		if err := verifier.VerifySignature(document.Signature.KeyID, payload, signature); err != nil {
			return AuthorityTrustStore{}, fmt.Errorf("verify authority trust signature: %w", err)
		}
	}
	store := AuthorityTrustStore{Reference: reference, Keys: activeKeys, Signature: document.Signature}
	if err := store.Validate(); err != nil {
		return AuthorityTrustStore{}, err
	}
	return store, nil
}

// LoadAuthorityTrustStore loads and verifies a local trust snapshot.
func LoadAuthorityTrustStore(reference AuthorityTrustReference) (AuthorityTrustStore, error) {
	data, err := readLocalArtifact(reference.Artifact.URI)
	if err != nil {
		return AuthorityTrustStore{}, fmt.Errorf("read Hammond authority trust %s: %w", reference.Artifact.URI, err)
	}
	return DecodeAuthorityTrustStore(data, reference)
}

// LoadAuthorityTrustStoreWithSignatureVerifier loads a trust snapshot and
// requires its root signature to validate against the supplied trust set.
func LoadAuthorityTrustStoreWithSignatureVerifier(reference AuthorityTrustReference, verifier AuthoritySignatureVerifier) (AuthorityTrustStore, error) {
	data, err := readLocalArtifact(reference.Artifact.URI)
	if err != nil {
		return AuthorityTrustStore{}, fmt.Errorf("read Hammond authority trust %s: %w", reference.Artifact.URI, err)
	}
	return DecodeAuthorityTrustStoreWithSignatureVerifier(data, reference, verifier)
}

func decodeAuthorityPublicKey(value string) (ed25519.PublicKey, error) {
	publicKey, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("is not valid base64: %w", err)
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("has invalid length")
	}
	return ed25519.PublicKey(publicKey), nil
}
