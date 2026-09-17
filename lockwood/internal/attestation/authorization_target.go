package attestation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/custody"
)

const AuthorizationTargetSchema = "lockwood.authorization-target/v1"

const CleanupAuthorizationTargetSchema = "lockwood.cleanup-authorization-target/v1"

// CleanupAuthorizationTarget binds a future cleanup decision to one local
// artifact and the exact read-only plan that produced its candidacy. It is a
// validation and transport value only; it does not authorize deletion.
type CleanupAuthorizationTarget struct {
	Schema            string `json:"schema"`
	Kind              string `json:"kind"`
	Action            string `json:"action"`
	ArtifactDigest    string `json:"artifact_digest"`
	CleanupPlanDigest string `json:"cleanup_plan_digest"`
	PlanAsOf          string `json:"plan_as_of"`
}

// NewCleanupArtifactAuthorizationTarget constructs the exact target proposed
// by CLEANUP-AUTHORIZATION.md. The plan digest identifies canonical
// cleanup-plan/v1 bytes; the caller remains responsible for supplying those
// bytes and for revalidating the candidate before any mutation.
func NewCleanupArtifactAuthorizationTarget(artifactDigest, cleanupPlanDigest, planAsOf string) (CleanupAuthorizationTarget, error) {
	target := CleanupAuthorizationTarget{
		Schema:            CleanupAuthorizationTargetSchema,
		Kind:              AuthorizationTargetArtifactBlob,
		Action:            AuthorizationActionCleanupDelete,
		ArtifactDigest:    artifactDigest,
		CleanupPlanDigest: cleanupPlanDigest,
		PlanAsOf:          planAsOf,
	}
	if err := target.Validate(); err != nil {
		return CleanupAuthorizationTarget{}, err
	}
	return target, nil
}

// AuthorizationTarget converts the cleanup-specific target into the generic
// handoff identity used by the caller-owned authorization verifier.
func (target CleanupAuthorizationTarget) AuthorizationTarget() (AuthorizationHandoffTarget, error) {
	if err := target.Validate(); err != nil {
		return AuthorizationHandoffTarget{}, err
	}
	digest, err := CleanupAuthorizationTargetDigest(target)
	if err != nil {
		return AuthorizationHandoffTarget{}, err
	}
	return AuthorizationHandoffTarget{Kind: target.Kind, Digest: digest}, nil
}

func (target CleanupAuthorizationTarget) Validate() error {
	if target.Schema != CleanupAuthorizationTargetSchema {
		return fmt.Errorf("unexpected cleanup authorization target schema %q", target.Schema)
	}
	if target.Kind != AuthorizationTargetArtifactBlob {
		return fmt.Errorf("unsupported cleanup authorization target kind %q", target.Kind)
	}
	if target.Action != AuthorizationActionCleanupDelete {
		return fmt.Errorf("unsupported cleanup authorization action %q", target.Action)
	}
	if err := artifact.ValidateDigest(target.ArtifactDigest); err != nil {
		return fmt.Errorf("invalid cleanup artifact digest: %w", err)
	}
	if err := artifact.ValidateDigest(target.CleanupPlanDigest); err != nil {
		return fmt.Errorf("invalid cleanup plan digest: %w", err)
	}
	if _, err := parseRequiredAuthorizationTime("plan_as_of", target.PlanAsOf); err != nil {
		return err
	}
	return nil
}

// MarshalCanonicalCleanupAuthorizationTarget returns the compact canonical
// JSON representation whose digest is used in the authorization handoff.
func MarshalCanonicalCleanupAuthorizationTarget(target CleanupAuthorizationTarget) ([]byte, error) {
	if err := target.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(target)
}

// UnmarshalCanonicalCleanupAuthorizationTarget decodes one strict canonical
// cleanup target and rejects unknown fields, multiple values, or reformatting.
func UnmarshalCanonicalCleanupAuthorizationTarget(data []byte) (CleanupAuthorizationTarget, error) {
	var target CleanupAuthorizationTarget
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&target); err != nil {
		return CleanupAuthorizationTarget{}, fmt.Errorf("decode cleanup authorization target: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return CleanupAuthorizationTarget{}, fmt.Errorf("cleanup authorization target contains multiple JSON values")
		}
		return CleanupAuthorizationTarget{}, fmt.Errorf("decode cleanup authorization target: %w", err)
	}
	canonical, err := MarshalCanonicalCleanupAuthorizationTarget(target)
	if err != nil {
		return CleanupAuthorizationTarget{}, err
	}
	if !bytes.Equal(data, canonical) {
		return CleanupAuthorizationTarget{}, fmt.Errorf("cleanup authorization target is not canonical JSON")
	}
	return target, nil
}

