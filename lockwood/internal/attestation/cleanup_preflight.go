package attestation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/custody"
)

const (
	CleanupRevalidationSchema    = "lockwood.cleanup-revalidation/v1"
	CleanupLeaseSchema           = "lockwood.cleanup-lease/v1"
	CleanupWorkerPreflightSchema = "lockwood.cleanup-worker-preflight/v1"

	CleanupRevalidationClear         = "clear"
	CleanupRevalidationBlocked       = "blocked"
	CleanupRevalidationIndeterminate = "indeterminate"

	CleanupLeaseValid         = "valid"
	CleanupLeaseExpired       = "expired"
	CleanupLeaseLost          = "lost"
	CleanupLeaseIndeterminate = "indeterminate"

	CleanupWorkerPreflightBlocked = "blocked"
	CleanupWorkerPreflightReady   = "ready-for-worker"
)

// CleanupRevalidationEvidence is a caller-supplied report from a fresh local
// scan. Lockwood validates its binding but does not perform the scan here.
type CleanupRevalidationEvidence struct {
	Schema            string   `json:"schema"`
	TargetDigest      string   `json:"target_digest"`
	CleanupPlanDigest string   `json:"cleanup_plan_digest"`
	ObservedDigest    string   `json:"observed_digest"`
	ObservedSizeBytes int64    `json:"observed_size_bytes"`
	CheckedAt         string   `json:"checked_at"`
	ReferenceState    string   `json:"reference_state"`
	RecoveryState     string   `json:"recovery_state"`
	ProtectionState   string   `json:"protection_state"`
	State             string   `json:"state"`
	Blockers          []string `json:"blockers"`
}

func (evidence CleanupRevalidationEvidence) Validate() error {
	if evidence.Schema != CleanupRevalidationSchema {
		return fmt.Errorf("unexpected cleanup revalidation schema %q", evidence.Schema)
	}
	for field, value := range map[string]string{
		"target":       evidence.TargetDigest,
		"cleanup plan": evidence.CleanupPlanDigest,
		"observed":     evidence.ObservedDigest,
	} {
		if err := artifact.ValidateDigest(value); err != nil {
			return fmt.Errorf("invalid cleanup revalidation %s digest: %w", field, err)
		}
	}
	if evidence.ObservedDigest != evidence.TargetDigest {
		return fmt.Errorf("cleanup revalidation observed digest does not match target")
	}
	if evidence.ObservedSizeBytes < 0 {
		return fmt.Errorf("cleanup revalidation observed size cannot be negative")
	}
	if _, err := parseRequiredAuthorizationTime("checked_at", evidence.CheckedAt); err != nil {
		return err
	}
	for name, state := range map[string]string{
		"reference":  evidence.ReferenceState,
		"recovery":   evidence.RecoveryState,
		"protection": evidence.ProtectionState,
	} {
		if err := validateCleanupEvidenceState(name, state); err != nil {
			return err
		}
	}
	switch evidence.State {
	case CleanupRevalidationClear:
		if evidence.ReferenceState != CleanupRevalidationClear || evidence.RecoveryState != CleanupRevalidationClear || evidence.ProtectionState != CleanupRevalidationClear {
			return fmt.Errorf("clear cleanup revalidation requires every sub-check to be clear")
		}
		if len(evidence.Blockers) != 0 {
			return fmt.Errorf("clear cleanup revalidation cannot have blockers")
		}
	case CleanupRevalidationBlocked, CleanupRevalidationIndeterminate:
		if len(evidence.Blockers) == 0 {
			return fmt.Errorf("non-clear cleanup revalidation requires blockers")
		}
	default:
		return fmt.Errorf("invalid cleanup revalidation state %q", evidence.State)
	}
	return validateCleanupBlockers(evidence.Blockers, "cleanup revalidation")
}

// CleanupLeaseEvidence is a caller-supplied coordinator result. The lease is
// bound to the exact target and plan, but this package does not acquire or
// renew it.
type CleanupLeaseEvidence struct {
	Schema            string   `json:"schema"`
	TargetDigest      string   `json:"target_digest"`
	CleanupPlanDigest string   `json:"cleanup_plan_digest"`
	Coordinator       string   `json:"coordinator"`
	LeaseID           string   `json:"lease_id"`
	FencingToken      string   `json:"fencing_token"`
	AcquiredAt        string   `json:"acquired_at"`
	ExpiresAt         string   `json:"expires_at"`
	State             string   `json:"state"`
	Blockers          []string `json:"blockers"`
}

