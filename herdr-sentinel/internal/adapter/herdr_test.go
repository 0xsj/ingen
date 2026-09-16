package adapter

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"ingen/core/ciresult"
	sentinelrun "ingen/herdr-sentinel/internal/run"
)

func TestApplyHerdrEventBindsContextAndIsIdempotent(t *testing.T) {
	receipt := sentinelReceipt(t)
	event := HerdrEvent{
		Schema:           HerdrEventSchema,
		EventID:          "herdr-event-1",
		RunID:            receipt.RunID,
		WorkspaceID:      receipt.Workspace.ID,
		WorkspaceVersion: receipt.Workspace.Version,
		Type:             "role-launched",
		At:               "2026-01-02T03:04:06Z",
		Role:             "backend-implementer",
		Workspace:        ".sentinel/backend",
		SessionID:        "session-1",
		ReceiptStatus:    "running",
		Outcome:          "started",
	}

	appended, err := ApplyHerdrEvent(&receipt, event)
	if err != nil || !appended {
		t.Fatalf("ApplyHerdrEvent() = %v, %v; want append", appended, err)
	}
	if got := receipt.Events[1]; got.SourceID != event.EventID || got.SessionID != event.SessionID || got.Role != event.Role || got.Status != event.ReceiptStatus {
		t.Fatalf("event = %+v, want Herdr identity and context", got)
	}
	if receipt.Status != "running" {
		t.Fatalf("receipt status = %q, want running", receipt.Status)
	}

	appended, err = ApplyHerdrEvent(&receipt, event)
	if err != nil || appended {
		t.Fatalf("replay ApplyHerdrEvent() = %v, %v; want idempotent no-op", appended, err)
	}
	if len(receipt.Events) != 2 {
		t.Fatalf("events = %d, want one appended event after replay", len(receipt.Events))
	}
	if receipt.Status != "running" {
		t.Fatalf("replayed receipt status = %q, want unchanged running", receipt.Status)
	}
}

func TestApplyHerdrEventRejectsMismatchedRunAndConflictingReplay(t *testing.T) {
	receipt := sentinelReceipt(t)
	event := HerdrEvent{
		Schema:           HerdrEventSchema,
		EventID:          "herdr-event-1",
		RunID:            "other-run",
		WorkspaceID:      receipt.Workspace.ID,
		WorkspaceVersion: receipt.Workspace.Version,
		Type:             "role-launched",
		At:               "2026-01-02T03:04:06Z",
	}
	if _, err := ApplyHerdrEvent(&receipt, event); err == nil || !strings.Contains(err.Error(), "does not match receipt") {
		t.Fatalf("run mismatch = %v, want rejection", err)
	}

	event.RunID = receipt.RunID
	if _, err := ApplyHerdrEvent(&receipt, event); err != nil {
		t.Fatal(err)
	}
	event.Outcome = "changed"
	if _, err := ApplyHerdrEvent(&receipt, event); err == nil || !strings.Contains(err.Error(), "different content") {
		t.Fatalf("conflicting replay = %v, want rejection", err)
	}
}

func TestApplyHerdrEventRejectsStatusWithoutMutatingReceipt(t *testing.T) {
	receipt := sentinelReceipt(t)
	event := HerdrEvent{
		Schema:           HerdrEventSchema,
		EventID:          "herdr-event-1",
		RunID:            receipt.RunID,
		WorkspaceID:      receipt.Workspace.ID,
		WorkspaceVersion: receipt.Workspace.Version,
		Type:             "role-launched",
		At:               "2026-01-02T03:04:06Z",
		ReceiptStatus:    "not-a-status",
	}
	if _, err := ApplyHerdrEvent(&receipt, event); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("invalid status = %v, want rejection", err)
	}
	if len(receipt.Events) != 1 || receipt.Status != "created" {
		t.Fatalf("receipt mutated after invalid status: %+v", receipt)
	}
}

func TestApplyHerdrEventRejectsUnknownArtifact(t *testing.T) {
	receipt := sentinelReceipt(t)
	event := HerdrEvent{
		Schema:           HerdrEventSchema,
		EventID:          "herdr-event-1",
		RunID:            receipt.RunID,
		WorkspaceID:      receipt.Workspace.ID,
		WorkspaceVersion: receipt.Workspace.Version,
		Type:             "artifact-produced",
		At:               "2026-01-02T03:04:06Z",
		ArtifactIDs:      []string{"missing"},
	}
	if _, err := ApplyHerdrEvent(&receipt, event); err == nil || !strings.Contains(err.Error(), "unknown artifact") {
		t.Fatalf("unknown artifact = %v, want rejection", err)
	}
	if len(receipt.Events) != 1 {
		t.Fatalf("events = %d, want unchanged receipt", len(receipt.Events))
	}
}

