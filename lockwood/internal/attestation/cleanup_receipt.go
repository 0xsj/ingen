package attestation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"ingen/lockwood/internal/artifact"
)

const (
	CleanupOutcomeReceiptSchema = "lockwood.cleanup-outcome-receipt/v1"

	CleanupOutcomeRequested       = "requested"
	CleanupOutcomeAuthorized      = "authorized"
	CleanupOutcomeAttempted       = "attempted"
	CleanupOutcomeSucceeded       = "succeeded"
	CleanupOutcomeFailed          = "failed"
	CleanupOutcomeEligibilityLost = "eligibility-lost"

	CleanupPostconditionNotChecked = "not-checked"
	CleanupPostconditionVerified   = "verified"
	CleanupPostconditionFailed     = "failed"
)

// CleanupOutcomeReceipt is a canonical lifecycle record for a future cleanup
// worker. It is a transport value only; this package does not persist it.
type CleanupOutcomeReceipt struct {
	Schema                       string `json:"schema"`
	ReceiptID                    string `json:"receipt_id"`
	TargetDigest                 string `json:"target_digest"`
	Action                       string `json:"action"`
	CleanupPlanDigest            string `json:"cleanup_plan_digest"`
	PolicySnapshotDigest         string `json:"policy_snapshot_digest"`
	HoldSnapshotDigest           string `json:"hold_snapshot_digest"`
	HandoffDigest                string `json:"handoff_digest"`
	EvaluationTime               string `json:"evaluation_time"`
	AttemptTime                  string `json:"attempt_time,omitempty"`
	AuthorizationAuthority       string `json:"authorization_authority"`
	AuthorizationAssertionDigest string `json:"authorization_assertion_digest"`
	PrincipalReference           string `json:"principal_reference"`
	AuthorizationDecision        string `json:"authorization_decision"`
	HoldDecision                 string `json:"hold_decision"`
	Coordinator                  string `json:"coordinator,omitempty"`
	LeaseID                      string `json:"lease_id,omitempty"`
	FencingToken                 string `json:"fencing_token,omitempty"`
	Outcome                      string `json:"outcome"`
	Postcondition                string `json:"postcondition"`
	FailureCode                  string `json:"failure_code,omitempty"`
}

func (receipt CleanupOutcomeReceipt) Validate() error {
	if receipt.Schema != CleanupOutcomeReceiptSchema {
		return fmt.Errorf("unexpected cleanup outcome receipt schema %q", receipt.Schema)
	}
	for name, value := range map[string]string{
		"receipt":                 receipt.ReceiptID,
		"authorization authority": receipt.AuthorizationAuthority,
		"principal":               receipt.PrincipalReference,
	} {
		if !validOpaqueAuthorizationReference(value) {
			return fmt.Errorf("invalid cleanup outcome receipt %s reference", name)
		}
	}
	if receipt.Action != AuthorizationActionCleanupDelete {
		return fmt.Errorf("cleanup outcome receipt action must be %q", AuthorizationActionCleanupDelete)
	}
	for name, value := range map[string]string{
		"target":                  receipt.TargetDigest,
		"cleanup plan":            receipt.CleanupPlanDigest,
		"policy snapshot":         receipt.PolicySnapshotDigest,
		"hold snapshot":           receipt.HoldSnapshotDigest,
		"handoff":                 receipt.HandoffDigest,
		"authorization assertion": receipt.AuthorizationAssertionDigest,
	} {
		if err := artifact.ValidateDigest(value); err != nil {
			return fmt.Errorf("invalid cleanup outcome receipt %s digest: %w", name, err)
		}
	}
	if _, err := parseRequiredAuthorizationTime("evaluation_time", receipt.EvaluationTime); err != nil {
		return err
	}
	if receipt.AttemptTime != "" {
		if _, err := parseRequiredAuthorizationTime("attempt_time", receipt.AttemptTime); err != nil {
			return err
		}
	}
	if receipt.Coordinator != "" || receipt.LeaseID != "" || receipt.FencingToken != "" {
		if !validOpaqueAuthorizationReference(receipt.Coordinator) || !validOpaqueAuthorizationReference(receipt.LeaseID) || !validOpaqueAuthorizationReference(receipt.FencingToken) {
			return fmt.Errorf("cleanup outcome receipt coordinator, lease id, and fencing token must be supplied together")
		}
	}
	switch receipt.AuthorizationDecision {
	case AuthorizationDecisionAuthorized, AuthorizationDecisionDenied, AuthorizationDecisionIndeterminate:
	default:
		return fmt.Errorf("invalid cleanup outcome authorization decision %q", receipt.AuthorizationDecision)
	}
	switch receipt.HoldDecision {
	case CleanupHoldDecisionNotHeld, CleanupHoldDecisionHeld, CleanupHoldDecisionIndeterminate:
	default:
		return fmt.Errorf("invalid cleanup outcome hold decision %q", receipt.HoldDecision)
	}
	switch receipt.Postcondition {
	case CleanupPostconditionNotChecked, CleanupPostconditionVerified, CleanupPostconditionFailed:
	default:
		return fmt.Errorf("invalid cleanup outcome postcondition %q", receipt.Postcondition)
	}
	switch receipt.Outcome {
	case CleanupOutcomeRequested:
		if receipt.AttemptTime != "" || receipt.Postcondition != CleanupPostconditionNotChecked {
			return fmt.Errorf("requested cleanup outcome cannot include an attempt or postcondition")
		}
	case CleanupOutcomeAuthorized:
		if receipt.AuthorizationDecision != AuthorizationDecisionAuthorized || receipt.AttemptTime != "" || receipt.Postcondition != CleanupPostconditionNotChecked {
			return fmt.Errorf("authorized cleanup outcome is not bound to an unattempted authorized decision")
		}
	case CleanupOutcomeAttempted:
		if receipt.AuthorizationDecision != AuthorizationDecisionAuthorized || receipt.HoldDecision != CleanupHoldDecisionNotHeld || receipt.AttemptTime == "" || receipt.Postcondition != CleanupPostconditionNotChecked {
			return fmt.Errorf("attempted cleanup outcome requires an attempt time and unchecked postcondition")
		}
	case CleanupOutcomeSucceeded:
		if receipt.AuthorizationDecision != AuthorizationDecisionAuthorized || receipt.HoldDecision != CleanupHoldDecisionNotHeld || receipt.AttemptTime == "" || receipt.Postcondition != CleanupPostconditionVerified || receipt.FailureCode != "" {
			return fmt.Errorf("succeeded cleanup outcome is not fully authorized and postcondition-verified")
		}
	case CleanupOutcomeFailed:
		if receipt.FailureCode == "" || (receipt.AttemptTime != "" && receipt.Postcondition != CleanupPostconditionFailed) || (receipt.AttemptTime != "" && (receipt.AuthorizationDecision != AuthorizationDecisionAuthorized || receipt.HoldDecision != CleanupHoldDecisionNotHeld)) {
			return fmt.Errorf("failed cleanup outcome requires a failure code and failed postcondition after an attempt")
		}
	case CleanupOutcomeEligibilityLost:
		if receipt.FailureCode == "" || receipt.Postcondition == CleanupPostconditionVerified {
			return fmt.Errorf("eligibility-lost cleanup outcome requires a failure code and cannot be verified success")
		}
	default:
		return fmt.Errorf("invalid cleanup outcome %q", receipt.Outcome)
	}
	if receipt.AttemptTime != "" && (receipt.LeaseID == "" || receipt.FencingToken == "") {
		return fmt.Errorf("attempted cleanup outcome requires lease and fencing evidence")
	}
	return nil
}

