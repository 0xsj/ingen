package attestation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/custody"
	"ingen/lockwood/internal/store"
)

const HandlingAuthorizationPolicySchema = "lockwood.handling-event-policy/v1"

// HandlingAuthorizationRule allowlists event types for one exact signing key
// ID. The key must still be active in the trust registry at verification time.
type HandlingAuthorizationRule struct {
	KeyID      string                      `json:"key_id"`
	EventTypes []custody.HandlingEventType `json:"event_types"`
}

// HandlingAuthorizationPolicy is a canonical snapshot of action authorization
// for handling-event signatures. It is not a human identity directory.
type HandlingAuthorizationPolicy struct {
	Schema string                      `json:"schema"`
	Rules  []HandlingAuthorizationRule `json:"rules"`
}

func (policy HandlingAuthorizationPolicy) Validate() error {
	if policy.Schema != HandlingAuthorizationPolicySchema {
		return fmt.Errorf("unexpected handling authorization policy schema %q", policy.Schema)
	}
	if policy.Rules == nil {
		return fmt.Errorf("handling authorization policy rules must be an array")
	}
	seenKeys := make(map[string]struct{}, len(policy.Rules))
	for _, rule := range policy.Rules {
		if !keyIDPattern.MatchString(rule.KeyID) {
			return fmt.Errorf("invalid handling authorization policy key id %q", rule.KeyID)
		}
		if _, ok := seenKeys[rule.KeyID]; ok {
			return fmt.Errorf("duplicate handling authorization policy key id %q", rule.KeyID)
		}
		seenKeys[rule.KeyID] = struct{}{}
		if len(rule.EventTypes) == 0 {
			return fmt.Errorf("handling authorization policy key %q must allow at least one event type", rule.KeyID)
		}
		seenTypes := make(map[custody.HandlingEventType]struct{}, len(rule.EventTypes))
		for _, eventType := range rule.EventTypes {
			if !validHandlingEventType(eventType) {
				return fmt.Errorf("handling authorization policy key %q has unsupported event type %q", rule.KeyID, eventType)
			}
			if _, ok := seenTypes[eventType]; ok {
				return fmt.Errorf("handling authorization policy key %q repeats event type %q", rule.KeyID, eventType)
			}
			seenTypes[eventType] = struct{}{}
		}
	}
	return nil
}

// Authorize permits one key/action combination from the policy snapshot.
func (policy HandlingAuthorizationPolicy) Authorize(keyID string, eventType custody.HandlingEventType) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	for _, rule := range policy.Rules {
		if rule.KeyID != keyID {
			continue
		}
		for _, allowed := range rule.EventTypes {
			if allowed == eventType {
				return nil
			}
		}
		return fmt.Errorf("handling key id %q is not authorized for event type %q", keyID, eventType)
	}
	return fmt.Errorf("handling key id %q is not authorized by policy", keyID)
}

// VerifyHandlingEventWithRegistryAndPolicy verifies the signature through the
// trusted key registry and then checks the key/action allowlist.
func VerifyHandlingEventWithRegistryAndPolicy(event custody.HandlingEvent, envelope HandlingEventEnvelope, registry TrustRegistry, policy HandlingAuthorizationPolicy, evaluatedAt time.Time) error {
	if err := VerifyHandlingEventWithRegistry(event, envelope, registry, evaluatedAt); err != nil {
		return err
	}
	return policy.Authorize(envelope.KeyID, event.Type)
}

// AuthorizedHandlingEventVerificationReceipt is transient evidence that a
// signed event verified under both snapshots and passed the action allowlist.
type AuthorizedHandlingEventVerificationReceipt struct {
	TrustedHandlingEventVerificationReceipt
	PolicyDigest string `json:"policy_digest"`
	Authorized   bool   `json:"authorized"`
}

// VerifyPublishedHandlingEventWithRegistryAndPolicyReceipt verifies a
// published event envelope and authorizes its key/action combination through
// explicit trust-registry and policy snapshots.
func VerifyPublishedHandlingEventWithRegistryAndPolicyReceipt(event custody.HandlingEvent, digest string, artifacts store.Store, registry TrustRegistry, policy HandlingAuthorizationPolicy, evaluatedAt time.Time) (AuthorizedHandlingEventVerificationReceipt, error) {
	receipt, err := VerifyPublishedHandlingEventWithRegistryReceipt(event, digest, artifacts, registry, evaluatedAt)
	if err != nil {
		return AuthorizedHandlingEventVerificationReceipt{}, err
	}
	if err := policy.Authorize(receipt.KeyID, event.Type); err != nil {
		return AuthorizedHandlingEventVerificationReceipt{}, err
	}
	policyDigest, err := HandlingAuthorizationPolicyDigest(policy)
	if err != nil {
		return AuthorizedHandlingEventVerificationReceipt{}, err
	}
	return AuthorizedHandlingEventVerificationReceipt{
		TrustedHandlingEventVerificationReceipt: receipt,
		PolicyDigest:                            policyDigest,
		Authorized:                              true,
	}, nil
}

// MarshalCanonicalHandlingAuthorizationPolicy returns compact canonical JSON.
// Rules and event types are sorted so policy identity is input-order stable.
func MarshalCanonicalHandlingAuthorizationPolicy(policy HandlingAuthorizationPolicy) ([]byte, error) {
	rules := make([]HandlingAuthorizationRule, len(policy.Rules))
	for i, rule := range policy.Rules {
		rules[i] = rule
		rules[i].EventTypes = append([]custody.HandlingEventType(nil), rule.EventTypes...)
		sort.Slice(rules[i].EventTypes, func(left, right int) bool {
			return rules[i].EventTypes[left] < rules[i].EventTypes[right]
		})
	}
	sort.Slice(rules, func(left, right int) bool { return rules[left].KeyID < rules[right].KeyID })
	normalized := policy
	normalized.Rules = rules
	if err := normalized.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(normalized)
}

// UnmarshalCanonicalHandlingAuthorizationPolicy strictly decodes one policy
// snapshot and rejects unknown fields, multiple values, or noncanonical JSON.
func UnmarshalCanonicalHandlingAuthorizationPolicy(data []byte) (HandlingAuthorizationPolicy, error) {
	var policy HandlingAuthorizationPolicy
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&policy); err != nil {
		return HandlingAuthorizationPolicy{}, fmt.Errorf("decode handling authorization policy: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return HandlingAuthorizationPolicy{}, fmt.Errorf("handling authorization policy contains multiple JSON values")
		}
		return HandlingAuthorizationPolicy{}, fmt.Errorf("decode handling authorization policy: %w", err)
	}
	canonical, err := MarshalCanonicalHandlingAuthorizationPolicy(policy)
	if err != nil {
		return HandlingAuthorizationPolicy{}, err
	}
	if !bytes.Equal(data, canonical) {
		return HandlingAuthorizationPolicy{}, fmt.Errorf("handling authorization policy is not canonical JSON")
	}
	return policy, nil
}

// HandlingAuthorizationPolicyDigest identifies the canonical policy snapshot.
func HandlingAuthorizationPolicyDigest(policy HandlingAuthorizationPolicy) (string, error) {
	encoded, err := MarshalCanonicalHandlingAuthorizationPolicy(policy)
	if err != nil {
		return "", err
	}
	return artifact.DigestBytes(encoded), nil
}

func validHandlingEventType(eventType custody.HandlingEventType) bool {
	switch eventType {
	case custody.RedactionEvent, custody.RetentionClassifiedEvent, custody.LegalHoldPlacedEvent, custody.LegalHoldReleasedEvent:
		return true
	default:
		return false
	}
}