// CleanupAuthorizationTargetDigest identifies the canonical cleanup target
// document, distinct from the artifact and cleanup-plan digests it contains.
func CleanupAuthorizationTargetDigest(target CleanupAuthorizationTarget) (string, error) {
	encoded, err := MarshalCanonicalCleanupAuthorizationTarget(target)
	if err != nil {
		return "", err
	}
	return artifact.DigestBytes(encoded), nil
}

// NewCustodyRecordAuthorizationTarget derives a target from the canonical
// custody-record representation. The returned digest identifies the record
// bytes and is not an authorization decision.
func NewCustodyRecordAuthorizationTarget(record custody.Record) (AuthorizationHandoffTarget, error) {
	digest, err := custody.CanonicalDigest(record)
	if err != nil {
		return AuthorizationHandoffTarget{}, fmt.Errorf("derive custody-record authorization target: %w", err)
	}
	return AuthorizationHandoffTarget{
		Kind:   AuthorizationActionCustodyRecord,
		Digest: digest,
	}, nil
}

// NewHandlingEventAuthorizationTarget derives a target from the exact event
// bytes plus the event identity fields that prevent cross-event replay.
func NewHandlingEventAuthorizationTarget(event custody.HandlingEvent) (AuthorizationHandoffTarget, error) {
	digest, err := CanonicalHandlingEventAuthorizationTargetDigest(event)
	if err != nil {
		return AuthorizationHandoffTarget{}, err
	}
	return AuthorizationHandoffTarget{
		Kind:   AuthorizationActionHandlingEvent,
		Digest: digest,
	}, nil
}

// NewRedactionPromotionAuthorizationTarget derives a target from the
// validated source/event/promoted relationship.
func NewRedactionPromotionAuthorizationTarget(source custody.Record, event custody.HandlingEvent, promoted custody.Record) (AuthorizationHandoffTarget, error) {
	provenanceTarget, err := redactionProvenanceTarget(source, event, promoted)
	if err != nil {
		return AuthorizationHandoffTarget{}, err
	}
	digest, err := CanonicalRedactionProvenanceTargetDigest(provenanceTarget)
	if err != nil {
		return AuthorizationHandoffTarget{}, fmt.Errorf("derive redaction-promotion authorization target: %w", err)
	}
	return AuthorizationHandoffTarget{
		Kind:   AuthorizationActionRedactionPromotion,
		Digest: digest,
	}, nil
}

type authorizationHandlingEventTargetDocument struct {
	Schema string                               `json:"schema"`
	Target authorizationHandlingEventTargetBody `json:"target"`
}

type authorizationHandlingEventTargetBody struct {
	Kind        string                    `json:"kind"`
	CustodyID   string                    `json:"custody_id"`
	EventID     string                    `json:"event_id"`
	EventDigest string                    `json:"event_digest"`
	EventType   custody.HandlingEventType `json:"event_type"`
}

// CanonicalHandlingEventAuthorizationTargetDigest returns the SHA-256 digest
// of the compact, domain-separated handling-event target representation.
func CanonicalHandlingEventAuthorizationTargetDigest(event custody.HandlingEvent) (string, error) {
	if err := event.Validate(); err != nil {
		return "", fmt.Errorf("validate handling event authorization target: %w", err)
	}
	eventDigest, err := custody.HandlingEventDigest(event)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(authorizationHandlingEventTargetDocument{
		Schema: AuthorizationTargetSchema,
		Target: authorizationHandlingEventTargetBody{
			Kind:        AuthorizationActionHandlingEvent,
			CustodyID:   event.CustodyID,
			EventID:     event.EventID,
			EventDigest: eventDigest,
			EventType:   event.Type,
		},
	})
	if err != nil {
		return "", fmt.Errorf("marshal handling event authorization target: %w", err)
	}
	return artifact.DigestBytes(encoded), nil
}
