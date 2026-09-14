package main

import (
	"testing"

	"ingen/sorna/internal/lifecycle"
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

func TestLifecycleObservationCoverageNamesSamplingBlindSpots(t *testing.T) {
	if got := lifecycleObservationCoverage(nil); got != "unavailable" {
		t.Fatalf("nil coverage = %q, want unavailable", got)
	}
	clean := &lifecycle.AccessTelemetry{
		Status:                      "captured",
		ExecutableSampleCount:       3,
		ExecutableObservationCount:  1,
		ExecutableObservationErrors: 0,
	}
	if got := lifecycleObservationCoverage(clean); got != "periodic-best-effort" {
		t.Fatalf("clean coverage = %q, want periodic-best-effort", got)
	}
	withGap := *clean
	withGap.ExecutableObservationErrors = 1
	if got := lifecycleObservationCoverage(&withGap); got != "periodic-best-effort-with-gaps" {
		t.Fatalf("gap coverage = %q, want periodic-best-effort-with-gaps", got)
	}
	if got := lifecycleObservationLimitation(&lifecycle.AccessTelemetry{ExecutableSamplingIntervalMS: 25}); got != "executable identity was sampled every 25 ms; transitions between samples may be unobserved" {
		t.Fatalf("sampling limitation = %q, want explicit interval limitation", got)
	}
}
