package governance

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type authorityRootKeyDocument struct {
	ID        string `json:"id"`
	Algorithm string `json:"algorithm"`
	PublicKey string `json:"public_key"`
	Status    string `json:"status"`
}

type authorityRootDocument struct {
	Schema    string                     `json:"schema"`
	ID        string                     `json:"id"`
	Version   int                        `json:"version"`
	Keys      []authorityRootKeyDocument `json:"keys"`
	Signature *AuthoritySignature        `json:"signature,omitempty"`
}

type authorityRootSigningDocument struct {
	Schema  string                     `json:"schema"`
	ID      string                     `json:"id"`
	Version int                        `json:"version"`
	Keys    []authorityRootKeyDocument `json:"keys"`
}

// AuthorityRootBootstrap is the caller-owned pin set for the first root
// snapshot. It must come from deployment configuration or another approved
// out-of-band channel, not from the root snapshot being verified.
type AuthorityRootBootstrap struct {
	ID      string
	Version int
	Keys    map[string]ed25519.PublicKey
}

func (bootstrap AuthorityRootBootstrap) Validate() error {
	if strings.TrimSpace(bootstrap.ID) == "" {
		return fmt.Errorf("authority root bootstrap id is required")
	}
	if bootstrap.Version < 1 {
		return fmt.Errorf("authority root bootstrap version must be at least 1")
	}
	if len(bootstrap.Keys) == 0 {
		return fmt.Errorf("authority root bootstrap keys are required")
	}
	for keyID, publicKey := range bootstrap.Keys {
		if strings.TrimSpace(keyID) == "" {
			return fmt.Errorf("authority root bootstrap key id is required")
		}
		if len(publicKey) != ed25519.PublicKeySize {
			return fmt.Errorf("authority root bootstrap key %q has invalid length", keyID)
		}
	}
	return nil
}

// SignatureVerifier returns a verifier backed by the pinned bootstrap keys.
func (bootstrap AuthorityRootBootstrap) SignatureVerifier() (Ed25519AuthoritySignatureVerifier, error) {
	if err := bootstrap.Validate(); err != nil {
		return Ed25519AuthoritySignatureVerifier{}, err
	}
	keys := make(map[string]ed25519.PublicKey, len(bootstrap.Keys))
	for keyID, publicKey := range bootstrap.Keys {
		keys[keyID] = append(ed25519.PublicKey(nil), publicKey...)
	}
	return Ed25519AuthoritySignatureVerifier{Keys: keys}, nil
}

// AuthorityRootStore contains only active root public keys. A caller may load
// a signed replacement snapshot with the previous store's verifier, then use
// the replacement's active keys for the next rotation.
type AuthorityRootStore struct {
	Reference AuthorityRootReference
	Keys      map[string]ed25519.PublicKey
	Signature *AuthoritySignature
}

func (store AuthorityRootStore) Validate() error {
	if problems := validateAuthorityRootReference(store.Reference, "root"); len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	if err := validateAuthoritySignature(store.Signature); err != nil {
		return err
	}
	if store.Keys == nil {
		return fmt.Errorf("authority root keys are required")
	}
	for keyID, publicKey := range store.Keys {
		if strings.TrimSpace(keyID) == "" {
			return fmt.Errorf("authority root key id is required")
		}
		if len(publicKey) != ed25519.PublicKeySize {
			return fmt.Errorf("authority root key %q has invalid length", keyID)
		}
	}
	return nil
}

// SignatureVerifier returns a verifier backed by the active root keys in this
// snapshot. Revoked keys are intentionally absent, so rotation fails closed.
func (store AuthorityRootStore) SignatureVerifier() Ed25519AuthoritySignatureVerifier {
	keys := make(map[string]ed25519.PublicKey, len(store.Keys))
	for keyID, publicKey := range store.Keys {
		keys[keyID] = append(ed25519.PublicKey(nil), publicKey...)
	}
	return Ed25519AuthoritySignatureVerifier{Keys: keys}
}

// Rotate verifies and accepts a successor root snapshot signed by this
// store's active keys. Root identity must remain stable and versions must
// increase strictly, so a valid old snapshot cannot be replayed as a
// replacement.
func (store AuthorityRootStore) Rotate(data []byte, reference AuthorityRootReference) (AuthorityRootStore, error) {
	if err := store.Validate(); err != nil {
		return AuthorityRootStore{}, fmt.Errorf("validate current Hammond authority root: %w", err)
	}
	replacement, err := DecodeAuthorityRootStoreWithSignatureVerifier(data, reference, store.SignatureVerifier())
	if err != nil {
		return AuthorityRootStore{}, err
	}
	if replacement.Reference.ID != store.Reference.ID {
		return AuthorityRootStore{}, fmt.Errorf("authority root rotation must retain root id")
	}
	if replacement.Reference.Version <= store.Reference.Version {
		return AuthorityRootStore{}, fmt.Errorf("authority root rotation version must increase")
	}
	return replacement, nil
}

