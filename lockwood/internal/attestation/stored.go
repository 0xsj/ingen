package attestation

import (
	"fmt"
	"time"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/custody"
	"ingen/lockwood/internal/store"
)

// VerificationReceipt is transient operator/API evidence that a published
// envelope verified successfully. It is not a custody record or a producer
// verdict.
type VerificationReceipt struct {
	CustodyID         string `json:"custody_id"`
	AttestationDigest string `json:"attestation_digest"`
	TargetDigest      string `json:"target_digest"`
	KeyID             string `json:"key_id"`
	Algorithm         string `json:"algorithm"`
	Verified          bool   `json:"verified"`
}

// TrustedVerificationReceipt is transient evidence that a published
// attestation both verified cryptographically and resolved through the
// supplied trust-registry snapshot. It is not a custody record or an access
// decision.
type TrustedVerificationReceipt struct {
	VerificationReceipt
	RegistryDigest string `json:"registry_digest"`
	EvaluatedAt    string `json:"evaluated_at"`
	Trusted        bool   `json:"trusted"`
}

// Inspection is read-only metadata for a known detached envelope artifact.
// Loading it does not verify the signature or establish signer trust.
type Inspection struct {
	AttestationDigest string   `json:"attestation_digest"`
	Envelope          Envelope `json:"envelope"`
}

// Load reads one published attestation artifact, verifies its content digest,
// and decodes its canonical envelope. It does not verify the signature.
func Load(artifacts store.Store, digest string) (Envelope, error) {
	if artifacts == nil {
		return Envelope{}, fmt.Errorf("artifact store is required")
	}
	if err := artifact.ValidateDigest(digest); err != nil {
		return Envelope{}, err
	}
	data, err := artifacts.Get(digest)
	if err != nil {
		return Envelope{}, fmt.Errorf("load attestation artifact: %w", err)
	}
	actualDigest := artifact.DigestBytes(data)
	if actualDigest != digest {
		return Envelope{}, fmt.Errorf("attestation artifact digest mismatch: got %s, want %s", actualDigest, digest)
	}
	envelope, err := UnmarshalCanonical(data)
	if err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

// Inspect loads one published envelope by its content digest without
// performing cryptographic verification.
func Inspect(artifacts store.Store, digest string) (Inspection, error) {
	envelope, err := Load(artifacts, digest)
	if err != nil {
		return Inspection{}, err
	}
	return Inspection{AttestationDigest: digest, Envelope: envelope}, nil
}

// VerifyPublished loads a canonical attestation artifact and verifies it
// against a custody record using the caller-supplied key set. Key trust and
// authorization remain outside Lockwood.
func VerifyPublished(record custody.Record, digest string, artifacts store.Store, keys PublicKeySet) error {
	_, err := VerifyPublishedReceipt(record, digest, artifacts, keys)
	return err
}

// VerifyPublishedReceipt loads and verifies a published envelope, returning
// structured evidence for the successful operation. Trust and authorization
// remain determined by the supplied key set and its caller.
func VerifyPublishedReceipt(record custody.Record, digest string, artifacts store.Store, keys PublicKeySet) (VerificationReceipt, error) {
	envelope, err := Load(artifacts, digest)
	if err != nil {
		return VerificationReceipt{}, err
	}
	if err := VerifyWithKeySet(record, envelope, keys); err != nil {
		return VerificationReceipt{}, fmt.Errorf("verify published attestation: %w", err)
	}
	return VerificationReceipt{
		CustodyID:         record.CustodyID,
		AttestationDigest: digest,
		TargetDigest:      envelope.Target.Digest,
		KeyID:             envelope.KeyID,
		Algorithm:         envelope.Algorithm,
		Verified:          true,
	}, nil
}

// VerifyPublishedWithRegistry verifies a published envelope against a target
// record using an explicit trust-registry snapshot and evaluation time.
func VerifyPublishedWithRegistry(record custody.Record, digest string, artifacts store.Store, registry TrustRegistry, evaluatedAt time.Time) error {
	_, err := VerifyPublishedWithRegistryReceipt(record, digest, artifacts, registry, evaluatedAt)
	return err
}

// VerifyPublishedWithRegistryReceipt returns transient evidence for a
// successful cryptographic verification performed with a trusted registry.
func VerifyPublishedWithRegistryReceipt(record custody.Record, digest string, artifacts store.Store, registry TrustRegistry, evaluatedAt time.Time) (TrustedVerificationReceipt, error) {
	envelope, err := Load(artifacts, digest)
	if err != nil {
		return TrustedVerificationReceipt{}, err
	}
	if err := VerifyWithRegistry(record, envelope, registry, evaluatedAt); err != nil {
		return TrustedVerificationReceipt{}, fmt.Errorf("verify published attestation with trust registry: %w", err)
	}
	registryDigest, err := TrustRegistryDigest(registry)
	if err != nil {
		return TrustedVerificationReceipt{}, err
	}
	evaluatedAt = evaluatedAt.UTC()
	return TrustedVerificationReceipt{
		VerificationReceipt: VerificationReceipt{
			CustodyID:         record.CustodyID,
			AttestationDigest: digest,
			TargetDigest:      envelope.Target.Digest,
			KeyID:             envelope.KeyID,
			Algorithm:         envelope.Algorithm,
			Verified:          true,
		},
		RegistryDigest: registryDigest,
		EvaluatedAt:    evaluatedAt.Format(time.RFC3339),
		Trusted:        true,
	}, nil
}
