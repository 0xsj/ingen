package attestation

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/custody"
	"ingen/lockwood/internal/store"
)

const (
	RedactionProvenanceSchema     = "lockwood.redaction-provenance-attestation/v1"
	RedactionProvenanceTargetKind = "redaction-promotion"
	RedactionProvenanceMediaType  = "application/vnd.ingen.lockwood.redaction-provenance-attestation+json"
)

// RedactionProvenanceTarget binds a source custody record and handling event
// to the custody record that anchors the resulting artifact.
type RedactionProvenanceTarget struct {
	Kind                 string `json:"kind"`
	SourceCustodyID      string `json:"source_custody_id"`
	SourceRecordDigest   string `json:"source_record_digest"`
	EventID              string `json:"event_id"`
	EventDigest          string `json:"event_digest"`
	OriginalDigest       string `json:"original_digest"`
	ResultingDigest      string `json:"resulting_digest"`
	PromotedCustodyID    string `json:"promoted_custody_id"`
	PromotedRecordDigest string `json:"promoted_record_digest"`
}

// RedactionProvenanceEnvelope is a detached signature over the exact source,
// event, and promoted-record relationship. It does not sign or replace any
// custody record or payload artifact.
type RedactionProvenanceEnvelope struct {
	Schema    string                    `json:"schema"`
	Target    RedactionProvenanceTarget `json:"target"`
	Algorithm string                    `json:"algorithm"`
	KeyID     string                    `json:"key_id"`
	Signature string                    `json:"signature"`
}

func (envelope RedactionProvenanceEnvelope) Validate() error {
	if envelope.Schema != RedactionProvenanceSchema {
		return fmt.Errorf("unexpected redaction provenance schema %q", envelope.Schema)
	}
	if envelope.Target.Kind != RedactionProvenanceTargetKind {
		return fmt.Errorf("unexpected redaction provenance target kind %q", envelope.Target.Kind)
	}
	if !handlingCustodyIDPattern.MatchString(envelope.Target.SourceCustodyID) {
		return fmt.Errorf("invalid redaction provenance source custody id %q", envelope.Target.SourceCustodyID)
	}
	if !handlingEventIDPattern.MatchString(envelope.Target.EventID) {
		return fmt.Errorf("invalid redaction provenance event id %q", envelope.Target.EventID)
	}
	if !handlingCustodyIDPattern.MatchString(envelope.Target.PromotedCustodyID) {
		return fmt.Errorf("invalid redaction provenance promoted custody id %q", envelope.Target.PromotedCustodyID)
	}
	if envelope.Target.SourceCustodyID == envelope.Target.PromotedCustodyID {
		return fmt.Errorf("redaction provenance source and promoted custody IDs must differ")
	}
	for name, digest := range map[string]string{
		"source record":   envelope.Target.SourceRecordDigest,
		"event":           envelope.Target.EventDigest,
		"original":        envelope.Target.OriginalDigest,
		"resulting":       envelope.Target.ResultingDigest,
		"promoted record": envelope.Target.PromotedRecordDigest,
	} {
		if err := artifact.ValidateDigest(digest); err != nil {
			return fmt.Errorf("invalid redaction provenance %s digest: %w", name, err)
		}
	}
	if envelope.Target.OriginalDigest == envelope.Target.ResultingDigest {
		return fmt.Errorf("redaction provenance original and resulting digests must differ")
	}
	if envelope.Algorithm != Algorithm {
		return fmt.Errorf("unsupported redaction provenance algorithm %q", envelope.Algorithm)
	}
	if !keyIDPattern.MatchString(envelope.KeyID) {
		return fmt.Errorf("invalid redaction provenance key id %q", envelope.KeyID)
	}
	signature, err := base64.StdEncoding.DecodeString(envelope.Signature)
	if err != nil {
		return fmt.Errorf("decode redaction provenance signature: %w", err)
	}
	if base64.StdEncoding.EncodeToString(signature) != envelope.Signature {
		return fmt.Errorf("redaction provenance signature is not canonical standard-base64")
	}
	if len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("redaction provenance signature has size %d, want %d", len(signature), ed25519.SignatureSize)
	}
	return nil
}

func SignRedactionProvenance(source custody.Record, event custody.HandlingEvent, promoted custody.Record, keyID string, privateKey ed25519.PrivateKey) (RedactionProvenanceEnvelope, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return RedactionProvenanceEnvelope{}, fmt.Errorf("redaction provenance private key has size %d, want %d", len(privateKey), ed25519.PrivateKeySize)
	}
	target, err := redactionProvenanceTarget(source, event, promoted)
	if err != nil {
		return RedactionProvenanceEnvelope{}, err
	}
	message, err := redactionProvenanceSigningMessage(target, keyID)
	if err != nil {
		return RedactionProvenanceEnvelope{}, err
	}
	return RedactionProvenanceEnvelope{
		Schema:    RedactionProvenanceSchema,
		Target:    target,
		Algorithm: Algorithm,
		KeyID:     keyID,
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, message)),
	}, nil
}

