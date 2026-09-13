package mutation

import "testing"

func TestClassifySeparatesDirectKillFromCascadingSetupFailure(t *testing.T) {
	result := Classify(Spec{
		ID:              "status-200-create",
		Plane:           "behavior",
		ExpectedRuleIDs: []string{"document.create.valid.accepted"},
	}, []RuleObservation{
		{RuleID: "document.create.valid.accepted", Status: "fail"},
		{RuleID: "document.status.exposes-queued-state", Status: "inconclusive"},
		{RuleID: "document.create.rejects-unsupported-type", Status: "pass"},
	})

	if result.Outcome != "killed" {
		t.Fatalf("outcome = %q, want killed", result.Outcome)
	}
	if len(result.DirectlyFailedRules) != 1 || result.DirectlyFailedRules[0] != "document.create.valid.accepted" {
		t.Fatalf("direct failures = %v, want valid-create rule", result.DirectlyFailedRules)
	}
	if len(result.CascadingInconclusive) != 1 || result.CascadingInconclusive[0] != "document.status.exposes-queued-state" {
		t.Fatalf("cascading inconclusive = %v, want queued-state rule", result.CascadingInconclusive)
	}
	if len(result.UnaffectedRules) != 1 || result.UnaffectedRules[0] != "document.create.rejects-unsupported-type" {
		t.Fatalf("unaffected = %v, want unsupported-type rule", result.UnaffectedRules)
	}
}

func TestClassifyDoesNotCallAnUnobservedMutationKilled(t *testing.T) {
	result := Classify(Spec{ExpectedRuleIDs: []string{"target"}}, []RuleObservation{
		{RuleID: "target", Status: "inconclusive"},
	})
	if result.Outcome != "inconclusive" {
		t.Fatalf("outcome = %q, want inconclusive", result.Outcome)
	}
}

func TestClassifyReportsACompleteSurvivor(t *testing.T) {
	result := Classify(Spec{ExpectedRuleIDs: []string{"target"}}, []RuleObservation{
		{RuleID: "target", Status: "pass"},
	})
	if result.Outcome != "survived" {
		t.Fatalf("outcome = %q, want survived", result.Outcome)
	}
}

func TestClassifyKillsWhenOneOfSeveralTargetsFails(t *testing.T) {
	result := Classify(Spec{ExpectedRuleIDs: []string{"target-a", "target-b"}}, []RuleObservation{
		{RuleID: "target-a", Status: "fail"},
		{RuleID: "target-b", Status: "pass"},
	})
	if result.Outcome != "killed" {
		t.Fatalf("outcome = %q, want killed when one target fails", result.Outcome)
	}
}
