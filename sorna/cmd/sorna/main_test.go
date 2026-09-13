package main

import (
	"testing"

	"ingen/sorna/internal/mutation"
	"ingen/sorna/internal/runner"
)

func TestExitCodeSeparatesContractVerdictFromKilledMutation(t *testing.T) {
	if got := exitCodeFor(runner.RunRecord{Verdict: runner.ContractVerdict{Status: "pass"}}); got != 0 {
		t.Fatalf("clean pass exit code = %d, want 0", got)
	}
	if got := exitCodeFor(runner.RunRecord{Verdict: runner.ContractVerdict{Status: "fail"}}); got != 1 {
		t.Fatalf("unexpected contract failure exit code = %d, want 1", got)
	}
	if got := exitCodeFor(runner.RunRecord{
		Verdict:  runner.ContractVerdict{Status: "fail"},
		Mutation: &mutation.Result{Outcome: "killed"},
	}); got != 0 {
		t.Fatalf("killed mutation exit code = %d, want 0", got)
	}
	if got := exitCodeFor(runner.RunRecord{
		Verdict:  runner.ContractVerdict{Status: "pass"},
		Mutation: &mutation.Result{Outcome: "survived"},
	}); got != 1 {
		t.Fatalf("surviving mutation exit code = %d, want 1", got)
	}
}
