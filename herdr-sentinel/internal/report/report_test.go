package report

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"ingen/core/ciresult"
	sentinelrun "ingen/herdr-sentinel/internal/run"
)

func TestWriteSeparatesLifecycleAuditAndProducerVerdict(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("workspace.yaml", []byte("workspace"), 0o644); err != nil {
		t.Fatal(err)
	}
	receipt := sentinelrun.Receipt{
		Schema: sentinelrun.Schema,
		RunID:  "run-report-test",
		Workspace: sentinelrun.WorkspaceRef{
			ID:      "webhook-validation",
			Version: 1,
			File:    ciresult.FileRef{Path: "workspace.yaml", SHA256: digest("workspace")},
		},
		Status:    "completed",
		CreatedAt: "2026-09-16T10:00:00Z",
		UpdatedAt: "2026-09-16T10:00:01Z",
		Events: []sentinelrun.Event{
			{Sequence: 1, Type: "workspace-created", At: "2026-09-16T10:00:00Z"},
			{Sequence: 2, Type: "role-completed", At: "2026-09-16T10:00:01Z", Role: "verifier", Status: "completed", SourceID: "herdr-1"},
		},
	}
	if err := sentinelrun.SaveFile("receipt.json", receipt); err != nil {
		t.Fatal(err)
	}
	document, err := Build("receipt.json", ".")
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := Write(&output, document); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, want := range []string{
		"Lifecycle receipt: completed",
		"Audit integrity: passed",
		"role=verifier",
		"receipt_status=completed",
		"source=herdr-1",
		"Sorna verdict: producer-owned; not interpreted by Sentinel",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("report missing %q:\n%s", want, text)
		}
	}
}

func digest(contents string) string {
	digest := sha256.Sum256([]byte(contents))
	return hex.EncodeToString(digest[:])
}