func (evidence CleanupLeaseEvidence) Validate() error {
	if evidence.Schema != CleanupLeaseSchema {
		return fmt.Errorf("unexpected cleanup lease schema %q", evidence.Schema)
	}
	if err := artifact.ValidateDigest(evidence.TargetDigest); err != nil {
		return fmt.Errorf("invalid cleanup lease target digest: %w", err)
	}
	if err := artifact.ValidateDigest(evidence.CleanupPlanDigest); err != nil {
		return fmt.Errorf("invalid cleanup lease plan digest: %w", err)
	}
	if !validOpaqueAuthorizationReference(evidence.Coordinator) || !validOpaqueAuthorizationReference(evidence.LeaseID) || !validOpaqueAuthorizationReference(evidence.FencingToken) {
		return fmt.Errorf("cleanup lease coordinator, lease id, and fencing token are required opaque references")
	}
	acquiredAt, err := parseRequiredAuthorizationTime("acquired_at", evidence.AcquiredAt)
	if err != nil {
		return err
	}
	expiresAt, err := parseRequiredAuthorizationTime("expires_at", evidence.ExpiresAt)
	if err != nil {
		return err
	}
	if !expiresAt.After(acquiredAt) {
		return fmt.Errorf("cleanup lease expires_at must be after acquired_at")
	}
	switch evidence.State {
	case CleanupLeaseValid:
		if len(evidence.Blockers) != 0 {
			return fmt.Errorf("valid cleanup lease cannot have blockers")
		}
	case CleanupLeaseExpired, CleanupLeaseLost, CleanupLeaseIndeterminate:
		if len(evidence.Blockers) == 0 {
			return fmt.Errorf("non-valid cleanup lease requires blockers")
		}
	default:
		return fmt.Errorf("invalid cleanup lease state %q", evidence.State)
	}
	return validateCleanupBlockers(evidence.Blockers, "cleanup lease")
}

// CleanupWorkerPreflight records the final read-only handoff before a future
// worker. Even its positive state remains not-authorized in this version.
type CleanupWorkerPreflight struct {
	Schema            string   `json:"schema"`
	TargetDigest      string   `json:"target_digest"`
	CleanupPlanDigest string   `json:"cleanup_plan_digest"`
	State             string   `json:"state"`
	ActionStatus      string   `json:"action_status"`
	Blockers          []string `json:"blockers"`
}

// EvaluateCleanupWorkerPreflight combines readiness, fresh revalidation, and
// lease evidence. It never acquires coordination or mutates storage.
func EvaluateCleanupWorkerPreflight(readiness CleanupReadinessReport, revalidation CleanupRevalidationEvidence, lease CleanupLeaseEvidence, evaluatedAt time.Time) CleanupWorkerPreflight {
	report := CleanupWorkerPreflight{
		Schema:            CleanupWorkerPreflightSchema,
		TargetDigest:      readiness.TargetDigest,
		CleanupPlanDigest: readiness.CleanupPlanDigest,
		State:             CleanupWorkerPreflightBlocked,
		ActionStatus:      custody.CleanupNotAuthorized,
		Blockers:          make([]string, 0),
	}
	if err := readiness.Validate(); err != nil {
		report.Blockers = append(report.Blockers, "cleanup readiness is invalid: "+err.Error())
	} else if readiness.State != CleanupReadinessReadyForRevalidation {
		report.Blockers = append(report.Blockers, "cleanup readiness has not reached ready-for-revalidation")
	}
	if err := revalidation.Validate(); err != nil {
		report.Blockers = append(report.Blockers, "cleanup revalidation is invalid: "+err.Error())
	} else {
		if revalidation.TargetDigest != readiness.TargetDigest || revalidation.CleanupPlanDigest != readiness.CleanupPlanDigest {
			report.Blockers = append(report.Blockers, "cleanup revalidation target or plan binding does not match readiness")
		}
		if revalidation.State != CleanupRevalidationClear {
			report.Blockers = append(report.Blockers, "cleanup revalidation is not clear")
		}
		if revalidation.CheckedAt != evaluatedAt.UTC().Format(time.RFC3339) {
			report.Blockers = append(report.Blockers, "cleanup revalidation time does not match preflight evaluation")
		}
	}
	if err := lease.Validate(); err != nil {
		report.Blockers = append(report.Blockers, "cleanup lease is invalid: "+err.Error())
	} else {
		if lease.TargetDigest != readiness.TargetDigest || lease.CleanupPlanDigest != readiness.CleanupPlanDigest {
			report.Blockers = append(report.Blockers, "cleanup lease target or plan binding does not match readiness")
		}
		if lease.State != CleanupLeaseValid {
			report.Blockers = append(report.Blockers, "cleanup lease is not valid")
		}
		acquiredAt, _ := parseRequiredAuthorizationTime("acquired_at", lease.AcquiredAt)
		expiresAt, _ := parseRequiredAuthorizationTime("expires_at", lease.ExpiresAt)
		if evaluatedAt.Before(acquiredAt) || !evaluatedAt.Before(expiresAt) {
			report.Blockers = append(report.Blockers, "cleanup lease is not valid at preflight evaluation")
		}
	}
	if evaluatedAt.IsZero() {
		report.Blockers = append(report.Blockers, "cleanup preflight evaluation time is required")
	}
	if len(report.Blockers) == 0 {
		report.State = CleanupWorkerPreflightReady
		report.Blockers = append(report.Blockers, "destructive worker is not implemented; no deletion may occur")
	}
	return report
}

