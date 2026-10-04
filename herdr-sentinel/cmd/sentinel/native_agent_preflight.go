package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"ingen/herdr-sentinel/internal/agentprobe"
	"ingen/herdr-sentinel/internal/codexbroker"
	"ingen/herdr-sentinel/internal/roleexec"
)

type codexDiagnostic func(context.Context, agentprobe.Request) (agentprobe.Report, error)

type nativePreflightRunner func(context.Context, string) (agentprobe.Report, error)

type nativeReadinessError struct {
	Status string
	Code   string
	Reason string
}

func (e *nativeReadinessError) Error() string {
	return fmt.Sprintf("Codex readiness preflight %s (%s): %s", e.Status, e.Code, e.Reason)
}

func runNativeCodexPreflight(ctx context.Context, executable string) (agentprobe.Report, error) {
	return checkNativeCodexPreflight(ctx, executable, agentprobe.Diagnose)
}

// checkNativeCodexPreflight binds the diagnostic to the selected executable
// bytes before and after the probe. Its diagnostic function is an internal
// test seam; production always supplies agentprobe.Diagnose.
func checkNativeCodexPreflight(ctx context.Context, executable string, diagnose codexDiagnostic) (agentprobe.Report, error) {
	if ctx == nil {
		return fallbackReadiness(executable, "diagnostic-context-invalid", "preflight context is unavailable"), &nativeReadinessError{Status: "indeterminate", Code: "diagnostic-context-invalid", Reason: "preflight context is unavailable"}
	}
	if diagnose == nil {
		return fallbackReadiness(executable, "diagnostic-unavailable", "synthetic diagnostic is unavailable"), &nativeReadinessError{Status: "indeterminate", Code: "diagnostic-unavailable", Reason: "synthetic diagnostic is unavailable"}
	}
	canonical, beforeSHA, err := roleexec.ToolIdentity(executable)
	if err != nil {
		return fallbackReadiness(executable, "executable-unavailable", "selected Codex executable is not a readable executable file"), &nativeReadinessError{Status: "indeterminate", Code: "executable-unavailable", Reason: "selected Codex executable is not a readable executable file"}
	}
	report, err := diagnose(ctx, agentprobe.Request{ExecutablePath: canonical})
	if err != nil {
		return fallbackReadiness(canonical, "diagnostic-failed", "local synthetic Codex diagnostic could not complete"), &nativeReadinessError{Status: "indeterminate", Code: "diagnostic-failed", Reason: "local synthetic Codex diagnostic could not complete"}
	}
	if report.Schema != agentprobe.ReportSchema || report.Scope != agentprobe.Scope {
		return invalidateReadiness(report, "diagnostic-identity-invalid", "diagnostic report schema or scope did not match the local synthetic check")
	}
	if report.ExecutablePath != canonical || report.ExecutableSHA256 != beforeSHA {
		return invalidateReadiness(report, "executable-identity-mismatch", "diagnostic report did not match the selected executable bytes")
	}
	afterPath, afterSHA, err := roleexec.ToolIdentity(canonical)
	if err != nil || afterPath != canonical || afterSHA != beforeSHA {
		return invalidateReadiness(report, "executable-changed-during-preflight", "selected Codex executable changed during readiness preflight")
	}
	if report.Status != "supported" {
		if report.Status != "unsupported" && report.Status != "indeterminate" {
			return invalidateReadiness(report, "diagnostic-status-invalid", "diagnostic returned an unknown readiness status")
		}
		return report, &nativeReadinessError{Status: report.Status, Code: safeReasonCode(report.ReasonCode), Reason: safeReadinessReason(report.Reason)}
	}
	if !completeSupportedReadiness(report) {
		return invalidateReadiness(report, "diagnostic-evidence-incomplete", "synthetic readiness evidence did not satisfy the required local checks")
	}
	return report, nil
}

func completeSupportedReadiness(report agentprobe.Report) bool {
	return report.Status == "supported" && report.Synthetic && report.ProcessStarted && report.ExitCode != nil && *report.ExitCode == 0 &&
		report.Backend == "macos-seatbelt" && report.Enforcement == "host-enforced" && report.Assurance == "unverified" && report.NetworkMode == "allowlist" &&
		report.BrokerStatus == "closed" && !report.Broker.StartedAt.IsZero() && report.Broker.ClosedAt != nil && report.MockRoundTrips == 1 &&
		report.Broker.RequestsReceived == 1 && report.Broker.RequestsForwarded == 1 && report.Broker.LastStatus == 200 && report.Broker.LastCompletedAt != nil &&
		report.Broker.RequestsRejected == 0 && report.Broker.UpstreamFailures == 0 && report.Broker.InFlight == 0 && report.ReplyObserved && report.CaptureComplete && !report.OutputTruncated &&
		isLowerSHA256(report.ExecutableSHA256) && isLowerSHA256(report.PolicySHA256)
}

func invalidateReadiness(report agentprobe.Report, code, reason string) (agentprobe.Report, error) {
	report.Status = "indeterminate"
	report.ReasonCode = code
	report.Reason = reason
	return report, &nativeReadinessError{Status: "indeterminate", Code: code, Reason: reason}
}

func fallbackReadiness(executable, code, reason string) agentprobe.Report {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return agentprobe.Report{
		Schema: agentprobe.ReportSchema, Scope: agentprobe.Scope, Status: "indeterminate", CheckedAt: now, StartedAt: now, FinishedAt: now,
		ExecutablePath: executable, Backend: "unavailable", Enforcement: "unavailable", Assurance: "unverified", NetworkMode: "allowlist",
		Model: "sentinel-offline-readiness", Synthetic: true, BrokerStatus: "unavailable", ReasonCode: code, Reason: reason,
		Broker: codexbroker.Stats{StartedAt: time.Now().UTC()},
		Limitations: []string{
			"supported means only that the selected CLI completed one local synthetic Responses round trip under the recorded Sorna backend",
			"the mock transport makes no provider request and provides no evidence of real inference, provider behavior, retention, or credential acceptance",
			"any executable digest in this report is a prelaunch byte check, not an observed running-image identity or host attestation",
			"macOS Seatbelt localhost network permission includes local host addresses at the pinned TCP port; it is not an IPv4-only grant",
			"this probe uses private temporary state and does not mutate a Sentinel, native Herdr, or project workspace",
		},
	}
}

func safeReasonCode(value string) string {
	if value == "" {
		return "readiness-not-supported"
	}
	if len(value) > 96 {
		return "readiness-not-supported"
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return "readiness-not-supported"
		}
	}
	return value
}

func safeReadinessReason(value string) string {
	if strings.TrimSpace(value) == "" || len(value) > 256 || strings.ContainsAny(value, "\r\n\x00") {
		return "local synthetic Codex readiness check did not complete successfully"
	}
	return value
}

func isLowerSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

func emitNativeReadiness(report agentprobe.Report) {
	encoder := json.NewEncoder(os.Stderr)
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintln(os.Stderr, `{"status":"indeterminate","reason_code":"report-encode-failed"}`)
	}
}

func nativeReadinessExitCode(err error) int {
	var readinessErr *nativeReadinessError
	if errors.As(err, &readinessErr) && readinessErr.Status == "unsupported" {
		return 1
	}
	return 2
}