func TestApplyHerdrEventRejectsOutOfOrderTimestamp(t *testing.T) {
	receipt := sentinelReceipt(t)
	event := HerdrEvent{
		Schema:           HerdrEventSchema,
		EventID:          "herdr-event-1",
		RunID:            receipt.RunID,
		WorkspaceID:      receipt.Workspace.ID,
		WorkspaceVersion: receipt.Workspace.Version,
		Type:             "role-launched",
		At:               "2026-01-02T03:04:04Z",
	}
	if _, err := ApplyHerdrEvent(&receipt, event); err == nil || !strings.Contains(err.Error(), "before receipt update") {
		t.Fatalf("out-of-order event = %v, want rejection", err)
	}
	if len(receipt.Events) != 1 {
		t.Fatalf("events = %d, want unchanged receipt", len(receipt.Events))
	}
}

func TestApplyHerdrEventWithRootRejectsArtifactDrift(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("artifact.json", []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	receipt := sentinelReceipt(t)
	if err := receipt.AddFileArtifact("result", "verifier", "sorna-run", "artifact.json"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("artifact.json", []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	event := HerdrEvent{
		Schema:           HerdrEventSchema,
		EventID:          "herdr-event-1",
		RunID:            receipt.RunID,
		WorkspaceID:      receipt.Workspace.ID,
		WorkspaceVersion: receipt.Workspace.Version,
		Type:             "artifact-produced",
		At:               "2026-01-02T03:04:06Z",
		ArtifactIDs:      []string{"result"},
	}
	if _, err := ApplyHerdrEventWithRoot(&receipt, event, "."); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("artifact drift = %v, want rejection", err)
	}
	if len(receipt.Events) != 1 {
		t.Fatalf("events = %d, want unchanged receipt", len(receipt.Events))
	}
}

func TestApplyHerdrEventsWithRootIsAtomic(t *testing.T) {
	receipt := sentinelReceipt(t)
	events := []HerdrEvent{
		{
			Schema:           HerdrEventSchema,
			EventID:          "herdr-event-1",
			RunID:            receipt.RunID,
			WorkspaceID:      receipt.Workspace.ID,
			WorkspaceVersion: receipt.Workspace.Version,
			Type:             "role-launched",
			At:               "2026-01-02T03:04:06Z",
			ReceiptStatus:    "running",
		},
		{
			Schema:           HerdrEventSchema,
			EventID:          "herdr-event-2",
			RunID:            receipt.RunID,
			WorkspaceID:      receipt.Workspace.ID,
			WorkspaceVersion: receipt.Workspace.Version,
			Type:             "unknown-event",
			At:               "2026-01-02T03:04:07Z",
		},
	}
	if _, err := ApplyHerdrEventsWithRoot(&receipt, events, "."); err == nil || !strings.Contains(err.Error(), "event 2") {
		t.Fatalf("batch error = %v, want indexed rejection", err)
	}
	if len(receipt.Events) != 1 || receipt.Status != "created" {
		t.Fatalf("receipt mutated after failed batch: %+v", receipt)
	}
}

func TestLoadHerdrEventStreamRejectsMalformedLine(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("events.jsonl", []byte("{\"schema\":\"ingen.herdr-event/v1\"}\nnot-json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadHerdrEventStream("events.jsonl"); err == nil || !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("LoadHerdrEventStream() = %v, want line-indexed validation error", err)
	}
}

func TestLoadHerdrEventStreamLoadsNonEmptyLines(t *testing.T) {
	t.Chdir(t.TempDir())
	event := HerdrEvent{
		Schema:           HerdrEventSchema,
		EventID:          "herdr-stream-event",
		RunID:            "run-herdr-test",
		WorkspaceID:      "webhook-validation",
		WorkspaceVersion: 1,
		Type:             "role-launched",
		At:               "2026-01-02T03:04:06Z",
	}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("events.jsonl", append(append([]byte("\n"), data...), '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	events, err := LoadHerdrEventStream("events.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventID != event.EventID {
		t.Fatalf("events = %+v, want one parsed event", events)
	}
}

func sentinelReceipt(t *testing.T) sentinelrun.Receipt {
	t.Helper()
	timestamp := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC).Format(time.RFC3339Nano)
	receipt := sentinelrun.Receipt{
		Schema: sentinelrun.Schema,
		RunID:  "run-herdr-test",
		Workspace: sentinelrun.WorkspaceRef{
			ID:      "webhook-validation",
			Version: 1,
			File:    ciresult.FileRef{Path: "workspace.yaml", SHA256: strings.Repeat("a", 64)},
		},
		Status:    "created",
		CreatedAt: timestamp,
		UpdatedAt: timestamp,
		Events:    []sentinelrun.Event{{Sequence: 1, Type: "workspace-created", At: timestamp}},
	}
	if err := receipt.Validate(); err != nil {
		t.Fatal(err)
	}
	return receipt
}
