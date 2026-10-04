package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ingen/herdr-sentinel/internal/agentprobe"
	"ingen/herdr-sentinel/internal/codexbroker"
	"ingen/herdr-sentinel/internal/roleexec"
)

func TestNativeCodexPreflightAcceptsOnlyCompleteBoundSyntheticEvidence(t *testing.T) {
	path, digest := readinessExecutableFixture(t)
	called := false
	report, err := checkNativeCodexPreflight(context.Background(), path, func(_ context.Context, request agentprobe.Request) (agentprobe.Report, error) {
		called = true
		if request.ExecutablePath != path || request.Timeout != 0 {
			t.Fatalf("diagnostic request was not fixed to the selected executable/default bounded timeout: %#v", request)
		}
		return supportedReadinessFixture(path, digest), nil
	})
	if err != nil || !called || report.Status != "supported" || report.ExecutableSHA256 != digest {
		t.Fatalf("matching synthetic readiness did not pass: called=%t report=%#v err=%v", called, report, err)
	}
}

func TestNativeCodexPreflightRejectsUnsupportedAndIndeterminateReports(t *testing.T) {
	path, digest := readinessExecutableFixture(t)
	for _, status := range []string{"unsupported", "indeterminate"} {
		t.Run(status, func(t *testing.T) {
			report := supportedReadinessFixture(path, digest)
			report.Status = status
			report.ReasonCode = "managed-preferences-unavailable-under-policy"
			report.Reason = "Codex could not synchronize managed preferences under the enforced Sorna policy"
			got, err := checkNativeCodexPreflight(context.Background(), path, func(context.Context, agentprobe.Request) (agentprobe.Report, error) {
				return report, nil
			})
			var readinessErr *nativeReadinessError
			if err == nil || !errors.As(err, &readinessErr) || readinessErr.Status != status || got.Status != status {
				t.Fatalf("%s readiness report was accepted: report=%#v err=%v", status, got, err)
			}
			if status == "unsupported" && nativeReadinessExitCode(err) != 1 || status == "indeterminate" && nativeReadinessExitCode(err) != 2 {
				t.Fatalf("wrong CLI exit mapping for %s: %d", status, nativeReadinessExitCode(err))
			}
		})
	}
}

func TestNativeCodexPreflightRejectsChangedOrMismatchedExecutable(t *testing.T) {
	t.Run("report digest mismatch", func(t *testing.T) {
		path, digest := readinessExecutableFixture(t)
		report := supportedReadinessFixture(path, strings.Repeat("b", 64))
		got, err := checkNativeCodexPreflight(context.Background(), path, func(context.Context, agentprobe.Request) (agentprobe.Report, error) { return report, nil })
		if err == nil || got.Status != "indeterminate" || got.ReasonCode != "executable-identity-mismatch" || digest == report.ExecutableSHA256 {
			t.Fatalf("mismatched executable digest was accepted: report=%#v err=%v", got, err)
		}
	})
	t.Run("binary changes during diagnosis", func(t *testing.T) {
		path, digest := readinessExecutableFixture(t)
		report := supportedReadinessFixture(path, digest)
		got, err := checkNativeCodexPreflight(context.Background(), path, func(context.Context, agentprobe.Request) (agentprobe.Report, error) {
			if err := os.WriteFile(path, []byte("changed while diagnostic was running"), 0700); err != nil {
				t.Fatal(err)
			}
			return report, nil
		})
		if err == nil || got.Status != "indeterminate" || got.ReasonCode != "executable-changed-during-preflight" {
			t.Fatalf("binary mutation during diagnostic was accepted: report=%#v err=%v", got, err)
		}
	})
}

func TestNativeCodexPreflightDoesNotTrustIncompleteSupportedReport(t *testing.T) {
	path, digest := readinessExecutableFixture(t)
	report := supportedReadinessFixture(path, digest)
	report.BrokerStatus = "running"
	got, err := checkNativeCodexPreflight(context.Background(), path, func(context.Context, agentprobe.Request) (agentprobe.Report, error) { return report, nil })
	if err == nil || got.Status != "indeterminate" || got.ReasonCode != "diagnostic-evidence-incomplete" {
		t.Fatalf("incomplete supported report passed the gate: report=%#v err=%v", got, err)
	}
}

func TestFallbackNativeReadinessHasClosedUncertaintyShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unavailable-codex")
	report := fallbackReadiness(path, "executable-unavailable", "selected Codex executable is not a readable executable file")
	if report.Status != "indeterminate" || report.ProcessStarted || report.ExitCode != nil || report.ExecutableSHA256 != "" || report.PolicySHA256 != "" {
		t.Fatalf("fallback fabricated process or executable evidence: %#v", report)
	}
	if report.Broker.StartedAt.IsZero() || report.Broker.ClosedAt != nil || report.Broker.RequestsReceived != 0 || report.Broker.RequestsForwarded != 0 || report.Broker.InFlight != 0 {
		t.Fatalf("fallback broker observation is not an unavailable zero-stat snapshot: %#v", report.Broker)
	}
	if len(report.Limitations) < 5 {
		t.Fatalf("fallback readiness lacks required scope limitations: %#v", report.Limitations)
	}
	if _, err := json.Marshal(report); err != nil {
		t.Fatalf("fallback readiness cannot be serialized: %v", err)
	}
}

func readinessExecutableFixture(t *testing.T) (string, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "codex")
	contents := []byte("Codex test executable fixture")
	if err := os.WriteFile(path, contents, 0700); err != nil {
		t.Fatal(err)
	}
	canonical, digest, err := roleexec.ToolIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	return canonical, digest
}

func supportedReadinessFixture(path, digest string) agentprobe.Report {
	started := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	closed := started.Add(time.Second)
	completed := started.Add(500 * time.Millisecond)
	zero := 0
	return agentprobe.Report{
		Schema: agentprobe.ReportSchema, Scope: agentprobe.Scope, Status: "supported",
		CheckedAt: closed.Format(time.RFC3339Nano), StartedAt: started.Format(time.RFC3339Nano), FinishedAt: closed.Format(time.RFC3339Nano),
		ExecutablePath: path, ExecutableSHA256: digest, PolicySHA256: strings.Repeat("a", 64),
		Backend: "macos-seatbelt", Enforcement: "host-enforced", Assurance: "unverified", NetworkMode: "allowlist", Model: "sentinel-offline-readiness",
		Synthetic: true, ProcessStarted: true, ExitCode: &zero, MockRoundTrips: 1, ReplyObserved: true, CaptureComplete: true,
		BrokerStatus: "closed", Broker: codexbroker.Stats{
			StartedAt: started, ClosedAt: &closed, RequestsReceived: 1, RequestsForwarded: 1, LastStatus: 200, LastCompletedAt: &completed,
		},
	}
}