func VerifyRedactionProvenance(source custody.Record, event custody.HandlingEvent, promoted custody.Record, envelope RedactionProvenanceEnvelope, publicKey ed25519.PublicKey) error {
	if err := envelope.Validate(); err != nil {
		return err
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("redaction provenance public key has size %d, want %d", len(publicKey), ed25519.PublicKeySize)
	}
	target, err := redactionProvenanceTarget(source, event, promoted)
	if err != nil {
		return err
	}
	if envelope.Target != target {
		return fmt.Errorf("redaction provenance target mismatch")
	}
	message, err := redactionProvenanceSigningMessage(target, envelope.KeyID)
	if err != nil {
		return err
	}
	signature, err := base64.StdEncoding.DecodeString(envelope.Signature)
	if err != nil {
		return fmt.Errorf("decode redaction provenance signature: %w", err)
	}
	if !ed25519.Verify(publicKey, message, signature) {
		return fmt.Errorf("redaction provenance signature verification failed")
	}
	return nil
}

func VerifyRedactionProvenanceWithRegistry(source custody.Record, event custody.HandlingEvent, promoted custody.Record, envelope RedactionProvenanceEnvelope, registry TrustRegistry, evaluatedAt time.Time) error {
	publicKey, err := registry.Resolve(envelope.KeyID, evaluatedAt)
	if err != nil {
		return err
	}
	return VerifyRedactionProvenance(source, event, promoted, envelope, publicKey)
}

func MarshalCanonicalRedactionProvenance(envelope RedactionProvenanceEnvelope) ([]byte, error) {
	if err := envelope.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(envelope)
}

func UnmarshalCanonicalRedactionProvenance(data []byte) (RedactionProvenanceEnvelope, error) {
	var envelope RedactionProvenanceEnvelope
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return RedactionProvenanceEnvelope{}, fmt.Errorf("decode redaction provenance envelope: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return RedactionProvenanceEnvelope{}, fmt.Errorf("redaction provenance envelope contains multiple JSON values")
		}
		return RedactionProvenanceEnvelope{}, fmt.Errorf("decode redaction provenance envelope: %w", err)
	}
	canonical, err := MarshalCanonicalRedactionProvenance(envelope)
	if err != nil {
		return RedactionProvenanceEnvelope{}, err
	}
	if !bytes.Equal(data, canonical) {
		return RedactionProvenanceEnvelope{}, fmt.Errorf("redaction provenance envelope is not canonical JSON")
	}
	return envelope, nil
}

type RedactionProvenancePublication struct {
	SourceCustodyID   string                      `json:"source_custody_id"`
	EventID           string                      `json:"event_id"`
	PromotedCustodyID string                      `json:"promoted_custody_id"`
	Artifact          artifact.Reference          `json:"artifact"`
	Envelope          RedactionProvenanceEnvelope `json:"envelope"`
}

func PublishRedactionProvenance(source custody.Record, event custody.HandlingEvent, promoted custody.Record, envelope RedactionProvenanceEnvelope, artifacts store.Store) (RedactionProvenancePublication, error) {
	target, err := redactionProvenanceTarget(source, event, promoted)
	if err != nil {
		return RedactionProvenancePublication{}, err
	}
	if envelope.Target != target {
		return RedactionProvenancePublication{}, fmt.Errorf("redaction provenance target does not match inputs")
	}
	encoded, err := MarshalCanonicalRedactionProvenance(envelope)
	if err != nil {
		return RedactionProvenancePublication{}, err
	}
	if artifacts == nil {
		return RedactionProvenancePublication{}, fmt.Errorf("artifact store is required")
	}
	ref, err := artifacts.Put(bytes.NewReader(encoded), store.PutOptions{
		ExpectedDigest: artifact.DigestBytes(encoded),
		MediaType:      RedactionProvenanceMediaType,
		LogicalName:    "redaction-provenance-attestation.json",
	})
	if err != nil {
		return RedactionProvenancePublication{}, fmt.Errorf("publish redaction provenance envelope: %w", err)
	}
	return RedactionProvenancePublication{
		SourceCustodyID:   source.CustodyID,
		EventID:           event.EventID,
		PromotedCustodyID: promoted.CustodyID,
		Artifact:          ref,
		Envelope:          envelope,
	}, nil
}

