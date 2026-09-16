package attestation

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/custody"
	"ingen/lockwood/internal/store"
)

const (
	HandlingEventSchema     = "lockwood.handling-event-attestation/v1"
	HandlingEventTargetKind = "handling-event"
	HandlingEventMediaType  = "application/vnd.ingen.lockwood.handling-event-attestation+json"
)

var (
	handlingEventIDPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	handlingCustodyIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

// HandlingEventTarget identifies the canonical bytes of one handling event.
// The event digest is separate from the detached envelope artifact digest.
type HandlingEventTarget struct {
	Kind      string `json:"kind"`
	CustodyID string `json:"custody_id"`
	EventID   string `json:"event_id"`
	Digest    string `json:"digest"`
}

// HandlingEventEnvelope is a detached signature over one canonical handling
// event. The key authenticates control of a trusted signing key; it does not
// identify a human or authorize the described action by itself.
type HandlingEventEnvelope struct {
	Schema    string              `json:"schema"`
	Target    HandlingEventTarget `json:"target"`
	Algorithm string              `json:"algorithm"`
	KeyID     string              `json:"key_id"`
	Signature string              `json:"signature"`
}

func (envelope HandlingEventEnvelope) Validate() error {
	if envelope.Schema != HandlingEventSchema {
		return fmt.Errorf("unexpected handling event attestation schema %q", envelope.Schema)
	}
	if envelope.Target.Kind != HandlingEventTargetKind {
		return fmt.Errorf("unexpected handling event attestation target kind %q", envelope.Target.Kind)
	}
	if !handlingCustodyIDPattern.MatchString(envelope.Target.CustodyID) {
		return fmt.Errorf("invalid handling event attestation custody id %q", envelope.Target.CustodyID)
	}
	if !handlingEventIDPattern.MatchString(envelope.Target.EventID) {
		return fmt.Errorf("invalid handling event attestation event id %q", envelope.Target.EventID)
	}
	if err := artifact.ValidateDigest(envelope.Target.Digest); err != nil {
		return fmt.Errorf("invalid handling event attestation target: %w", err)
	}
	if envelope.Algorithm != Algorithm {
		return fmt.Errorf("unsupported handling event attestation algorithm %q", envelope.Algorithm)
	}
	if !keyIDPattern.MatchString(envelope.KeyID) {
		return fmt.Errorf("invalid handling event attestation key id %q", envelope.KeyID)
	}
	signature, err := base64.StdEncoding.DecodeString(envelope.Signature)
	if err != nil {
		return fmt.Errorf("decode handling event attestation signature: %w", err)
	}
	if base64.StdEncoding.EncodeToString(signature) != envelope.Signature {
		return fmt.Errorf("handling event attestation signature is not canonical standard-base64")
	}
	if len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("handling event attestation signature has size %d, want %d", len(signature), ed25519.SignatureSize)
	}
	return nil
}

// SignHandlingEvent signs the canonical handling-event digest with a
// domain-separated payload that binds the custody ID and event ID.
func SignHandlingEvent(event custody.HandlingEvent, keyID string, privateKey ed25519.PrivateKey) (HandlingEventEnvelope, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return HandlingEventEnvelope{}, fmt.Errorf("handling event attestation private key has size %d, want %d", len(privateKey), ed25519.PrivateKeySize)
	}
	digest, err := custody.HandlingEventDigest(event)
	if err != nil {
		return HandlingEventEnvelope{}, err
	}
	message, err := handlingEventSigningMessage(event.CustodyID, event.EventID, digest, keyID)
	if err != nil {
		return HandlingEventEnvelope{}, err
	}
	return HandlingEventEnvelope{
		Schema: HandlingEventSchema,
		Target: HandlingEventTarget{
			Kind:      HandlingEventTargetKind,
			CustodyID: event.CustodyID,
			EventID:   event.EventID,
			Digest:    digest,
		},
		Algorithm: Algorithm,
		KeyID:     keyID,
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, message)),
	}, nil
}

// VerifyHandlingEvent checks that an envelope binds to the exact canonical
// event and verifies its signature with the supplied public key.
func VerifyHandlingEvent(event custody.HandlingEvent, envelope HandlingEventEnvelope, publicKey ed25519.PublicKey) error {
	if err := envelope.Validate(); err != nil {
		return err
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("handling event attestation public key has size %d, want %d", len(publicKey), ed25519.PublicKeySize)
	}
	digest, err := custody.HandlingEventDigest(event)
	if err != nil {
		return err
	}
	if envelope.Target.CustodyID != event.CustodyID || envelope.Target.EventID != event.EventID {
		return fmt.Errorf("handling event attestation target identity mismatch")
	}
	if envelope.Target.Digest != digest {
		return fmt.Errorf("handling event attestation target digest mismatch: got %s, want %s", envelope.Target.Digest, digest)
	}
	message, err := handlingEventSigningMessage(event.CustodyID, event.EventID, digest, envelope.KeyID)
	if err != nil {
		return err
	}
	signature, err := base64.StdEncoding.DecodeString(envelope.Signature)
	if err != nil {
		return fmt.Errorf("decode handling event attestation signature: %w", err)
	}
	if !ed25519.Verify(publicKey, message, signature) {
		return fmt.Errorf("handling event attestation signature verification failed")
	}
	return nil
}

