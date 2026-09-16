package governance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

type membershipDocument struct {
	Schema    string              `json:"schema"`
	ID        string              `json:"id"`
	Version   int                 `json:"version"`
	IssuedAt  string              `json:"issued_at"`
	ExpiresAt string              `json:"expires_at,omitempty"`
	Grants    []AuthorityGrant    `json:"grants"`
	Signature *AuthoritySignature `json:"signature,omitempty"`
}

type membershipSigningDocument struct {
	Schema    string           `json:"schema"`
	ID        string           `json:"id"`
	Version   int              `json:"version"`
	IssuedAt  string           `json:"issued_at"`
	ExpiresAt string           `json:"expires_at,omitempty"`
	Grants    []AuthorityGrant `json:"grants"`
}

// MembershipSnapshot is an authenticated, normalized provider response. Its
// verifier applies grant windows but does not contact the provider again.
type MembershipSnapshot struct {
	Reference MembershipReference
	IssuedAt  string
	ExpiresAt string
	Grants    []AuthorityGrant
	Signature *AuthoritySignature
}

func (snapshot MembershipSnapshot) Validate() error {
	if problems := validateMembershipReference(snapshot.Reference, "membership"); len(problems) > 0 {
		return fmt.Errorf("%s", joinProblems(problems))
	}
	issuedAt, err := parseUTC(snapshot.IssuedAt)
	if err != nil {
		return fmt.Errorf("membership issued_at must be an RFC3339 UTC timestamp: %w", err)
	}
	if snapshot.ExpiresAt != "" {
		expiresAt, err := parseUTC(snapshot.ExpiresAt)
		if err != nil {
			return fmt.Errorf("membership expires_at must be an RFC3339 UTC timestamp: %w", err)
		}
		if !expiresAt.After(issuedAt) {
			return fmt.Errorf("membership expires_at must be after issued_at")
		}
	}
	if snapshot.Grants == nil {
		return fmt.Errorf("membership grants are required")
	}
	if err := validateAuthoritySignature(snapshot.Signature); err != nil {
		return err
	}
	return (TimeScopedAuthority{Grants: snapshot.Grants}).Validate()
}

// Verifier returns a time-scoped verifier over a copy of the normalized
// membership grants.
func (snapshot MembershipSnapshot) Verifier() TimeScopedAuthority {
	return TimeScopedAuthority{Grants: append([]AuthorityGrant(nil), snapshot.Grants...)}
}

// FreshAt checks provider freshness at now. maxAge must be positive and
// maxFutureSkew may be zero; the latter bounds clock skew for future-dated
// responses.
func (snapshot MembershipSnapshot) FreshAt(now string, maxAge, maxFutureSkew time.Duration) error {
	if maxAge <= 0 {
		return fmt.Errorf("membership max age must be positive")
	}
	if maxFutureSkew < 0 {
		return fmt.Errorf("membership max future skew must not be negative")
	}
	if err := snapshot.Validate(); err != nil {
		return err
	}
	nowAt, err := parseUTC(now)
	if err != nil {
		return fmt.Errorf("membership freshness time must be an RFC3339 UTC timestamp: %w", err)
	}
	issuedAt, _ := parseUTC(snapshot.IssuedAt)
	if nowAt.Before(issuedAt.Add(-maxFutureSkew)) {
		return fmt.Errorf("membership snapshot is future-dated beyond allowed skew")
	}
	if nowAt.Sub(issuedAt) > maxAge {
		return fmt.Errorf("membership snapshot is stale")
	}
	if snapshot.ExpiresAt != "" {
		expiresAt, _ := parseUTC(snapshot.ExpiresAt)
		if !nowAt.Before(expiresAt) {
			return fmt.Errorf("membership snapshot is expired")
		}
	}
	return nil
}

// VerifierAt validates freshness before returning a membership verifier for
// policy evaluation.
func (snapshot MembershipSnapshot) VerifierAt(now string, maxAge, maxFutureSkew time.Duration) (TimeScopedAuthority, error) {
	if err := snapshot.FreshAt(now, maxAge, maxFutureSkew); err != nil {
		return TimeScopedAuthority{}, err
	}
	return snapshot.Verifier(), nil
}

// DecodeMembershipSnapshot strictly decodes a membership response and binds
// its exact bytes to the supplied reference.
func DecodeMembershipSnapshot(data []byte, reference MembershipReference) (MembershipSnapshot, error) {
	return decodeMembershipSnapshot(data, reference, nil)
}

