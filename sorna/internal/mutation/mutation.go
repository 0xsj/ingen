// Package mutation classifies the sensitivity of a contract run to a named
// behavioral mutation.
package mutation

// Spec describes a mutation before its subject is exercised.
type Spec struct {
	ID              string   `json:"id"`
	Plane           string   `json:"plane"`
	Description     string   `json:"description"`
	ExpectedRuleIDs []string `json:"expected_rule_ids"`
}

// RuleObservation is the small part of a rule result needed for mutation
// classification. The mutation package does not depend on the HTTP runner.
type RuleObservation struct {
	RuleID string
	Status string
}

// Result keeps direct sensitivity separate from setup fallout.
type Result struct {
	Spec                  Spec     `json:"spec"`
	Outcome               string   `json:"outcome"`
	DirectlyFailedRules   []string `json:"directly_failed_rules,omitempty"`
	CascadingInconclusive []string `json:"cascading_inconclusive,omitempty"`
	UnaffectedRules       []string `json:"unaffected_rules,omitempty"`
	UnobservedExpected    []string `json:"unobserved_expected,omitempty"`
	Reason                string   `json:"reason,omitempty"`
}

// Classify determines whether the declared target rule observed the mutation.
// A target failure kills the mutation; an inconclusive target does not.
func Classify(spec Spec, observations []RuleObservation) Result {
	result := Result{Spec: spec, Outcome: "inconclusive"}
	byRule := make(map[string]string, len(observations))
	for _, observation := range observations {
		byRule[observation.RuleID] = observation.Status
		switch observation.Status {
		case "fail":
			result.DirectlyFailedRules = append(result.DirectlyFailedRules, observation.RuleID)
		case "inconclusive":
			result.CascadingInconclusive = append(result.CascadingInconclusive, observation.RuleID)
		case "pass":
			result.UnaffectedRules = append(result.UnaffectedRules, observation.RuleID)
		}
	}

	if len(spec.ExpectedRuleIDs) == 0 {
		result.Reason = "mutation has no expected rule IDs"
		return result
	}
	targetFailed := false
	for _, ruleID := range spec.ExpectedRuleIDs {
		status, observed := byRule[ruleID]
		if !observed {
			result.UnobservedExpected = append(result.UnobservedExpected, ruleID)
			continue
		}
		if status == "fail" {
			targetFailed = true
		}
	}
	if targetFailed {
		result.Outcome = "killed"
		result.Reason = "at least one expected rule failed"
		return result
	}
	if len(result.UnobservedExpected) > 0 || hasExpectedInconclusive(spec.ExpectedRuleIDs, byRule) {
		result.Reason = "an expected rule was not directly observed as failed"
		return result
	}
	result.Outcome = "survived"
	result.Reason = "all expected rules passed"
	return result
}

func hasExpectedInconclusive(expected []string, statuses map[string]string) bool {
	for _, ruleID := range expected {
		if statuses[ruleID] == "inconclusive" || statuses[ruleID] == "skipped" || statuses[ruleID] == "error" {
			return true
		}
	}
	return false
}
