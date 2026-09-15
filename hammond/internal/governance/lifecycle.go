package governance

import "fmt"

func transition(state State, event Event) (State, error) {
	return transitionWithPolicy(state, event, "", nil, DefaultReviewPolicy())
}

func transitionWithPolicy(state State, event Event, activeReviewCycleID string, priorEvents []Event, policy ReviewPolicy) (State, error) {
	if state == "" {
		if event.Type != EventRegistered {
			return state, fmt.Errorf("first event must be registered")
		}
		return StateRegistered, nil
	}

	switch event.Type {
	case EventRegistered:
		return state, fmt.Errorf("registered must be the first event")
	case EventReviewOpened:
		if state != StateRegistered && state != StateRejected {
			return state, fmt.Errorf("review-opened is only valid from registered or rejected")
		}
		return StateInReview, nil
	case EventApprovalRecorded:
		if state != StateInReview {
			return state, fmt.Errorf("approval-recorded is only valid from in_review")
		}
		candidateEvents := append(append([]Event(nil), priorEvents...), event)
		cycleID := event.ReviewCycleID
		if cycleID == "" {
			cycleID = activeReviewCycleID
		}
		if policy.satisfied(candidateEvents, cycleID) {
			return StateApproved, nil
		}
		return StateInReview, nil
	case EventRejectionRecorded:
		if state != StateInReview {
			return state, fmt.Errorf("rejection-recorded is only valid from in_review")
		}
		return StateRejected, nil
	case EventAmendmentCreated:
		if state != StateApproved {
			return state, fmt.Errorf("amendment-created is only valid from approved")
		}
		return state, nil
	case EventSuperseded:
		if state != StateApproved {
			return state, fmt.Errorf("superseded is only valid from approved")
		}
		return StateSuperseded, nil
	default:
		return state, fmt.Errorf("unsupported event type %q", event.Type)
	}
}
