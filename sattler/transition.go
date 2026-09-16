package sattler

import "fmt"

// TransitionClassification is a neutral description of a primary state
// transition. It does not rank a change as good or bad.
type TransitionClassification string

const (
	TransitionUnchanged    TransitionClassification = "unchanged"
	TransitionChanged      TransitionClassification = "changed"
	TransitionIncompatible TransitionClassification = "incompatible"
)

// StateTransition records the primary state field selected by an adapter.
// The field is explicit because not every artifact type has a verdict.
type StateTransition struct {
	Field          string                   `json:"field"`
	Before         string                   `json:"before"`
	After          string                   `json:"after"`
	Classification TransitionClassification `json:"classification"`
}

// NewStateTransition classifies a primary state without inferring direction,
// quality, or causation.
func NewStateTransition(field, before, after string, compatible bool) StateTransition {
	classification := TransitionUnchanged
	if !compatible {
		classification = TransitionIncompatible
	} else if before != after {
		classification = TransitionChanged
	}
	return StateTransition{Field: field, Before: before, After: after, Classification: classification}
}

func (transition StateTransition) String() string {
	return fmt.Sprintf("%s %q -> %q [%s]", transition.Field, transition.Before, transition.After, transition.Classification)
}
