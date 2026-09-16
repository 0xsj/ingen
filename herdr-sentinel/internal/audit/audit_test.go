package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"ingen/core/ciresult"
	sentinelrun "ingen/herdr-sentinel/internal/run"
)

func TestBuildPassesTerminalReceiptWithVerifiedReferences(t *testing.T) {
	t.Chdir(t.TempDir())
	writeAuditFile(t, "workspace.yaml", "workspace")
	writeAuditFile(t, "artifact.json", "artifact")
	receipt := auditReceipt(t, "completed")
	if err := receipt.AddFileArtifact("result", "verifier", "sorna-run", "artifact.json"); err != nil {
		t.Fatal(err)
	}
	if err := receipt.SetStatus("completed", receiptTime("2026-01-02T03:04:07Z")); err != nil {
		t.Fatal(err)
	}
	if err := sentinelrun.SaveFile("receipt.json", receipt); err != nil {
		t.Fatal(err)
	}

	report, err := Build("receipt.json", ".")
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "passed" || report.ExitCode() != 0 {
		t.Fatalf("report = %+v, want passing audit", report)
	}
	if len(report.Checks) != 4 {
		t.Fatalf("checks = %+v, want receipt, workspace, artifact, terminal checks", report.Checks)
	}
}

func TestBuildReportsReferenceMismatchAsFailure(t *testing.T) {
	t.Chdir(t.TempDir())
	writeAuditFile(t, "workspace.yaml", "workspace")
	writeAuditFile(t, "artifact.json", "changed")
	receipt := auditReceipt(t, "failed")
	receipt.Artifacts = []sentinelrun.ArtifactRef{{
		ID:   "result",
		Role: "verifier",
		Kind: "sorna-run",
		Ref:  ciresult.FileRef{Path: "artifact.json", SHA256: digest("original")},
	}}
	if err := sentinelrun.SaveFile("receipt.json", receipt); err != nil {
		t.Fatal(err)
	}

	report, err := Build("receipt.json", ".")
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "failed" || report.ExitCode() != 1 {
		t.Fatalf("report = %+v, want failed integrity audit", report)
	}
}

func TestBuildReportsNonTerminalReceiptAsError(t *testing.T) {
	t.Chdir(t.TempDir())
	writeAuditFile(t, "workspace.yaml", "workspace")
	receipt := auditReceipt(t, "running")
	if err := sentinelrun.SaveFile("receipt.json", receipt); err != nil {
		t.Fatal(err)
	}

	report, err := Build("receipt.json", ".")
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "error" || report.ExitCode() != 2 {
		t.Fatalf("report = %+v, want incomplete audit error", report)
	}
}

func auditReceipt(t *testing.T, status string) sentinelrun.Receipt {
	t.Helper()
	return sentinelrun.Receipt{
		Schema: sentinelrun.Schema,
		RunID:  "run-audit-test",
		Workspace: sentinelrun.WorkspaceRef{
			ID:      "audit-workspace",
			Version: 1,
			File:    ciresult.FileRef{Path: "workspace.yaml", SHA256: digest("workspace")},
		},
		Status:    status,
		CreatedAt: "2026-01-02T03:04:05Z",
		UpdatedAt: "2026-01-02T03:04:05Z",
		Events:    []sentinelrun.Event{{Sequence: 1, Type: "workspace-created", At: "2026-01-02T03:04:05Z"}},
	}
}

func writeAuditFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func digest(contents string) string {
	digest := sha256.Sum256([]byte(contents))
	return hex.EncodeToString(digest[:])
}

func receiptTime(value string) (parsedTime time.Time) {
	parsedTime, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		panic(err)
	}
	return parsedTime
}
