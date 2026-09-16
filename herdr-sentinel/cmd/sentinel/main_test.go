package main

import (
	"bytes"
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
	if code := run([]string{"adapter", "herdr-event", "--receipt", receiptPath, "--event", eventPath, "--output", receiptPath}); code != 0 {
		t.Fatalf("same-path adapter command exit code = %d, want lock-protected success", code)
	}
	inPlace, err := sentinelrun.LoadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(inPlace.Events) != 2 || inPlace.Status != "running" {
		t.Fatalf("same-path receipt = %+v, want one appended event", inPlace)
	}

	beforeReplay, err := os.ReadFile(updatedPath)
	if err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"adapter", "herdr-event", "--receipt", updatedPath, "--event", eventPath}); code != 0 {
		t.Fatalf("replay adapter command exit code = %d, want 0", code)
	}
	afterReplay, err := os.ReadFile(updatedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterReplay, beforeReplay) {
		t.Fatalf("receipt bytes changed after idempotent replay: before=%q after=%q", beforeReplay, afterReplay)
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

func TestArtifactCommandUsesSuppliedRoot(t *testing.T) {
	root := t.TempDir()
	caller := t.TempDir()
	t.Chdir(caller)
	receiptPath := filepath.Join(caller, "receipt.json")
	outputPath := filepath.Join(caller, "updated-receipt.json")
	if err := os.WriteFile(filepath.Join(root, "result.json"), []byte("root result"), 0o644); err != nil {
		t.Fatal(err)
	}
	receipt := sentinelrun.Receipt{
		Schema: sentinelrun.Schema,
		RunID:  "run-artifact-root-cli-test",
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
	if err := sentinelrun.SaveFile(receiptPath, receipt); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"run", "artifact", "--receipt", receiptPath, "--root", root, "--id", "result", "--role", "verifier", "--kind", "sorna-run", "--path", "result.json", "--output", outputPath}); code != 0 {
		t.Fatalf("rooted artifact command exit code = %d, want 0", code)
	}
	loaded, err := sentinelrun.LoadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Artifacts) != 1 || loaded.Artifacts[0].Ref.Path != "result.json" || loaded.Artifacts[0].Ref.SHA256 != digestBytes([]byte("root result")) {
		t.Fatalf("artifacts = %+v, want root-relative hash", loaded.Artifacts)
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

func TestArtifactCommandRejectsSymlinkEscapeWithoutPublishing(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "workspace.yaml"), []byte("workspace"), 0o644); err != nil {
		t.Fatal(err)
	}
	outsidePath := filepath.Join(outside, "result.json")
	if err := os.WriteFile(outsidePath, []byte("result"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsidePath, filepath.Join(root, "result.json")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	receiptPath := filepath.Join(".artifacts", "receipt.json")
	receipt := sentinelrun.Receipt{
		Schema: sentinelrun.Schema,
		RunID:  "run-artifact-symlink-cli-test",
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
	before, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"run", "artifact", "--receipt", receiptPath, "--id", "result", "--role", "verifier", "--kind", "sorna-run", "--path", "result.json"}); code != 1 {
		t.Fatalf("artifact symlink exit code = %d, want rejection", code)
	}
	after, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("receipt bytes changed after artifact symlink rejection: before=%q after=%q", before, after)
	}
	loaded, err := sentinelrun.LoadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Artifacts) != 0 {
		t.Fatalf("artifacts after symlink rejection = %+v, want none", loaded.Artifacts)
	}
}