// DecodeAuthorityRootStore strictly decodes a versioned root-key snapshot and
// binds its exact bytes to the supplied reference.
func DecodeAuthorityRootStore(data []byte, reference AuthorityRootReference) (AuthorityRootStore, error) {
	return decodeAuthorityRootStore(data, reference, nil)
}

// DecodeAuthorityRootStoreWithSignatureVerifier verifies a root-key snapshot
// against a separately configured bootstrap or predecessor root set.
func DecodeAuthorityRootStoreWithSignatureVerifier(data []byte, reference AuthorityRootReference, verifier AuthoritySignatureVerifier) (AuthorityRootStore, error) {
	if verifier == nil {
		return AuthorityRootStore{}, fmt.Errorf("authority root signature verifier is required")
	}
	return decodeAuthorityRootStore(data, reference, verifier)
}

// DecodeAuthorityRootStoreWithBootstrap verifies a root snapshot using the
// caller-owned pins for the initial bootstrap or recovery operation.
func DecodeAuthorityRootStoreWithBootstrap(data []byte, reference AuthorityRootReference, bootstrap AuthorityRootBootstrap) (AuthorityRootStore, error) {
	verifier, err := bootstrap.SignatureVerifier()
	if err != nil {
		return AuthorityRootStore{}, err
	}
	store, err := DecodeAuthorityRootStoreWithSignatureVerifier(data, reference, verifier)
	if err != nil {
		return AuthorityRootStore{}, err
	}
	if store.Reference.ID != bootstrap.ID {
		return AuthorityRootStore{}, fmt.Errorf("authority root bootstrap id does not match root snapshot")
	}
	if store.Reference.Version != bootstrap.Version {
		return AuthorityRootStore{}, fmt.Errorf("authority root bootstrap version does not match root snapshot")
	}
	return store, nil
}

