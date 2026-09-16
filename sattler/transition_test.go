package sattler

import "testing"

func TestNewStateTransitionClassifiesNeutralStates(t *testing.T) {
	tests := []struct {
		name       string
		before     string
		after      string
		compatible bool
		wantClass  TransitionClassification
	}{
		{name: "unchanged", before: "passed", after: "passed", compatible: true, wantClass: TransitionUnchanged},
		{name: "changed", before: "passed", after: "failed", compatible: true, wantClass: TransitionChanged},
		{name: "incompatible", before: "passed", after: "passed", compatible: false, wantClass: TransitionIncompatible},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transition := NewStateTransition("status", test.before, test.after, test.compatible)
			if transition.Classification != test.wantClass {
				t.Fatalf("classification = %q, want %q", transition.Classification, test.wantClass)
			}
			if transition.String() == "" {
				t.Fatal("transition string is empty")
			}
		})
	}
}