func TestReceiptArtifactPathIsRelativeToSuppliedRoot(t *testing.T) {
	caller := t.TempDir()
	root := t.TempDir()
	t.Chdir(caller)
	path, ok := receiptArtifactPath(root, ".artifacts/verifier", "run.json")
	if !ok || path != filepath.Join(".artifacts", "verifier", "run.json") {
		t.Fatalf("receiptArtifactPath() = %q, %v; want root-relative reference", path, ok)
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

func TestCIResultCommandPublishesFailedLifecycleAsFailedEnvelope(t *testing.T) {
	t.Chdir(t.TempDir())
	receiptPath := filepath.Join(".artifacts", "receipt.json")
	outputPath := filepath.Join(".artifacts", "ci-result.json")
	if err := os.MkdirAll(filepath.Dir(receiptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	workspaceBytes := []byte("workspace")
	if err := os.WriteFile("workspace.yaml", workspaceBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	receipt := sentinelrun.Receipt{
		Schema: sentinelrun.Schema,
		RunID:  "run-ci-failed-test",
		Workspace: sentinelrun.WorkspaceRef{
			ID:      "webhook-validation",
			Version: 1,
			File:    ciresult.FileRef{Path: "workspace.yaml", SHA256: digestBytes(workspaceBytes)},
		},
		Status:    "failed",
		CreatedAt: "2026-09-16T10:00:00Z",
		UpdatedAt: "2026-09-16T10:00:01Z",
		Events:    []sentinelrun.Event{{Sequence: 1, Type: "workspace-created", At: "2026-09-16T10:00:00Z"}},
	}
	if err := sentinelrun.SaveFile(receiptPath, receipt); err != nil {
		t.Fatal(err)
	}

	if code := run([]string{"run", "ci-result", "--receipt", receiptPath, "--source-root", ".", "--output", outputPath}); code != 1 {
		t.Fatalf("failed ci-result exit code = %d, want producer failure code 1", code)
	}
	artifact, err := ciresult.LoadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Tool != "sentinel" || artifact.Status != "failed" || artifact.ExitCode != 1 || artifact.Error != "" {
		t.Fatalf("failed Sentinel envelope = %+v, want failed producer envelope", artifact)
	}
	var explanation struct {
		ReceiptStatus string `json:"receipt_status"`
		Outcome       string `json:"outcome"`
	}
	if err := json.Unmarshal(artifact.Explanation, &explanation); err != nil {
		t.Fatal(err)
	}
	if explanation.ReceiptStatus != "failed" || explanation.Outcome == "" {
		t.Fatalf("explanation = %+v, want failed receipt context", explanation)
	}
}

func TestCIResultCommandMapsBlockedLifecycleToErrorEnvelope(t *testing.T) {
	t.Chdir(t.TempDir())
	receiptPath := filepath.Join(".artifacts", "receipt.json")
	outputPath := filepath.Join(".artifacts", "ci-result.json")
	if err := os.MkdirAll(filepath.Dir(receiptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	workspaceBytes := []byte("workspace")
	if err := os.WriteFile("workspace.yaml", workspaceBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	receipt := sentinelrun.Receipt{
		Schema: sentinelrun.Schema,
		RunID:  "run-ci-blocked-test",
		Workspace: sentinelrun.WorkspaceRef{
			ID:      "webhook-validation",
			Version: 1,
			File:    ciresult.FileRef{Path: "workspace.yaml", SHA256: digestBytes(workspaceBytes)},
		},
		Status:    "blocked",
		CreatedAt: "2026-09-16T10:00:00Z",
		UpdatedAt: "2026-09-16T10:00:01Z",
		Events:    []sentinelrun.Event{{Sequence: 1, Type: "workspace-created", At: "2026-09-16T10:00:00Z"}},
	}
	if err := sentinelrun.SaveFile(receiptPath, receipt); err != nil {
		t.Fatal(err)
	}

	if code := run([]string{"run", "ci-result", "--receipt", receiptPath, "--source-root", ".", "--output", outputPath}); code != 2 {
		t.Fatalf("blocked ci-result exit code = %d, want orchestration error code 2", code)
	}
	artifact, err := ciresult.LoadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Tool != "sentinel" || artifact.Status != "error" || artifact.ExitCode != 2 || !strings.Contains(artifact.Error, "blocked") {
		t.Fatalf("blocked Sentinel envelope = %+v, want error envelope with lifecycle reason", artifact)
	}
	var explanation struct {
		ReceiptStatus string `json:"receipt_status"`
		Outcome       string `json:"outcome"`
	}
	if err := json.Unmarshal(artifact.Explanation, &explanation); err != nil {
		t.Fatal(err)
	}
	if explanation.ReceiptStatus != "blocked" || !strings.Contains(explanation.Outcome, "not a terminal verifier outcome") {
		t.Fatalf("explanation = %+v, want blocked lifecycle context", explanation)
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
	if code := run([]string{"adapter", "herdr-events", "--receipt", receiptPath, "--events", eventsPath, "--output", receiptPath}); code != 0 {
		t.Fatalf("same-path batch command exit code = %d, want lock-protected success", code)
	}
	inPlace, err := sentinelrun.LoadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(inPlace.Events) != 3 || inPlace.Status != "completed" {
		t.Fatalf("same-path batch receipt = %+v, want two appended events", inPlace)
	}
}

func TestHerdrEventBatchCommandRejectsConflictWithoutPublishing(t *testing.T) {
	t.Chdir(t.TempDir())
	receiptPath := filepath.Join(".artifacts", "receipt.json")
	eventsPath := filepath.Join(".artifacts", "events.jsonl")
	receipt := sentinelrun.Receipt{
		Schema: sentinelrun.Schema,
		RunID:  "run-herdr-batch-conflict-test",
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
	before, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	events := []map[string]any{
		{
			"schema":            "ingen.herdr-event/v1",
			"event_id":          "herdr-cli-conflict",
			"run_id":            receipt.RunID,
			"workspace_id":      receipt.Workspace.ID,
			"workspace_version": receipt.Workspace.Version,
			"type":              "role-launched",
			"at":                "2026-01-02T03:04:06Z",
		},
		{
			"schema":            "ingen.herdr-event/v1",
			"event_id":          "herdr-cli-conflict",
			"run_id":            receipt.RunID,
			"workspace_id":      receipt.Workspace.ID,
			"workspace_version": receipt.Workspace.Version,
			"type":              "role-completed",
			"at":                "2026-01-02T03:04:07Z",
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
	if code := run([]string{"adapter", "herdr-events", "--receipt", receiptPath, "--events", eventsPath, "--output", receiptPath}); code != 1 {
		t.Fatalf("conflicting batch exit code = %d, want rejection", code)
	}
	after, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("receipt bytes changed after conflicting batch: before=%q after=%q", before, after)
	}
	loaded, err := sentinelrun.LoadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Events) != 1 || loaded.Status != "created" {
		t.Fatalf("receipt after conflicting batch = %+v, want unchanged receipt", loaded)
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