// DecodeMembershipSnapshotWithSignatureVerifier verifies a membership
// response against a caller-owned provider trust set.
func DecodeMembershipSnapshotWithSignatureVerifier(data []byte, reference MembershipReference, verifier AuthoritySignatureVerifier) (MembershipSnapshot, error) {
	if verifier == nil {
		return MembershipSnapshot{}, fmt.Errorf("membership signature verifier is required")
	}
	return decodeMembershipSnapshot(data, reference, verifier)
}

func decodeMembershipSnapshot(data []byte, reference MembershipReference, verifier AuthoritySignatureVerifier) (MembershipSnapshot, error) {
	var document membershipDocument
	if err := decodeStrict(data, &document); err != nil {
		return MembershipSnapshot{}, fmt.Errorf("decode Hammond membership: %w", err)
	}
	if document.Schema != MembershipSchema {
		return MembershipSnapshot{}, fmt.Errorf("membership schema must be %s", MembershipSchema)
	}
	if document.ID != reference.ID || document.Version != reference.Version {
		return MembershipSnapshot{}, fmt.Errorf("membership identity does not match its reference")
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != reference.Artifact.SHA256 {
		return MembershipSnapshot{}, fmt.Errorf("membership bytes do not match reference artifact.sha256")
	}
	if verifier != nil {
		if document.Signature == nil {
			return MembershipSnapshot{}, fmt.Errorf("membership signature is required")
		}
		if err := validateAuthoritySignature(document.Signature); err != nil {
			return MembershipSnapshot{}, err
		}
		payload, err := canonicalMembershipPayload(document)
		if err != nil {
			return MembershipSnapshot{}, err
		}
		signature, err := decodeAuthoritySignature(document.Signature.Signature)
		if err != nil {
			return MembershipSnapshot{}, err
		}
		if err := verifier.VerifySignature(document.Signature.KeyID, payload, signature); err != nil {
			return MembershipSnapshot{}, fmt.Errorf("verify membership signature: %w", err)
		}
	}
	snapshot := MembershipSnapshot{
		Reference: reference,
		IssuedAt:  document.IssuedAt,
		ExpiresAt: document.ExpiresAt,
		Grants:    document.Grants,
		Signature: document.Signature,
	}
	if err := snapshot.Validate(); err != nil {
		return MembershipSnapshot{}, err
	}
	return snapshot, nil
}

// LoadMembershipSnapshot loads and verifies a local membership response.
func LoadMembershipSnapshot(reference MembershipReference) (MembershipSnapshot, error) {
	data, err := readLocalArtifact(reference.Artifact.URI)
	if err != nil {
		return MembershipSnapshot{}, fmt.Errorf("read Hammond membership %s: %w", reference.Artifact.URI, err)
	}
	return DecodeMembershipSnapshot(data, reference)
}

// LoadMembershipSnapshotWithSignatureVerifier requires the membership
// response signature to validate against the supplied provider trust set.
func LoadMembershipSnapshotWithSignatureVerifier(reference MembershipReference, verifier AuthoritySignatureVerifier) (MembershipSnapshot, error) {
	data, err := readLocalArtifact(reference.Artifact.URI)
	if err != nil {
		return MembershipSnapshot{}, fmt.Errorf("read Hammond membership %s: %w", reference.Artifact.URI, err)
	}
	return DecodeMembershipSnapshotWithSignatureVerifier(data, reference, verifier)
}

func canonicalMembershipPayload(document membershipDocument) ([]byte, error) {
	grants := append([]AuthorityGrant(nil), document.Grants...)
	sort.Slice(grants, func(left, right int) bool {
		if grants[left].Actor != grants[right].Actor {
			return grants[left].Actor < grants[right].Actor
		}
		if grants[left].Role != grants[right].Role {
			return grants[left].Role < grants[right].Role
		}
		if grants[left].ValidFrom != grants[right].ValidFrom {
			return grants[left].ValidFrom < grants[right].ValidFrom
		}
		return grants[left].ValidUntil < grants[right].ValidUntil
	})
	return json.Marshal(membershipSigningDocument{
		Schema:    document.Schema,
		ID:        document.ID,
		Version:   document.Version,
		IssuedAt:  document.IssuedAt,
		ExpiresAt: document.ExpiresAt,
		Grants:    grants,
	})
}

func joinProblems(problems []string) string {
	if len(problems) == 0 {
		return ""
	}
	result := problems[0]
	for _, problem := range problems[1:] {
		result += "; " + problem
	}
	return result
}