// VerifyHandlingEventWithRegistry resolves the exact key through the shared
// explicit trust registry before verifying the event signature.
func VerifyHandlingEventWithRegistry(event custody.HandlingEvent, envelope HandlingEventEnvelope, registry TrustRegistry, evaluatedAt time.Time) error {
	publicKey, err := registry.Resolve(envelope.KeyID, evaluatedAt)
	if err != nil {
		return err
	}
	return VerifyHandlingEvent(event, envelope, publicKey)
}

// MarshalCanonicalHandlingEventEnvelope returns the compact canonical local
// JSON encoding with no trailing newline.
func MarshalCanonicalHandlingEventEnvelope(envelope HandlingEventEnvelope) ([]byte, error) {
	if err := envelope.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(envelope)
}

// UnmarshalCanonicalHandlingEventEnvelope strictly decodes one canonical
// handling-event attestation envelope.
func UnmarshalCanonicalHandlingEventEnvelope(data []byte) (HandlingEventEnvelope, error) {
	var envelope HandlingEventEnvelope
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return HandlingEventEnvelope{}, fmt.Errorf("decode handling event attestation envelope: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return HandlingEventEnvelope{}, fmt.Errorf("handling event attestation envelope contains multiple JSON values")
		}
		return HandlingEventEnvelope{}, fmt.Errorf("decode handling event attestation envelope: %w", err)
	}
	canonical, err := MarshalCanonicalHandlingEventEnvelope(envelope)
	if err != nil {
		return HandlingEventEnvelope{}, err
	}
	if !bytes.Equal(data, canonical) {
		return HandlingEventEnvelope{}, fmt.Errorf("handling event attestation envelope is not canonical JSON")
	}
	return envelope, nil
}

// CanonicalHandlingEventEnvelopeDigest identifies detached envelope bytes.
func CanonicalHandlingEventEnvelopeDigest(envelope HandlingEventEnvelope) (string, error) {
	encoded, err := MarshalCanonicalHandlingEventEnvelope(envelope)
	if err != nil {
		return "", err
	}
	return artifact.DigestBytes(encoded), nil
}

// HandlingEventPublication is a transient receipt for a detached signed
// handling event.
type HandlingEventPublication struct {
	CustodyID   string                `json:"custody_id"`
	EventID     string                `json:"event_id"`
	EventDigest string                `json:"event_digest"`
	Artifact    artifact.Reference    `json:"artifact"`
	Envelope    HandlingEventEnvelope `json:"envelope"`
}

// PublishHandlingEvent stores one signed handling-event envelope without
// mutating the event stream or custody record.
func PublishHandlingEvent(event custody.HandlingEvent, envelope HandlingEventEnvelope, artifacts store.Store) (HandlingEventPublication, error) {
	eventDigest, err := custody.HandlingEventDigest(event)
	if err != nil {
		return HandlingEventPublication{}, err
	}
	if envelope.Target.CustodyID != event.CustodyID || envelope.Target.EventID != event.EventID || envelope.Target.Digest != eventDigest {
		return HandlingEventPublication{}, fmt.Errorf("handling event attestation target does not match event")
	}
	encoded, err := MarshalCanonicalHandlingEventEnvelope(envelope)
	if err != nil {
		return HandlingEventPublication{}, err
	}
	if artifacts == nil {
		return HandlingEventPublication{}, fmt.Errorf("artifact store is required")
	}
	ref, err := artifacts.Put(bytes.NewReader(encoded), store.PutOptions{
		ExpectedDigest: artifact.DigestBytes(encoded),
		MediaType:      HandlingEventMediaType,
		LogicalName:    "handling-event-attestation.json",
	})
	if err != nil {
		return HandlingEventPublication{}, err
	}
	return HandlingEventPublication{
		CustodyID:   event.CustodyID,
		EventID:     event.EventID,
		EventDigest: eventDigest,
		Artifact:    ref,
		Envelope:    envelope,
	}, nil
}

// LoadHandlingEventEnvelope loads and validates a detached envelope by its
// content digest without verifying its signature.
func LoadHandlingEventEnvelope(artifacts store.Store, digest string) (HandlingEventEnvelope, error) {
	if artifacts == nil {
		return HandlingEventEnvelope{}, fmt.Errorf("artifact store is required")
	}
	if err := artifact.ValidateDigest(digest); err != nil {
		return HandlingEventEnvelope{}, err
	}
	data, err := artifacts.Get(digest)
	if err != nil {
		return HandlingEventEnvelope{}, fmt.Errorf("load handling event attestation artifact: %w", err)
	}
	if actual := artifact.DigestBytes(data); actual != digest {
		return HandlingEventEnvelope{}, fmt.Errorf("handling event attestation artifact digest mismatch: got %s, want %s", actual, digest)
	}
	return UnmarshalCanonicalHandlingEventEnvelope(data)
}

