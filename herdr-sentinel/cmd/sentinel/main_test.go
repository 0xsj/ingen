package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/core/ciresult"
	sentinelrun "ingen/herdr-sentinel/internal/run"
)

func TestHerdrEventAdapterCommandAppendsAndReplays(t *testing.T) {
	t.Chdir(t.TempDir())
	receiptPath := filepath.Join(".artifacts", "receipt.json")
	eventPath := filepath.Join(".artifacts", "event.json")
	updatedPath := filepath.Join(".artifacts", "updated-receipt.json")
	receipt := sentinelrun.Receipt{
		Schema: sentinelrun.Schema,
		RunID:  "run-herdr-cli-test",
		Workspace: sentinelrun.WorkspaceRef{
			ID:      "webhook-validation",
			Version: 1,
			File:    ciresult.FileRef{Path: "workspace.yaml", SHA256: strings.Repeat("a", 64)},
		},
		Status:    "created",
		CreatedAt: "2026-01-02T03:04:05Z",
		UpdatedAt: "2026-01-02T03:04:05Z",
		Events:    []sentinelrun.Event{{Sequence: 1, Type: "workspace-created", At: "2026-01-02T03:04:05Z"}},
	}
	if err := os.MkdirAll(filepath.Dir(receiptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := sentinelrun.SaveFile(receiptPath, receipt); err != nil {
		t.Fatal(err)
	}
	event := map[string]any{
		"schema":            "ingen.herdr-event/v1",
		"event_id":          "herdr-cli-event-1",
		"run_id":            receipt.RunID,
		"workspace_id":      receipt.Workspace.ID,
		"workspace_version": receipt.Workspace.Version,
		"type":              "role-launched",
		"at":                "2026-01-02T03:04:06Z",
		"role":              "backend-implementer",
		"workspace":         ".sentinel/backend",
		"session_id":        "session-cli-1",
		"receipt_status":    "running",
		"outcome":           "started",
	}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(eventPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	if code := run([]string{"adapter", "herdr-event", "--receipt", receiptPath, "--event", eventPath, "--output", updatedPath}); code != 0 {
		t.Fatalf("first adapter command exit code = %d, want 0", code)
	}
	loaded, err := sentinelrun.LoadFile(updatedPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Events) != 2 || loaded.Events[1].SourceID != "herdr-cli-event-1" || loaded.Status != "running" {
		t.Fatalf("loaded events = %+v, want appended source event", loaded.Events)
	}

	if code := run([]string{"adapter", "herdr-event", "--receipt", updatedPath, "--event", eventPath}); code != 0 {
		t.Fatalf("replay adapter command exit code = %d, want 0", code)
	}
	replayed, err := sentinelrun.LoadFile(updatedPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(replayed.Events) != 2 {
		t.Fatalf("replayed events = %d, want idempotent receipt", len(replayed.Events))
	}
}

func TestArtifactCommandRegistersHashedArtifact(t *testing.T) {
	t.Chdir(t.TempDir())
	receiptPath := filepath.Join(".artifacts", "receipt.json")
	outputPath := filepath.Join(".artifacts", "updated-receipt.json")
	receipt := sentinelrun.Receipt{
		Schema: sentinelrun.Schema,
		RunID:  "run-artifact-cli-test",
		Workspace: sentinelrun.WorkspaceRef{
			ID:      "webhook-validation",
			Version: 1,
			File:    ciresult.FileRef{Path: "workspace.yaml", SHA256: strings.Repeat("a", 64)},
		},
		Status:    "created",
		CreatedAt: "2026-01-02T03:04:05Z",
		UpdatedAt: "2026-01-02T03:04:05Z",
		Events:    []sentinelrun.Event{{Sequence: 1, Type: "workspace-created", At: "2026-01-02T03:04:05Z"}},
	}
	if err := os.MkdirAll(filepath.Dir(receiptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := sentinelrun.SaveFile(receiptPath, receipt); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("result.json", []byte("result"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"run", "artifact", "--receipt", receiptPath, "--id", "result", "--role", "verifier", "--kind", "sorna-run", "--path", "result.json", "--output", outputPath}); code != 0 {
		t.Fatalf("artifact command exit code = %d, want 0", code)
	}
	loaded, err := sentinelrun.LoadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Artifacts) != 1 || loaded.Artifacts[0].ID != "result" || loaded.Artifacts[0].Ref.SHA256 == "" {
		t.Fatalf("artifacts = %+v, want hashed registration", loaded.Artifacts)
	}
}

func TestArtifactCommandUpdatesReceiptInPlace(t *testing.T) {
	t.Chdir(t.TempDir())
	receiptPath := filepath.Join(".artifacts", "receipt.json")
	if err := os.MkdirAll(filepath.Dir(receiptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("workspace.yaml", []byte("workspace"), 0o644); err != nil {
		t.Fatal(err)
	}
	receipt := sentinelrun.Receipt{
		Schema: sentinelrun.Schema,
		RunID:  "run-artifact-in-place-test",
		Workspace: sentinelrun.WorkspaceRef{
			ID:      "webhook-validation",
			Version: 1,
			File:    ciresult.FileRef{Path: "workspace.yaml", SHA256: digestBytes([]byte("workspace"))},
		},
		Status:    "created",
		CreatedAt: "2026-09-16T03:04:05Z",
		UpdatedAt: "2026-09-16T03:04:05Z",
		Events:    []sentinelrun.Event{{Sequence: 1, Type: "workspace-created", At: "2026-09-16T03:04:05Z"}},
	}
	if err := sentinelrun.SaveFile(receiptPath, receipt); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("result.json", []byte("result"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"run", "artifact", "--receipt", receiptPath, "--id", "result", "--role", "verifier", "--kind", "sorna-run", "--path", "result.json"}); code != 0 {
		t.Fatalf("artifact command exit code = %d, want 0", code)
	}
	if code := run([]string{"run", "artifact", "--receipt", receiptPath, "--id", "result", "--role", "verifier", "--kind", "sorna-run", "--path", "result.json"}); code != 0 {
		t.Fatalf("artifact retry exit code = %d, want idempotent success", code)
	}
	if code := run([]string{"run", "artifact", "--receipt", receiptPath, "--output", "./" + receiptPath, "--id", "result", "--role", "verifier", "--kind", "sorna-run", "--path", "result.json"}); code != 0 {
		t.Fatalf("artifact same-path snapshot exit code = %d, want lock-protected success", code)
	}
	loaded, err := sentinelrun.LoadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Artifacts) != 1 || loaded.Artifacts[0].ID != "result" {
		t.Fatalf("artifacts = %+v, want in-place registration", loaded.Artifacts)
	}
}

func TestCIResultCommandRejectsCompletedReceiptWithDriftedReference(t *testing.T) {
	t.Chdir(t.TempDir())
	receiptPath := filepath.Join(".artifacts", "receipt.json")
	if err := os.MkdirAll(filepath.Dir(receiptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("workspace.yaml", []byte("original workspace"), 0o644); err != nil {
		t.Fatal(err)
	}
	receipt := sentinelrun.Receipt{
		Schema: sentinelrun.Schema,
		RunID:  "run-ci-drift-test",
		Workspace: sentinelrun.WorkspaceRef{
			ID:      "webhook-validation",
			Version: 1,
			File:    ciresult.FileRef{Path: "workspace.yaml", SHA256: strings.Repeat("a", 64)},
		},
		Status:    "completed",
		CreatedAt: "2026-01-02T03:04:05Z",
		UpdatedAt: "2026-01-02T03:04:05Z",
		Events:    []sentinelrun.Event{{Sequence: 1, Type: "workspace-created", At: "2026-01-02T03:04:05Z"}},
	}
	if err := sentinelrun.SaveFile(receiptPath, receipt); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"run", "ci-result", "--receipt", receiptPath, "--source-root", "."}); code != 1 {
		t.Fatalf("ci-result drift exit code = %d, want 1", code)
	}
}

func TestHerdrEventBatchCommandPublishesAllEvents(t *testing.T) {
	t.Chdir(t.TempDir())
	receiptPath := filepath.Join(".artifacts", "receipt.json")
	outputPath := filepath.Join(".artifacts", "updated-receipt.json")
	eventsPath := filepath.Join(".artifacts", "events.jsonl")
	receipt := sentinelrun.Receipt{
		Schema: sentinelrun.Schema,
		RunID:  "run-herdr-batch-test",
		Workspace: sentinelrun.WorkspaceRef{
			ID:      "webhook-validation",
			Version: 1,
			File:    ciresult.FileRef{Path: "workspace.yaml", SHA256: strings.Repeat("a", 64)},
		},
		Status:    "created",
		CreatedAt: "2026-01-02T03:04:05Z",
		UpdatedAt: "2026-01-02T03:04:05Z",
		Events:    []sentinelrun.Event{{Sequence: 1, Type: "workspace-created", At: "2026-01-02T03:04:05Z"}},
	}
	if err := os.MkdirAll(filepath.Dir(receiptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := sentinelrun.SaveFile(receiptPath, receipt); err != nil {
		t.Fatal(err)
	}
	events := []map[string]any{
		{
			"schema":            "ingen.herdr-event/v1",
			"event_id":          "herdr-batch-1",
			"run_id":            receipt.RunID,
			"workspace_id":      receipt.Workspace.ID,
			"workspace_version": receipt.Workspace.Version,
			"type":              "role-launched",
			"at":                "2026-01-02T03:04:06Z",
			"receipt_status":    "running",
		},
		{
			"schema":            "ingen.herdr-event/v1",
			"event_id":          "herdr-batch-2",
			"run_id":            receipt.RunID,
			"workspace_id":      receipt.Workspace.ID,
			"workspace_version": receipt.Workspace.Version,
			"type":              "role-completed",
			"at":                "2026-01-02T03:04:07Z",
			"receipt_status":    "completed",
		},
	}
	stream := make([]byte, 0)
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		stream = append(stream, data...)
		stream = append(stream, '\n')
	}
	if err := os.WriteFile(eventsPath, stream, 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"adapter", "herdr-events", "--receipt", receiptPath, "--events", eventsPath, "--output", outputPath}); code != 0 {
		t.Fatalf("batch command exit code = %d, want 0", code)
	}
	loaded, err := sentinelrun.LoadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Events) != 3 || loaded.Status != "completed" || loaded.Events[2].SourceID != "herdr-batch-2" {
		t.Fatalf("loaded receipt = %+v, want two appended events and completed status", loaded)
	}
}

func TestReportCommandWritesOperatorView(t *testing.T) {
	t.Chdir(t.TempDir())
	receiptPath := filepath.Join(".artifacts", "receipt.json")
	outputPath := filepath.Join(".artifacts", "report.txt")
	if err := os.MkdirAll(filepath.Dir(receiptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	workspaceBytes := []byte("workspace")
	if err := os.WriteFile("workspace.yaml", workspaceBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(workspaceBytes)
	receipt := sentinelrun.Receipt{
		Schema: sentinelrun.Schema,
		RunID:  "run-report-cli-test",
		Workspace: sentinelrun.WorkspaceRef{
			ID:      "webhook-validation",
			Version: 1,
			File:    ciresult.FileRef{Path: "workspace.yaml", SHA256: hex.EncodeToString(digest[:])},
		},
		Status:    "completed",
		CreatedAt: "2026-09-16T10:00:00Z",
		UpdatedAt: "2026-09-16T10:00:01Z",
		Events:    []sentinelrun.Event{{Sequence: 1, Type: "workspace-created", At: "2026-09-16T10:00:00Z"}},
	}
	if err := sentinelrun.SaveFile(receiptPath, receipt); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"run", "report", "--receipt", receiptPath, "--root", ".", "--output", outputPath}); code != 0 {
		t.Fatalf("report command exit code = %d, want 0", code)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"Lifecycle receipt: completed", "Audit integrity: passed", "Sorna verdict: producer-owned; not interpreted by Sentinel"} {
		if !strings.Contains(text, want) {
			t.Fatalf("report missing %q:\n%s", want, text)
		}
	}
}

func digestBytes(contents []byte) string {
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:])
}
