package governance

import (
	"fmt"
	"strings"
)

// ReviewPolicy controls when an active review cycle materializes approved
// state. Identity and role authorization remain outside this value for now.
type ReviewPolicy struct {
	MinimumApprovals int
}

// DefaultReviewPolicy is the explicit v1 policy used by the local store and
// CLI: one approval from one distinct actor.
func DefaultReviewPolicy() ReviewPolicy {
	return ReviewPolicy{MinimumApprovals: 1}
}

func (policy ReviewPolicy) Validate() error {
	if policy.MinimumApprovals < 1 {
		return fmt.Errorf("minimum approvals must be positive")
	}
	return nil
}

func (policy ReviewPolicy) satisfied(events []Event, cycleID string) bool {
	if policy.MinimumApprovals < 1 || strings.TrimSpace(cycleID) == "" {
		return false
	}

	actors := make(map[string]struct{})
	for _, event := range events {
		if event.Type != EventApprovalRecorded || event.ReviewCycleID != cycleID || event.Decision != DecisionApprove {
			continue
		}
		actor := strings.TrimSpace(event.Actor)
		if actor != "" {
			actors[actor] = struct{}{}
		}
	}
	return len(actors) >= policy.MinimumApprovals
}

func activeReviewCycle(events []Event) string {
	cycleID := ""
	for _, event := range events {
		if event.Type == EventReviewOpened && strings.TrimSpace(event.ReviewCycleID) != "" {
			cycleID = event.ReviewCycleID
		}
	}
	return cycleID
}
