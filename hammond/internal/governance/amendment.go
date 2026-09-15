package governance

import "fmt"

// BuildAmendmentEvent creates the event that links an approved predecessor to
// a separately registered successor. It does not mutate either record.
func BuildAmendmentEvent(predecessor, successor Record, eventID, actor, at string, kind AmendmentKind, reason string) (Event, error) {
	return BuildAmendmentEventWithPolicy(predecessor, successor, eventID, actor, at, kind, reason, DefaultReviewPolicy())
}

// BuildAmendmentEventWithPolicy is the policy-aware form of
// BuildAmendmentEvent.
func BuildAmendmentEventWithPolicy(predecessor, successor Record, eventID, actor, at string, kind AmendmentKind, reason string, policy ReviewPolicy) (Event, error) {
	if err := predecessor.ValidateWithPolicy(policy); err != nil {
		return Event{}, fmt.Errorf("validate predecessor: %w", err)
	}
	if err := successor.ValidateWithPolicy(policy); err != nil {
		return Event{}, fmt.Errorf("validate successor: %w", err)
	}
	if predecessor.State != StateApproved {
		return Event{}, fmt.Errorf("predecessor must be approved")
	}
	if successor.State != StateRegistered || len(successor.Events) != 1 || successor.Events[0].Type != EventRegistered {
		return Event{}, fmt.Errorf("successor must be independently registered")
	}
	if predecessor.RecordID == successor.RecordID {
		return Event{}, fmt.Errorf("predecessor and successor record IDs must differ")
	}

	predecessorIdentity := predecessor.Contract.Identity()
	successorIdentity := successor.Contract.Identity()
	event := Event{
		ID:            eventID,
		Type:          EventAmendmentCreated,
		Actor:         actor,
		At:            at,
		Reason:        reason,
		Predecessor:   &predecessorIdentity,
		Successor:     &successorIdentity,
		AmendmentKind: kind,
	}
	if _, err := predecessor.AppendEventWithPolicy(event, policy); err != nil {
		return Event{}, err
	}
	return event, nil
}

// BuildSupersededEvent creates the event that closes an approved predecessor
// in favor of a successor. It does not mutate the predecessor record.
func BuildSupersededEvent(predecessor, successor Record, eventID, actor, at string) (Event, error) {
	return BuildSupersededEventWithPolicy(predecessor, successor, eventID, actor, at, DefaultReviewPolicy())
}

// BuildSupersededEventWithPolicy is the policy-aware form of
// BuildSupersededEvent.
func BuildSupersededEventWithPolicy(predecessor, successor Record, eventID, actor, at string, policy ReviewPolicy) (Event, error) {
	if err := predecessor.ValidateWithPolicy(policy); err != nil {
		return Event{}, fmt.Errorf("validate predecessor: %w", err)
	}
	if err := successor.ValidateWithPolicy(policy); err != nil {
		return Event{}, fmt.Errorf("validate successor: %w", err)
	}
	if predecessor.State != StateApproved {
		return Event{}, fmt.Errorf("predecessor must be approved before superseding")
	}
	if successor.State != StateApproved {
		return Event{}, fmt.Errorf("successor must be approved before superseding")
	}
	if predecessor.Contract.Identity().Equal(successor.Contract.Identity()) {
		return Event{}, fmt.Errorf("predecessor and successor identities must differ")
	}
	predecessorIdentity := predecessor.Contract.Identity()
	successorIdentity := successor.Contract.Identity()
	if predecessorIdentity.ProjectID != successorIdentity.ProjectID || predecessorIdentity.ID != successorIdentity.ID {
		return Event{}, fmt.Errorf("successor must retain project_id and id")
	}
	if successorIdentity.Version <= predecessorIdentity.Version {
		return Event{}, fmt.Errorf("successor.version must be greater than predecessor.version")
	}

	event := Event{
		ID:        eventID,
		Type:      EventSuperseded,
		Actor:     actor,
		At:        at,
		Successor: &successorIdentity,
	}
	if _, err := predecessor.AppendEventWithPolicy(event, policy); err != nil {
		return Event{}, err
	}
	return event, nil
}
