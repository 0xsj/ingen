package attestation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode"

	"ingen/lockwood/internal/artifact"
)

const (
	AuthorizationHandoffSchema = "lockwood.authorization-handoff/v1"

	AuthorizationActionHandlingEvent      = "handling-event"
	AuthorizationActionRedactionPromotion = "redaction-promotion"
	AuthorizationActionCustodyRecord      = "custody-record"
	AuthorizationActionCleanupDelete      = "cleanup-delete"

	AuthorizationTargetArtifactBlob = "artifact-blob"

	AuthorizationDecisionAuthorized = "authorized"
	AuthorizationDecisionDenied     = "denied"
)

var authorizationAuthorityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,255}$`)

// AuthorizationHandoffTarget identifies the exact local operation for which
// external authorization evidence is being supplied.
type AuthorizationHandoffTarget struct {
	Kind   string `json:"kind"`
	Digest string `json:"digest"`
}

// AuthorizationPrincipalReference carries an opaque external identity
// relationship. It is descriptive unless the external identity owner verifies
// the optional assertion digest through its own trust boundary.
type AuthorizationPrincipalReference struct {
	Authority       string `json:"authority"`
	Reference       string `json:"reference"`
	AssertionDigest string `json:"assertion_digest,omitempty"`
}

// AuthorizationEvidence identifies the external authorization evidence and
// its validity/revocation snapshots. Lockwood does not verify the external
// authority's signature or interpret its identity semantics here.
type AuthorizationEvidence struct {
	Authority                string `json:"authority"`
	Decision                 string `json:"decision"`
	AssertionDigest          string `json:"assertion_digest"`
	PolicyDigest             string `json:"policy_digest,omitempty"`
	IssuedAt                 string `json:"issued_at"`
	NotBefore                string `json:"not_before"`
	ExpiresAt                string `json:"expires_at"`
	RevocationSnapshotDigest string `json:"revocation_snapshot_digest"`
}

// AuthorizationHandoff is a strict, read-only transport value for passing
// independently verifiable external evidence into Lockwood. It does not
// authenticate an actor or grant authorization by itself.
type AuthorizationHandoff struct {
	Schema        string                          `json:"schema"`
	Target        AuthorizationHandoffTarget      `json:"target"`
	Action        string                          `json:"action"`
	Principal     AuthorizationPrincipalReference `json:"principal"`
	Authorization AuthorizationEvidence           `json:"authorization"`
}

// NewAuthorizationHandoff builds a versioned handoff from a validated target
// and external evidence references. The action is derived from the target kind
// so callers cannot accidentally authorize one action against another target.
func NewAuthorizationHandoff(target AuthorizationHandoffTarget, principal AuthorizationPrincipalReference, authorization AuthorizationEvidence) (AuthorizationHandoff, error) {
	handoff := AuthorizationHandoff{
		Schema:        AuthorizationHandoffSchema,
		Target:        target,
		Action:        authorizationActionForTarget(target.Kind),
		Principal:     principal,
		Authorization: authorization,
	}
	if err := handoff.Validate(); err != nil {
		return AuthorizationHandoff{}, err
	}
	return handoff, nil
}

// Validate checks the canonical shape and intrinsic validity of a handoff.
// It does not verify external signatures, policy bytes, identity assertions,
// or revocation snapshots.
func (handoff AuthorizationHandoff) Validate() error {
	if handoff.Schema != AuthorizationHandoffSchema {
		return fmt.Errorf("unexpected authorization handoff schema %q", handoff.Schema)
	}
	if err := handoff.Target.Validate(); err != nil {
		return err
	}
	if !validAuthorizationAction(handoff.Action) {
		return fmt.Errorf("unsupported authorization handoff action %q", handoff.Action)
	}
	if !authorizationActionMatchesTarget(handoff.Action, handoff.Target.Kind) {
		return fmt.Errorf("authorization handoff action %q does not match target kind %q", handoff.Action, handoff.Target.Kind)
	}
	if err := validateAuthorizationPrincipal(handoff.Principal); err != nil {
		return err
	}
	if err := validateAuthorizationEvidence(handoff.Authorization); err != nil {
		return err
	}
	return nil
}

// Validate checks the supported target kind and its canonical digest shape.
func (target AuthorizationHandoffTarget) Validate() error {
	return validateAuthorizationTarget(target)
}

// ValidateAt checks that the handoff is valid at an explicit evaluation time.
// Validity windows are inclusive at not_before and exclusive at expires_at.
func (handoff AuthorizationHandoff) ValidateAt(evaluatedAt time.Time) error {
	if err := handoff.Validate(); err != nil {
		return err
	}
	if evaluatedAt.IsZero() {
		return fmt.Errorf("authorization handoff evaluation time is required")
	}
	issuedAt, _ := parseRegistryTime("issued_at", handoff.Authorization.IssuedAt)
	notBefore, _ := parseRegistryTime("not_before", handoff.Authorization.NotBefore)
	expiresAt, _ := parseRegistryTime("expires_at", handoff.Authorization.ExpiresAt)
	evaluatedAt = evaluatedAt.UTC()
	if evaluatedAt.Before(notBefore) {
		return fmt.Errorf("authorization handoff is not yet active at %s", evaluatedAt.Format(time.RFC3339))
	}
	if !evaluatedAt.Before(expiresAt) {
		return fmt.Errorf("authorization handoff is expired at %s", evaluatedAt.Format(time.RFC3339))
	}
	if issuedAt.After(evaluatedAt) {
		return fmt.Errorf("authorization handoff was issued after evaluation time")
	}
	return nil
}

// MarshalCanonicalAuthorizationHandoff returns the compact canonical JSON
// representation used for handoff identity and snapshot comparison.
func MarshalCanonicalAuthorizationHandoff(handoff AuthorizationHandoff) ([]byte, error) {
	if err := handoff.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(handoff)
}

// UnmarshalCanonicalAuthorizationHandoff strictly decodes one canonical
// handoff and rejects unknown fields, multiple values, or noncanonical JSON.
func UnmarshalCanonicalAuthorizationHandoff(data []byte) (AuthorizationHandoff, error) {
	var handoff AuthorizationHandoff
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&handoff); err != nil {
		return AuthorizationHandoff{}, fmt.Errorf("decode authorization handoff: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return AuthorizationHandoff{}, fmt.Errorf("authorization handoff contains multiple JSON values")
		}
		return AuthorizationHandoff{}, fmt.Errorf("decode authorization handoff: %w", err)
	}
	canonical, err := MarshalCanonicalAuthorizationHandoff(handoff)
	if err != nil {
		return AuthorizationHandoff{}, err
	}
	if !bytes.Equal(data, canonical) {
		return AuthorizationHandoff{}, fmt.Errorf("authorization handoff is not canonical JSON")
	}
	return handoff, nil
}

// AuthorizationHandoffDigest identifies the canonical handoff transport
// value. It is distinct from the target digest and external assertion digest.
func AuthorizationHandoffDigest(handoff AuthorizationHandoff) (string, error) {
	encoded, err := MarshalCanonicalAuthorizationHandoff(handoff)
	if err != nil {
		return "", err
	}
	return artifact.DigestBytes(encoded), nil
}

func validateAuthorizationTarget(target AuthorizationHandoffTarget) error {
	if !validAuthorizationTargetKind(target.Kind) {
		return fmt.Errorf("unsupported authorization handoff target kind %q", target.Kind)
	}
	if err := artifact.ValidateDigest(target.Digest); err != nil {
		return fmt.Errorf("invalid authorization handoff target digest: %w", err)
	}
	return nil
}

func validateAuthorizationPrincipal(principal AuthorizationPrincipalReference) error {
	if !authorizationAuthorityPattern.MatchString(principal.Authority) {
		return fmt.Errorf("invalid authorization principal authority %q", principal.Authority)
	}
	if !validOpaqueAuthorizationReference(principal.Reference) {
		return fmt.Errorf("invalid authorization principal reference")
	}
	if principal.AssertionDigest != "" {
		if err := artifact.ValidateDigest(principal.AssertionDigest); err != nil {
			return fmt.Errorf("invalid authorization principal assertion digest: %w", err)
		}
	}
	return nil
}

func validateAuthorizationEvidence(evidence AuthorizationEvidence) error {
	if !authorizationAuthorityPattern.MatchString(evidence.Authority) {
		return fmt.Errorf("invalid authorization authority %q", evidence.Authority)
	}
	switch evidence.Decision {
	case AuthorizationDecisionAuthorized, AuthorizationDecisionDenied:
	default:
		return fmt.Errorf("invalid authorization decision %q", evidence.Decision)
	}
	if err := artifact.ValidateDigest(evidence.AssertionDigest); err != nil {
		return fmt.Errorf("invalid authorization assertion digest: %w", err)
	}
	if err := artifact.ValidateDigest(evidence.RevocationSnapshotDigest); err != nil {
		return fmt.Errorf("invalid authorization revocation digest: %w", err)
	}
	if evidence.PolicyDigest != "" {
		if err := artifact.ValidateDigest(evidence.PolicyDigest); err != nil {
			return fmt.Errorf("invalid authorization policy digest: %w", err)
		}
	}
	issuedAt, err := parseRequiredAuthorizationTime("issued_at", evidence.IssuedAt)
	if err != nil {
		return err
	}
	notBefore, err := parseRequiredAuthorizationTime("not_before", evidence.NotBefore)
	if err != nil {
		return err
	}
	expiresAt, err := parseRequiredAuthorizationTime("expires_at", evidence.ExpiresAt)
	if err != nil {
		return err
	}
	if issuedAt.After(notBefore) {
		return fmt.Errorf("authorization issued_at must not be after not_before")
	}
	if !expiresAt.After(notBefore) {
		return fmt.Errorf("authorization expires_at must be after not_before")
	}
	return nil
}

func parseRequiredAuthorizationTime(field, value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, fmt.Errorf("authorization %s is required", field)
	}
	parsed, err := parseRegistryTime(field, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed, nil
}

func validAuthorizationAction(action string) bool {
	switch action {
	case AuthorizationActionHandlingEvent, AuthorizationActionRedactionPromotion, AuthorizationActionCustodyRecord, AuthorizationActionCleanupDelete:
		return true
	default:
		return false
	}
}

func validAuthorizationTargetKind(kind string) bool {
	switch kind {
	case AuthorizationActionHandlingEvent, AuthorizationActionRedactionPromotion, AuthorizationActionCustodyRecord, AuthorizationTargetArtifactBlob:
		return true
	default:
		return false
	}
}

func authorizationActionForTarget(targetKind string) string {
	if targetKind == AuthorizationTargetArtifactBlob {
		return AuthorizationActionCleanupDelete
	}
	return targetKind
}

func authorizationActionMatchesTarget(action, targetKind string) bool {
	return action == authorizationActionForTarget(targetKind)
}

func validOpaqueAuthorizationReference(value string) bool {
	if value == "" || len(value) > 512 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return false
		}
	}
	return true
}
