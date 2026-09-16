package run

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/core/ciresult"
)

func TestBuildCIResultFilePreservesReceiptAndInputLineage(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	path := "sentinel-receipt.json"
	receipt := testCIReceipt("completed")
	receipt.Artifacts = []ArtifactRef{{
		ID:   "sorna-run",
		Role: "verifier",
		Kind: "sorna-run",
		Ref:  testFileRef(".artifacts/run.json"),
	}}
	contents, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	contents = append(contents, '\n')
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}

	artifact, err := BuildCIResultFile(path, "workspace")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Tool != "sentinel" || artifact.Kind != "orchestration-receipt" || artifact.Status != "passed" || artifact.ExitCode != 0 {
		t.Fatalf("artifact = %+v, want passing Sentinel envelope", artifact)
	}
	if string(artifact.Report) != string(contents) {
		t.Fatalf("report changed receipt bytes, got %q want %q", artifact.Report, contents)
	}
	if artifact.Inputs["receipt"].SHA256 == "" || artifact.Inputs["workspace"] != receipt.Workspace.File || artifact.Inputs["artifact_sorna-run"] != receipt.Artifacts[0].Ref {
		t.Fatalf("inputs = %+v, want receipt, workspace, and artifact lineage", artifact.Inputs)
	}
	if err := artifact.Validate(); err != nil {
		t.Fatal(err)
	}

	var explanation map[string]any
	if err := json.Unmarshal(artifact.Explanation, &explanation); err != nil {
		t.Fatal(err)
	}
	if explanation["receipt_status"] != "completed" {
		t.Fatalf("explanation = %v, want completed receipt status", explanation)
	}
}

func TestBuildCIResultFileWithAuditIncludesIntegritySummary(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	path := "sentinel-receipt.json"
	receipt := testCIReceipt("completed")
	contents, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}

	artifact, err := BuildCIResultBytes(path, contents, ".", AuditSummary{
		Status: "passed",
		Checks: []AuditCheck{
			{ID: "receipt-structure", Status: "passed"},
			{ID: "lifecycle-terminal", Status: "passed"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var explanation struct {
		AuditStatus string       `json:"audit_status"`
		AuditChecks []AuditCheck `json:"audit_checks"`
	}
	if err := json.Unmarshal(artifact.Explanation, &explanation); err != nil {
		t.Fatal(err)
	}
	if explanation.AuditStatus != "passed" || len(explanation.AuditChecks) != 2 || explanation.AuditChecks[1].ID != "lifecycle-terminal" {
		t.Fatalf("explanation = %+v, want compact audit summary", explanation)
	}
}

func TestBuildCIResultFileMapsFailedReceipt(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	path := "sentinel-receipt.json"
	receipt := testCIReceipt("failed")
	contents, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}

	artifact, err := BuildCIResultFile(path, ".")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Status != "failed" || artifact.ExitCode != 1 || artifact.Error != "" {
		t.Fatalf("artifact = %+v, want failed lifecycle without envelope error", artifact)
	}
}

func TestBuildCIResultFileRejectsIncompleteReceipt(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	path := "sentinel-receipt.json"
	receipt := testCIReceipt("running")
	contents, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}

	artifact, err := BuildCIResultFile(path, ".")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Status != "error" || artifact.ExitCode != 2 || !strings.Contains(artifact.Error, "not complete") {
		t.Fatalf("artifact = %+v, want incomplete lifecycle error envelope", artifact)
	}
}

func TestBuildCIResultFileRejectsUnknownReceiptFields(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sentinel-receipt.json")
	contents, err := json.Marshal(testCIReceipt("completed"))
	if err != nil {
		t.Fatal(err)
	}
	contents = append(contents[:len(contents)-1], []byte(`,"future":true}`)...)
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildCIResultFile(path, "."); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("BuildCIResultFile() = %v, want unknown-field error", err)
	}
}

func testCIReceipt(status string) Receipt {
	return Receipt{
		Schema: Schema,
		RunID:  "sentinel-run-01",
		Workspace: WorkspaceRef{
			ID:      "webhook-validation",
			Version: 1,
			File:    testFileRef("workspace.yaml"),
		},
		Status:    status,
		CreatedAt: "2026-09-16T10:00:00Z",
		UpdatedAt: "2026-09-16T10:00:01Z",
		Events: []Event{{
			Sequence: 1,
			Type:     "workspace-created",
			At:       "2026-09-16T10:00:00Z",
		}},
	}
}

func testFileRef(path string) ciresult.FileRef {
	digest := sha256.Sum256([]byte(path))
	return ciresult.FileRef{Path: path, SHA256: hex.EncodeToString(digest[:])}
}
