package attestation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"ingen/lockwood/internal/artifact"
)

const (
	CleanupPolicyReferenceSchema = "lockwood.cleanup-policy-reference/v1"
	CleanupHoldReferenceSchema   = "lockwood.cleanup-hold-reference/v1"

	CleanupEvidenceStatusComplete      = "complete"
	CleanupEvidenceStatusIndeterminate = "indeterminate"
)

// CleanupPolicyReference identifies the canonical external policy bytes and
// records the target-specific evaluation made from them. The digest covers
// the external policy snapshot, not this reference envelope.
type CleanupPolicyReference struct {
	Schema          string `json:"schema"`
	TargetDigest    string `json:"target_digest"`
	PolicyID        string `json:"policy_id"`
	PolicyVersion   string `json:"policy_version"`
	SnapshotDigest  string `json:"snapshot_digest"`
	RetentionRule   string `json:"retention_rule"`
	SourceTimestamp string `json:"source_timestamp"`
	ExpiresAt       string `json:"expires_at"`
	EvaluatedAt     string `json:"evaluated_at"`
	Status          string `json:"status"`
}

func (reference CleanupPolicyReference) Validate() error {
	if reference.Schema != CleanupPolicyReferenceSchema {
		return fmt.Errorf("unexpected cleanup policy reference schema %q", reference.Schema)
	}
	if err := artifact.ValidateDigest(reference.TargetDigest); err != nil {
		return fmt.Errorf("invalid cleanup policy target digest: %w", err)
	}
	if !validOpaqueAuthorizationReference(reference.PolicyID) {
		return fmt.Errorf("invalid cleanup policy id")
	}
	if !validOpaqueAuthorizationReference(reference.PolicyVersion) {
		return fmt.Errorf("invalid cleanup policy version")
	}
	if err := artifact.ValidateDigest(reference.SnapshotDigest); err != nil {
		return fmt.Errorf("invalid cleanup policy snapshot digest: %w", err)
	}
	if !validOpaqueAuthorizationReference(reference.RetentionRule) {
		return fmt.Errorf("invalid cleanup retention rule")
	}
	sourceTimestamp, err := parseRequiredAuthorizationTime("source_timestamp", reference.SourceTimestamp)
	if err != nil {
		return err
	}
	if _, err := parseRequiredAuthorizationTime("expires_at", reference.ExpiresAt); err != nil {
		return err
	}
	evaluatedAt, err := parseRequiredAuthorizationTime("evaluated_at", reference.EvaluatedAt)
	if err != nil {
		return err
	}
	if evaluatedAt.Before(sourceTimestamp) {
		return fmt.Errorf("cleanup policy evaluation time precedes source timestamp")
	}
	switch reference.Status {
	case CleanupEvidenceStatusComplete, CleanupEvidenceStatusIndeterminate:
	default:
		return fmt.Errorf("invalid cleanup policy status %q", reference.Status)
	}
	return nil
}

// CleanupHoldReference identifies the canonical external hold-state snapshot
// used for the exact artifact target at the same evaluation time.
type CleanupHoldReference struct {
	Schema         string `json:"schema"`
	TargetDigest   string `json:"target_digest"`
	SnapshotDigest string `json:"snapshot_digest"`
	Decision       string `json:"decision"`
	EvaluatedAt    string `json:"evaluated_at"`
	Status         string `json:"status"`
}

func (reference CleanupHoldReference) Validate() error {
	if reference.Schema != CleanupHoldReferenceSchema {
		return fmt.Errorf("unexpected cleanup hold reference schema %q", reference.Schema)
	}
	if err := artifact.ValidateDigest(reference.TargetDigest); err != nil {
		return fmt.Errorf("invalid cleanup hold target digest: %w", err)
	}
	if err := artifact.ValidateDigest(reference.SnapshotDigest); err != nil {
		return fmt.Errorf("invalid cleanup hold snapshot digest: %w", err)
	}
	if _, err := parseRequiredAuthorizationTime("evaluated_at", reference.EvaluatedAt); err != nil {
		return err
	}
	switch reference.Decision {
	case CleanupHoldDecisionNotHeld, CleanupHoldDecisionHeld, CleanupHoldDecisionIndeterminate:
	default:
		return fmt.Errorf("invalid cleanup hold reference decision %q", reference.Decision)
	}
	switch reference.Status {
	case CleanupEvidenceStatusComplete:
		if reference.Decision == CleanupHoldDecisionIndeterminate {
			return fmt.Errorf("complete cleanup hold reference cannot be indeterminate")
		}
	case CleanupEvidenceStatusIndeterminate:
		if reference.Decision != CleanupHoldDecisionIndeterminate {
			return fmt.Errorf("indeterminate cleanup hold reference must have indeterminate decision")
		}
	default:
		return fmt.Errorf("invalid cleanup hold status %q", reference.Status)
	}
	return nil
}

func MarshalCanonicalCleanupPolicyReference(reference CleanupPolicyReference) ([]byte, error) {
	if err := reference.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(reference)
}

func UnmarshalCanonicalCleanupPolicyReference(data []byte) (CleanupPolicyReference, error) {
	var reference CleanupPolicyReference
	if err := decodeCanonicalCleanupEvidence(data, &reference, func() ([]byte, error) {
		return MarshalCanonicalCleanupPolicyReference(reference)
	}); err != nil {
		return CleanupPolicyReference{}, err
	}
	return reference, nil
}

func CleanupPolicyReferenceDigest(reference CleanupPolicyReference) (string, error) {
	encoded, err := MarshalCanonicalCleanupPolicyReference(reference)
	if err != nil {
		return "", err
	}
	return artifact.DigestBytes(encoded), nil
}

func MarshalCanonicalCleanupHoldReference(reference CleanupHoldReference) ([]byte, error) {
	if err := reference.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(reference)
}

func UnmarshalCanonicalCleanupHoldReference(data []byte) (CleanupHoldReference, error) {
	var reference CleanupHoldReference
	if err := decodeCanonicalCleanupEvidence(data, &reference, func() ([]byte, error) {
		return MarshalCanonicalCleanupHoldReference(reference)
	}); err != nil {
		return CleanupHoldReference{}, err
	}
	return reference, nil
}

func CleanupHoldReferenceDigest(reference CleanupHoldReference) (string, error) {
	encoded, err := MarshalCanonicalCleanupHoldReference(reference)
	if err != nil {
		return "", err
	}
	return artifact.DigestBytes(encoded), nil
}

func decodeCanonicalCleanupEvidence(data []byte, value any, canonical func() ([]byte, error)) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("decode cleanup evidence: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("cleanup evidence contains multiple JSON values")
		}
		return fmt.Errorf("decode cleanup evidence: %w", err)
	}
	canonicalBytes, err := canonical()
	if err != nil {
		return err
	}
	if !bytes.Equal(data, canonicalBytes) {
		return fmt.Errorf("cleanup evidence is not canonical JSON")
	}
	return nil
}