func LoadRedactionProvenance(artifacts store.Store, digest string) (RedactionProvenanceEnvelope, error) {
	if artifacts == nil {
		return RedactionProvenanceEnvelope{}, fmt.Errorf("artifact store is required")
	}
	if err := artifact.ValidateDigest(digest); err != nil {
		return RedactionProvenanceEnvelope{}, err
	}
	data, err := artifacts.Get(digest)
	if err != nil {
		return RedactionProvenanceEnvelope{}, fmt.Errorf("load redaction provenance artifact: %w", err)
	}
	if actual := artifact.DigestBytes(data); actual != digest {
		return RedactionProvenanceEnvelope{}, fmt.Errorf("redaction provenance artifact digest mismatch: got %s, want %s", actual, digest)
	}
	return UnmarshalCanonicalRedactionProvenance(data)
}

type RedactionProvenanceVerificationReceipt struct {
	SourceCustodyID      string `json:"source_custody_id"`
	SourceRecordDigest   string `json:"source_record_digest"`
	EventID              string `json:"event_id"`
	EventDigest          string `json:"event_digest"`
	OriginalDigest       string `json:"original_digest"`
	ResultingDigest      string `json:"resulting_digest"`
	PromotedCustodyID    string `json:"promoted_custody_id"`
	PromotedRecordDigest string `json:"promoted_record_digest"`
	AttestationDigest    string `json:"attestation_digest"`
	KeyID                string `json:"key_id"`
	Algorithm            string `json:"algorithm"`
	Verified             bool   `json:"verified"`
}

type TrustedRedactionProvenanceVerificationReceipt struct {
	RedactionProvenanceVerificationReceipt
	RegistryDigest string `json:"registry_digest"`
	EvaluatedAt    string `json:"evaluated_at"`
	Trusted        bool   `json:"trusted"`
}

func VerifyPublishedRedactionProvenanceReceipt(source custody.Record, event custody.HandlingEvent, promoted custody.Record, digest string, records custody.RecordStore, artifacts store.Store, publicKey ed25519.PublicKey) (RedactionProvenanceVerificationReceipt, error) {
	envelope, err := LoadRedactionProvenance(artifacts, digest)
	if err != nil {
		return RedactionProvenanceVerificationReceipt{}, err
	}
	if err := verifyRedactionProvenanceRecords(source, event, promoted, records, artifacts); err != nil {
		return RedactionProvenanceVerificationReceipt{}, err
	}
	if err := VerifyRedactionProvenance(source, event, promoted, envelope, publicKey); err != nil {
		return RedactionProvenanceVerificationReceipt{}, fmt.Errorf("verify published redaction provenance: %w", err)
	}
	return redactionProvenanceReceipt(envelope, digest), nil
}

func VerifyPublishedRedactionProvenanceWithRegistryReceipt(source custody.Record, event custody.HandlingEvent, promoted custody.Record, digest string, records custody.RecordStore, artifacts store.Store, registry TrustRegistry, evaluatedAt time.Time) (TrustedRedactionProvenanceVerificationReceipt, error) {
	envelope, err := LoadRedactionProvenance(artifacts, digest)
	if err != nil {
		return TrustedRedactionProvenanceVerificationReceipt{}, err
	}
	if err := verifyRedactionProvenanceRecords(source, event, promoted, records, artifacts); err != nil {
		return TrustedRedactionProvenanceVerificationReceipt{}, err
	}
	if err := VerifyRedactionProvenanceWithRegistry(source, event, promoted, envelope, registry, evaluatedAt); err != nil {
		return TrustedRedactionProvenanceVerificationReceipt{}, fmt.Errorf("verify published redaction provenance with trust registry: %w", err)
	}
	registryDigest, err := TrustRegistryDigest(registry)
	if err != nil {
		return TrustedRedactionProvenanceVerificationReceipt{}, err
	}
	evaluatedAt = evaluatedAt.UTC()
	return TrustedRedactionProvenanceVerificationReceipt{
		RedactionProvenanceVerificationReceipt: redactionProvenanceReceipt(envelope, digest),
		RegistryDigest:                         registryDigest,
		EvaluatedAt:                            evaluatedAt.Format(time.RFC3339),
		Trusted:                                true,
	}, nil
}