func (report CleanupWorkerPreflight) Validate() error {
	if report.Schema != CleanupWorkerPreflightSchema {
		return fmt.Errorf("unexpected cleanup worker preflight schema %q", report.Schema)
	}
	if err := artifact.ValidateDigest(report.TargetDigest); err != nil {
		return fmt.Errorf("invalid cleanup preflight target digest: %w", err)
	}
	if err := artifact.ValidateDigest(report.CleanupPlanDigest); err != nil {
		return fmt.Errorf("invalid cleanup preflight plan digest: %w", err)
	}
	if report.State != CleanupWorkerPreflightBlocked && report.State != CleanupWorkerPreflightReady {
		return fmt.Errorf("invalid cleanup worker preflight state %q", report.State)
	}
	if report.ActionStatus != custody.CleanupNotAuthorized {
		return fmt.Errorf("invalid cleanup worker preflight action status %q", report.ActionStatus)
	}
	return validateCleanupBlockers(report.Blockers, "cleanup worker preflight")
}

func validateCleanupEvidenceState(name, state string) error {
	switch state {
	case CleanupRevalidationClear, CleanupRevalidationBlocked, CleanupRevalidationIndeterminate:
		return nil
	default:
		return fmt.Errorf("invalid cleanup %s state %q", name, state)
	}
}

func validateCleanupBlockers(blockers []string, name string) error {
	for _, blocker := range blockers {
		if blocker == "" {
			return fmt.Errorf("%s blocker cannot be empty", name)
		}
	}
	return nil
}

func MarshalCanonicalCleanupRevalidation(evidence CleanupRevalidationEvidence) ([]byte, error) {
	if err := evidence.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(evidence)
}

func UnmarshalCanonicalCleanupRevalidation(data []byte) (CleanupRevalidationEvidence, error) {
	var evidence CleanupRevalidationEvidence
	if err := decodeCanonicalCleanupPreflight(data, &evidence, func() ([]byte, error) { return MarshalCanonicalCleanupRevalidation(evidence) }); err != nil {
		return CleanupRevalidationEvidence{}, err
	}
	return evidence, nil
}

func MarshalCanonicalCleanupLease(evidence CleanupLeaseEvidence) ([]byte, error) {
	if err := evidence.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(evidence)
}

func UnmarshalCanonicalCleanupLease(data []byte) (CleanupLeaseEvidence, error) {
	var evidence CleanupLeaseEvidence
	if err := decodeCanonicalCleanupPreflight(data, &evidence, func() ([]byte, error) { return MarshalCanonicalCleanupLease(evidence) }); err != nil {
		return CleanupLeaseEvidence{}, err
	}
	return evidence, nil
}

func MarshalCanonicalCleanupWorkerPreflight(report CleanupWorkerPreflight) ([]byte, error) {
	if err := report.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(report)
}

func UnmarshalCanonicalCleanupWorkerPreflight(data []byte) (CleanupWorkerPreflight, error) {
	var report CleanupWorkerPreflight
	if err := decodeCanonicalCleanupPreflight(data, &report, func() ([]byte, error) { return MarshalCanonicalCleanupWorkerPreflight(report) }); err != nil {
		return CleanupWorkerPreflight{}, err
	}
	return report, nil
}

func decodeCanonicalCleanupPreflight(data []byte, value any, canonical func() ([]byte, error)) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("decode cleanup preflight evidence: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("cleanup preflight evidence contains multiple JSON values")
		}
		return fmt.Errorf("decode cleanup preflight evidence: %w", err)
	}
	canonicalBytes, err := canonical()
	if err != nil {
		return err
	}
	if !bytes.Equal(data, canonicalBytes) {
		return fmt.Errorf("cleanup preflight evidence is not canonical JSON")
	}
	return nil
}