// ValidateAgainst binds a receipt to the exact typed request and result that
// supplied its policy, hold, authorization, and handoff evidence.
func (receipt CleanupOutcomeReceipt) ValidateAgainst(request CleanupAuthorizationRequest, result CleanupAuthorizationResult) error {
	if err := receipt.Validate(); err != nil {
		return err
	}
	if err := result.ValidateAgainst(request); err != nil {
		return fmt.Errorf("validate cleanup authorization result: %w", err)
	}
	handoffDigest, err := AuthorizationHandoffDigest(request.Handoff)
	if err != nil {
		return err
	}
	checks := []struct{ name, got, want string }{
		{"target digest", receipt.TargetDigest, request.Target.ArtifactDigest},
		{"cleanup plan digest", receipt.CleanupPlanDigest, request.Target.CleanupPlanDigest},
		{"policy snapshot digest", receipt.PolicySnapshotDigest, request.Policy.SnapshotDigest},
		{"hold snapshot digest", receipt.HoldSnapshotDigest, request.Hold.SnapshotDigest},
		{"handoff digest", receipt.HandoffDigest, handoffDigest},
		{"evaluation time", receipt.EvaluationTime, request.EvaluatedAt},
		{"authorization authority", receipt.AuthorizationAuthority, request.Handoff.Authorization.Authority},
		{"authorization assertion digest", receipt.AuthorizationAssertionDigest, request.Handoff.Authorization.AssertionDigest},
		{"principal reference", receipt.PrincipalReference, request.Handoff.Principal.Reference},
		{"authorization decision", receipt.AuthorizationDecision, result.Authorization.AuthorizationDecision},
		{"hold decision", receipt.HoldDecision, result.HoldDecision},
	}
	for _, check := range checks {
		if check.got != check.want {
			return fmt.Errorf("cleanup outcome receipt %s mismatch", check.name)
		}
	}
	return nil
}

func MarshalCanonicalCleanupOutcomeReceipt(receipt CleanupOutcomeReceipt) ([]byte, error) {
	if err := receipt.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(receipt)
}

func UnmarshalCanonicalCleanupOutcomeReceipt(data []byte) (CleanupOutcomeReceipt, error) {
	var receipt CleanupOutcomeReceipt
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return CleanupOutcomeReceipt{}, fmt.Errorf("decode cleanup outcome receipt: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return CleanupOutcomeReceipt{}, fmt.Errorf("cleanup outcome receipt contains multiple JSON values")
		}
		return CleanupOutcomeReceipt{}, fmt.Errorf("decode cleanup outcome receipt: %w", err)
	}
	canonical, err := MarshalCanonicalCleanupOutcomeReceipt(receipt)
	if err != nil {
		return CleanupOutcomeReceipt{}, err
	}
	if !bytes.Equal(data, canonical) {
		return CleanupOutcomeReceipt{}, fmt.Errorf("cleanup outcome receipt is not canonical JSON")
	}
	return receipt, nil
}