func redactionProvenanceTarget(source custody.Record, event custody.HandlingEvent, promoted custody.Record) (RedactionProvenanceTarget, error) {
	if err := source.Validate(); err != nil {
		return RedactionProvenanceTarget{}, fmt.Errorf("validate redaction provenance source: %w", err)
	}
	if source.Status != custody.Accepted {
		return RedactionProvenanceTarget{}, fmt.Errorf("redaction provenance source record must be accepted")
	}
	if err := event.Validate(); err != nil {
		return RedactionProvenanceTarget{}, fmt.Errorf("validate redaction provenance event: %w", err)
	}
	if event.Type != custody.RedactionEvent {
		return RedactionProvenanceTarget{}, fmt.Errorf("redaction provenance event must be a redaction event")
	}
	if event.CustodyID != source.CustodyID {
		return RedactionProvenanceTarget{}, fmt.Errorf("redaction provenance event custody ID does not match source record")
	}
	if err := promoted.Validate(); err != nil {
		return RedactionProvenanceTarget{}, fmt.Errorf("validate redaction provenance promoted record: %w", err)
	}
	if promoted.Status != custody.Accepted {
		return RedactionProvenanceTarget{}, fmt.Errorf("redaction provenance promoted record must be accepted")
	}
	if promoted.CustodyID == source.CustodyID {
		return RedactionProvenanceTarget{}, fmt.Errorf("redaction provenance source and promoted custody IDs must differ")
	}
	if promoted.Artifact.Digest != event.ResultingDigest {
		return RedactionProvenanceTarget{}, fmt.Errorf("redaction provenance promoted artifact does not match event result")
	}
	if !hasDerivedFrom(promoted, event.OriginalDigest) {
		return RedactionProvenanceTarget{}, fmt.Errorf("redaction provenance promoted record lacks derived-from source")
	}
	sourceDigest, err := custody.CanonicalDigest(source)
	if err != nil {
		return RedactionProvenanceTarget{}, err
	}
	eventDigest, err := custody.HandlingEventDigest(event)
	if err != nil {
		return RedactionProvenanceTarget{}, err
	}
	promotedDigest, err := custody.CanonicalDigest(promoted)
	if err != nil {
		return RedactionProvenanceTarget{}, err
	}
	return RedactionProvenanceTarget{
		Kind:                 RedactionProvenanceTargetKind,
		SourceCustodyID:      source.CustodyID,
		SourceRecordDigest:   sourceDigest,
		EventID:              event.EventID,
		EventDigest:          eventDigest,
		OriginalDigest:       event.OriginalDigest,
		ResultingDigest:      event.ResultingDigest,
		PromotedCustodyID:    promoted.CustodyID,
		PromotedRecordDigest: promotedDigest,
	}, nil
}

func hasDerivedFrom(record custody.Record, digest string) bool {
	for _, parent := range record.Parents {
		if parent.Relation == custody.DerivedFrom && parent.Digest == digest {
			return true
		}
	}
	return false
}

func verifyRedactionProvenanceRecords(source custody.Record, event custody.HandlingEvent, promoted custody.Record, records custody.RecordStore, artifacts store.Store) error {
	if artifacts == nil {
		return fmt.Errorf("artifact store is required")
	}
	if err := artifacts.VerifyReference(source.Artifact); err != nil {
		return fmt.Errorf("verify redaction provenance source artifact: %w", err)
	}
	if err := artifacts.Verify(event.OriginalDigest); err != nil {
		return fmt.Errorf("verify redaction provenance original artifact: %w", err)
	}
	if err := artifacts.VerifyReference(promoted.Artifact); err != nil {
		return fmt.Errorf("verify redaction provenance resulting artifact: %w", err)
	}
	if records != nil {
		if _, err := custody.VerifyRecord(records, artifacts, promoted.CustodyID); err != nil {
			return fmt.Errorf("verify redaction provenance promoted record: %w", err)
		}
	}
	return nil
}

func redactionProvenanceSigningMessage(target RedactionProvenanceTarget, keyID string) ([]byte, error) {
	if !keyIDPattern.MatchString(keyID) {
		return nil, fmt.Errorf("invalid redaction provenance key id %q", keyID)
	}
	return []byte(strings.Join([]string{
		RedactionProvenanceSchema,
		RedactionProvenanceTargetKind,
		keyID,
		target.SourceCustodyID,
		target.SourceRecordDigest,
		target.EventID,
		target.EventDigest,
		target.OriginalDigest,
		target.ResultingDigest,
		target.PromotedCustodyID,
		target.PromotedRecordDigest,
		"",
	}, "\n")), nil
}

func redactionProvenanceReceipt(envelope RedactionProvenanceEnvelope, digest string) RedactionProvenanceVerificationReceipt {
	target := envelope.Target
	return RedactionProvenanceVerificationReceipt{
		SourceCustodyID:      target.SourceCustodyID,
		SourceRecordDigest:   target.SourceRecordDigest,
		EventID:              target.EventID,
		EventDigest:          target.EventDigest,
		OriginalDigest:       target.OriginalDigest,
		ResultingDigest:      target.ResultingDigest,
		PromotedCustodyID:    target.PromotedCustodyID,
		PromotedRecordDigest: target.PromotedRecordDigest,
		AttestationDigest:    digest,
		KeyID:                envelope.KeyID,
		Algorithm:            envelope.Algorithm,
		Verified:             true,
	}
}
