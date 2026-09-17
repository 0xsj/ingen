package attestation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/custody"
)

const (
	CleanupReadinessSchema               = "lockwood.cleanup-readiness/v1"
	CleanupReadinessBlocked              = "blocked"
	CleanupReadinessReadyForRevalidation = "ready-for-revalidation"
)

// CleanupReadinessReport is a read-only evidence gate. A ready report means
// the supplied plan, policy, hold, and authorization evidence is internally
// consistent; it deliberately leaves fresh local revalidation and coordination
// as blockers and never grants deletion authority.
type CleanupReadinessReport struct {
	Schema            string   `json:"schema"`
	TargetDigest      string   `json:"target_digest"`
	CleanupPlanDigest string   `json:"cleanup_plan_digest"`
	State             string   `json:"state"`
	ActionStatus      string   `json:"action_status"`
	Blockers          []string `json:"blockers"`
}

// EvaluateCleanupReadiness combines a plan entry with typed policy, hold, and
// authorization evidence. It does not read storage, acquire a lease, or
// change any custody or artifact state.
func EvaluateCleanupReadiness(plan custody.CleanupPlan, target CleanupAuthorizationTarget, request CleanupAuthorizationRequest, result CleanupAuthorizationResult) CleanupReadinessReport {
	report := CleanupReadinessReport{
		Schema:       CleanupReadinessSchema,
		TargetDigest: target.ArtifactDigest,
		State:        CleanupReadinessBlocked,
		ActionStatus: custody.CleanupNotAuthorized,
		Blockers:     make([]string, 0),
	}
	planDigest, err := custody.CleanupPlanDigest(plan)
	if err != nil {
		report.Blockers = append(report.Blockers, "cleanup plan is invalid: "+err.Error())
		return report
	}
	report.CleanupPlanDigest = planDigest
	if err := target.Validate(); err != nil {
		report.Blockers = append(report.Blockers, "cleanup target is invalid: "+err.Error())
	} else {
		if target.CleanupPlanDigest != planDigest {
			report.Blockers = append(report.Blockers, "cleanup target plan digest does not match supplied plan")
		}
		entryFound := false
		for _, entry := range plan.Entries {
			if entry.Digest != target.ArtifactDigest {
				continue
			}
			entryFound = true
			if entry.State != custody.CleanupCandidateState {
				report.Blockers = append(report.Blockers, "cleanup target is not a cleanup-candidate plan entry")
			}
			if entry.ActionStatus != custody.CleanupNotAuthorized {
				report.Blockers = append(report.Blockers, "cleanup plan entry action status is not not-authorized")
			}
			break
		}
		if !entryFound {
			report.Blockers = append(report.Blockers, "cleanup target is absent from supplied cleanup plan")
		}
	}
	if request.Target != target {
		report.Blockers = append(report.Blockers, "cleanup authorization request target does not match readiness target")
	}
	if err := request.Validate(); err != nil {
		report.Blockers = append(report.Blockers, "cleanup authorization request is invalid: "+err.Error())
	} else if err := result.ValidateAgainst(request); err != nil {
		report.Blockers = append(report.Blockers, "cleanup authorization result is invalid: "+err.Error())
	}
	if len(report.Blockers) == 0 {
		report.State = CleanupReadinessReadyForRevalidation
		report.Blockers = append(report.Blockers, "fresh exclusive local revalidation and cleanup coordination are still required")
	}
	return report
}

func (report CleanupReadinessReport) Validate() error {
	if report.Schema != CleanupReadinessSchema {
		return fmt.Errorf("unexpected cleanup readiness schema %q", report.Schema)
	}
	if err := artifact.ValidateDigest(report.TargetDigest); err != nil {
		return fmt.Errorf("invalid cleanup readiness target digest: %w", err)
	}
	if err := artifact.ValidateDigest(report.CleanupPlanDigest); err != nil {
		return fmt.Errorf("invalid cleanup readiness plan digest: %w", err)
	}
	switch report.State {
	case CleanupReadinessBlocked, CleanupReadinessReadyForRevalidation:
	default:
		return fmt.Errorf("invalid cleanup readiness state %q", report.State)
	}
	if report.ActionStatus != custody.CleanupNotAuthorized {
		return fmt.Errorf("invalid cleanup readiness action status %q", report.ActionStatus)
	}
	if len(report.Blockers) == 0 {
		return fmt.Errorf("cleanup readiness blockers are required")
	}
	for _, blocker := range report.Blockers {
		if blocker == "" {
			return fmt.Errorf("cleanup readiness blocker cannot be empty")
		}
	}
	return nil
}

func MarshalCanonicalCleanupReadinessReport(report CleanupReadinessReport) ([]byte, error) {
	if err := report.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(report)
}

func UnmarshalCanonicalCleanupReadinessReport(data []byte) (CleanupReadinessReport, error) {
	var report CleanupReadinessReport
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return CleanupReadinessReport{}, fmt.Errorf("decode cleanup readiness report: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return CleanupReadinessReport{}, fmt.Errorf("cleanup readiness report contains multiple JSON values")
		}
		return CleanupReadinessReport{}, fmt.Errorf("decode cleanup readiness report: %w", err)
	}
	canonical, err := MarshalCanonicalCleanupReadinessReport(report)
	if err != nil {
		return CleanupReadinessReport{}, err
	}
	if !bytes.Equal(data, canonical) {
		return CleanupReadinessReport{}, fmt.Errorf("cleanup readiness report is not canonical JSON")
	}
	return report, nil
}