func decodeAuthorityRootStore(data []byte, reference AuthorityRootReference, verifier AuthoritySignatureVerifier) (AuthorityRootStore, error) {
	var document authorityRootDocument
	if err := decodeStrict(data, &document); err != nil {
		return AuthorityRootStore{}, fmt.Errorf("decode Hammond authority root: %w", err)
	}
	if document.Schema != AuthorityRootSchema {
		return AuthorityRootStore{}, fmt.Errorf("authority root schema must be %s", AuthorityRootSchema)
	}
	if document.ID != reference.ID || document.Version != reference.Version {
		return AuthorityRootStore{}, fmt.Errorf("authority root identity does not match its reference")
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != reference.Artifact.SHA256 {
		return AuthorityRootStore{}, fmt.Errorf("authority root bytes do not match reference artifact.sha256")
	}
	if len(document.Keys) == 0 {
		return AuthorityRootStore{}, fmt.Errorf("authority root keys are required")
	}
	if verifier != nil {
		if document.Signature == nil {
			return AuthorityRootStore{}, fmt.Errorf("authority root signature is required")
		}
		if err := validateAuthoritySignature(document.Signature); err != nil {
			return AuthorityRootStore{}, err
		}
	}
	activeKeys := make(map[string]ed25519.PublicKey, len(document.Keys))
	seenIDs := make(map[string]struct{}, len(document.Keys))
	for index, key := range document.Keys {
		path := fmt.Sprintf("keys[%d]", index)
		keyID := strings.TrimSpace(key.ID)
		if keyID == "" {
			return AuthorityRootStore{}, fmt.Errorf("%s.id is required", path)
		}
		if _, exists := seenIDs[keyID]; exists {
			return AuthorityRootStore{}, fmt.Errorf("%s.id is duplicated", path)
		}
		seenIDs[keyID] = struct{}{}
		if key.Algorithm != AuthoritySignatureAlgorithmEd25519 {
			return AuthorityRootStore{}, fmt.Errorf("%s.algorithm must be %s", path, AuthoritySignatureAlgorithmEd25519)
		}
		publicKey, err := decodeAuthorityPublicKey(key.PublicKey)
		if err != nil {
			return AuthorityRootStore{}, fmt.Errorf("%s.public_key: %w", path, err)
		}
		switch key.Status {
		case AuthorityTrustKeyActive:
			activeKeys[keyID] = publicKey
		case AuthorityTrustKeyRevoked:
		default:
			return AuthorityRootStore{}, fmt.Errorf("%s.status must be %s or %s", path, AuthorityTrustKeyActive, AuthorityTrustKeyRevoked)
		}
	}
	if verifier != nil {
		keys := append([]authorityRootKeyDocument(nil), document.Keys...)
		sort.Slice(keys, func(left, right int) bool { return keys[left].ID < keys[right].ID })
		payload, err := json.Marshal(authorityRootSigningDocument{
			Schema:  document.Schema,
			ID:      document.ID,
			Version: document.Version,
			Keys:    keys,
		})
		if err != nil {
			return AuthorityRootStore{}, fmt.Errorf("canonicalize authority root for signature: %w", err)
		}
		signature, err := decodeAuthoritySignature(document.Signature.Signature)
		if err != nil {
			return AuthorityRootStore{}, err
		}
		if err := verifier.VerifySignature(document.Signature.KeyID, payload, signature); err != nil {
			return AuthorityRootStore{}, fmt.Errorf("verify authority root signature: %w", err)
		}
	}
	store := AuthorityRootStore{Reference: reference, Keys: activeKeys, Signature: document.Signature}
	if err := store.Validate(); err != nil {
		return AuthorityRootStore{}, err
	}
	return store, nil
}

// LoadAuthorityRootStore loads an unsigned root snapshot from its local
// artifact. The initial bootstrap key set is intentionally caller-owned.
func LoadAuthorityRootStore(reference AuthorityRootReference) (AuthorityRootStore, error) {
	data, err := readLocalArtifact(reference.Artifact.URI)
	if err != nil {
		return AuthorityRootStore{}, fmt.Errorf("read Hammond authority root %s: %w", reference.Artifact.URI, err)
	}
	return DecodeAuthorityRootStore(data, reference)
}

// LoadAuthorityRootStoreWithSignatureVerifier loads a root snapshot and
// verifies it against a bootstrap or predecessor root set.
func LoadAuthorityRootStoreWithSignatureVerifier(reference AuthorityRootReference, verifier AuthoritySignatureVerifier) (AuthorityRootStore, error) {
	data, err := readLocalArtifact(reference.Artifact.URI)
	if err != nil {
		return AuthorityRootStore{}, fmt.Errorf("read Hammond authority root %s: %w", reference.Artifact.URI, err)
	}
	return DecodeAuthorityRootStoreWithSignatureVerifier(data, reference, verifier)
}

// LoadAuthorityRootStoreWithBootstrap loads the first root snapshot using
// caller-owned bootstrap pins rather than trusting keys from that snapshot.
func LoadAuthorityRootStoreWithBootstrap(reference AuthorityRootReference, bootstrap AuthorityRootBootstrap) (AuthorityRootStore, error) {
	data, err := readLocalArtifact(reference.Artifact.URI)
	if err != nil {
		return AuthorityRootStore{}, fmt.Errorf("read Hammond authority root %s: %w", reference.Artifact.URI, err)
	}
	return DecodeAuthorityRootStoreWithBootstrap(data, reference, bootstrap)
}

// DecodeAuthorityTrustStoreWithRootStore verifies a trust snapshot using the
// active root keys from a validated root snapshot.
func DecodeAuthorityTrustStoreWithRootStore(data []byte, reference AuthorityTrustReference, roots AuthorityRootStore) (AuthorityTrustStore, error) {
	if err := roots.Validate(); err != nil {
		return AuthorityTrustStore{}, fmt.Errorf("validate Hammond authority root: %w", err)
	}
	return DecodeAuthorityTrustStoreWithSignatureVerifier(data, reference, roots.SignatureVerifier())
}

// LoadAuthorityTrustStoreWithRootStore loads a trust snapshot and verifies it
// using the active root keys from a validated root snapshot.
func LoadAuthorityTrustStoreWithRootStore(reference AuthorityTrustReference, roots AuthorityRootStore) (AuthorityTrustStore, error) {
	data, err := readLocalArtifact(reference.Artifact.URI)
	if err != nil {
		return AuthorityTrustStore{}, fmt.Errorf("read Hammond authority trust %s: %w", reference.Artifact.URI, err)
	}
	return DecodeAuthorityTrustStoreWithRootStore(data, reference, roots)
}
