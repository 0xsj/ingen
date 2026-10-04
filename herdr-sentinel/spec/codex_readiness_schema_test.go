package spec

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"ingen/herdr-sentinel/internal/agentprobe"
	"ingen/herdr-sentinel/internal/codexbroker"
)

func TestCodexReadinessSchemaPreservesUncertainty(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report, err := agentprobe.Diagnose(ctx, agentprobe.Request{ExecutablePath: executable})
	if err != nil {
		t.Fatal(err)
	}
	schema := compileNativeBoundarySchema(t, "codex-readiness-v1.schema.json")
	document := boundaryDocument(t, report)
	if err := schema.Validate(document); err != nil {
		t.Fatalf("produced canceled readiness report rejected: %v", err)
	}
	fallback := boundaryDocument(t, agentprobe.Report{
		Schema: agentprobe.ReportSchema, Scope: agentprobe.Scope, Status: "indeterminate",
		CheckedAt: report.CheckedAt, StartedAt: report.StartedAt, FinishedAt: report.FinishedAt,
		ExecutablePath: executable, Backend: "unavailable", Enforcement: "unavailable", Assurance: "unverified",
		NetworkMode: "allowlist", Model: "sentinel-offline-readiness", Synthetic: true,
		BrokerStatus: "unavailable", Broker: codexbroker.Stats{StartedAt: time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)},
		ReasonCode: "executable-unavailable", Reason: "selected Codex executable is not a readable executable file",
		Limitations: []string{
			"supported means only that the selected CLI completed one local synthetic Responses round trip under the recorded Sorna backend",
			"the mock transport makes no provider request and provides no evidence of real inference, provider behavior, retention, or credential acceptance",
			"any executable digest in this report is a prelaunch byte check, not an observed running-image identity or host attestation",
			"macOS Seatbelt localhost network permission includes local host addresses at the pinned TCP port; it is not an IPv4-only grant",
			"this probe uses private temporary state and does not mutate a Sentinel, native Herdr, or project workspace",
		},
	})
	delete(fallback, "executable_sha256")
	delete(fallback, "policy_sha256")
	delete(fallback, "exit_code")
	fallback["process_started"] = false
	if err := schema.Validate(fallback); err != nil {
		t.Fatalf("fallback readiness without executable/process evidence rejected: %v", err)
	}
	document["status"] = "supported"
	if err := schema.Validate(document); err == nil {
		t.Fatal("schema accepted support without a completed process and mock round trip")
	}
	document["status"] = "indeterminate"
	document["credential"] = "undeclared"
	if err := schema.Validate(document); err == nil {
		t.Fatal("schema accepted an undeclared credential field")
	}
}

func TestPublishedCodexReadinessReport(t *testing.T) {
	path := os.Getenv("INGEN_CODEX_READINESS_REPORT")
	if path == "" {
		t.Skip("set INGEN_CODEX_READINESS_REPORT to validate an observed runtime diagnostic")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(contents, &document); err != nil {
		t.Fatal(err)
	}
	if err := compileNativeBoundarySchema(t, "codex-readiness-v1.schema.json").Validate(document); err != nil {
		t.Fatal(err)
	}
}