// VerifyPublishedHandlingEventReceipt loads and verifies a published
// handling-event envelope with a caller-supplied public key.
func VerifyPublishedHandlingEventReceipt(event custody.HandlingEvent, digest string, artifacts store.Store, publicKey ed25519.PublicKey) (HandlingEventVerificationReceipt, error) {
	envelope, err := LoadHandlingEventEnvelope(artifacts, digest)
	if err != nil {
		return HandlingEventVerificationReceipt{}, err
	}
	if err := VerifyHandlingEvent(event, envelope, publicKey); err != nil {
		return HandlingEventVerificationReceipt{}, fmt.Errorf("verify published handling event attestation: %w", err)
	}
	eventDigest, err := custody.HandlingEventDigest(event)
	if err != nil {
		return HandlingEventVerificationReceipt{}, err
	}
	return HandlingEventVerificationReceipt{
		CustodyID:         event.CustodyID,
		EventID:           event.EventID,
		EventDigest:       eventDigest,
		AttestationDigest: digest,
		KeyID:             envelope.KeyID,
		Algorithm:         envelope.Algorithm,
		Verified:          true,
	}, nil
}

// VerifyPublishedHandlingEventWithRegistryReceipt loads and verifies a
// published handling-event envelope through a trust-registry snapshot.
func VerifyPublishedHandlingEventWithRegistryReceipt(event custody.HandlingEvent, digest string, artifacts store.Store, registry TrustRegistry, evaluatedAt time.Time) (TrustedHandlingEventVerificationReceipt, error) {
	envelope, err := LoadHandlingEventEnvelope(artifacts, digest)
	if err != nil {
		return TrustedHandlingEventVerificationReceipt{}, err
	}
	if err := VerifyHandlingEventWithRegistry(event, envelope, registry, evaluatedAt); err != nil {
		return TrustedHandlingEventVerificationReceipt{}, fmt.Errorf("verify published handling event attestation: %w", err)
	}
	registryDigest, err := TrustRegistryDigest(registry)
	if err != nil {
		return TrustedHandlingEventVerificationReceipt{}, err
	}
	eventDigest, err := custody.HandlingEventDigest(event)
	if err != nil {
		return TrustedHandlingEventVerificationReceipt{}, err
	}
	evaluatedAt = evaluatedAt.UTC()
	return TrustedHandlingEventVerificationReceipt{
		CustodyID:         event.CustodyID,
		EventID:           event.EventID,
		EventDigest:       eventDigest,
		AttestationDigest: digest,
		KeyID:             envelope.KeyID,
		Algorithm:         envelope.Algorithm,
		Verified:          true,
		RegistryDigest:    registryDigest,
		EvaluatedAt:       evaluatedAt.Format(time.RFC3339),
		Trusted:           true,
	}, nil
}

// HandlingEventVerificationReceipt is transient evidence of a signed event
// verification with a caller-supplied public key.
type HandlingEventVerificationReceipt struct {
	CustodyID         string `json:"custody_id"`
	EventID           string `json:"event_id"`
	EventDigest       string `json:"event_digest"`
	AttestationDigest string `json:"attestation_digest"`
	KeyID             string `json:"key_id"`
	Algorithm         string `json:"algorithm"`
	Verified          bool   `json:"verified"`
}

// TrustedHandlingEventVerificationReceipt is transient evidence of a signed
// event verification and trust-registry resolution.
type TrustedHandlingEventVerificationReceipt struct {
	CustodyID         string `json:"custody_id"`
	EventID           string `json:"event_id"`
	EventDigest       string `json:"event_digest"`
	AttestationDigest string `json:"attestation_digest"`
	KeyID             string `json:"key_id"`
	Algorithm         string `json:"algorithm"`
	Verified          bool   `json:"verified"`
	RegistryDigest    string `json:"registry_digest"`
	EvaluatedAt       string `json:"evaluated_at"`
	Trusted           bool   `json:"trusted"`
}

func handlingEventSigningMessage(custodyID, eventID, digest, keyID string) ([]byte, error) {
	if !handlingCustodyIDPattern.MatchString(custodyID) {
		return nil, fmt.Errorf("invalid handling event attestation custody id %q", custodyID)
	}
	if !handlingEventIDPattern.MatchString(eventID) {
		return nil, fmt.Errorf("invalid handling event attestation event id %q", eventID)
	}
	if err := artifact.ValidateDigest(digest); err != nil {
		return nil, err
	}
	if !keyIDPattern.MatchString(keyID) {
		return nil, fmt.Errorf("invalid handling event attestation key id %q", keyID)
	}
	return []byte(strings.Join([]string{HandlingEventSchema, HandlingEventTargetKind, keyID, custodyID, eventID, digest, ""}, "\n")), nil
}
