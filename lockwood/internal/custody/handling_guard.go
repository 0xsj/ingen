package custody

import "fmt"

type HandlingAction string

const (
	RedactAction HandlingAction = "redact"
	DeleteAction HandlingAction = "delete"
)

type HandlingGuardStatus string

const (
	HandlingBlocked    HandlingGuardStatus = "blocked"
	HandlingNotBlocked HandlingGuardStatus = "not-blocked"
)

// HandlingGuardDecision reports only whether a visible legal hold blocks a
// payload-changing action. It is not permission to perform that action.
type HandlingGuardDecision struct {
	CustodyID          string              `json:"custody_id"`
	ArtifactDigest     string              `json:"artifact_digest"`
	Action             HandlingAction      `json:"action"`
	Status             HandlingGuardStatus `json:"status"`
	ActiveLegalHoldIDs []string            `json:"active_legal_hold_ids"`
	Basis              string              `json:"basis"`
}

// EvaluateHandlingGuard is a read-only legal-hold guard for redact/delete
// workflows. A not-blocked result does not evaluate retention, authorization,
// actor identity, or whether the requested operation is otherwise permitted.
func EvaluateHandlingGuard(records RecordStore, events HandlingEventStore, record Record, action HandlingAction) (HandlingGuardDecision, error) {
	decision := HandlingGuardDecision{
		CustodyID:          record.CustodyID,
		ArtifactDigest:     record.Artifact.Digest,
		Action:             action,
		ActiveLegalHoldIDs: make([]string, 0),
	}
	switch action {
	case RedactAction, DeleteAction:
	default:
		return decision, fmt.Errorf("unsupported handling guard action %q", action)
	}
	status, err := AnalyzeHandling(records, events, record)
	if err != nil {
		return decision, err
	}
	decision.ActiveLegalHoldIDs = append(decision.ActiveLegalHoldIDs, status.ActiveLegalHoldIDs...)
	if len(decision.ActiveLegalHoldIDs) > 0 {
		decision.Status = HandlingBlocked
		decision.Basis = "active legal hold in visible handling event stream"
		return decision, nil
	}
	decision.Status = HandlingNotBlocked
	decision.Basis = "no active legal hold in visible handling event stream; retention and authorization were not evaluated"
	return decision, nil
}
