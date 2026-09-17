package adapter

import (
	"encoding/json"
	"os"
	"path/filepath"
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

func TestApplyHerdrEventRejectsMismatchedRunWorkspaceAndConflictingReplay(t *testing.T) {
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
	event.WorkspaceID = "other-workspace"
	if _, err := ApplyHerdrEvent(&receipt, event); err == nil || !strings.Contains(err.Error(), "does not match receipt") {
		t.Fatalf("workspace mismatch = %v, want rejection", err)
	}
	if len(receipt.Events) != 1 || receipt.Status != "created" {
		t.Fatalf("receipt mutated after context rejection: %+v", receipt)
	}

	event.WorkspaceID = receipt.Workspace.ID
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

func TestApplyHerdrEventClosesFailedLifecycleAndRejectsReopen(t *testing.T) {
	receipt := sentinelReceipt(t)
	event := HerdrEvent{
		Schema:           HerdrEventSchema,
		EventID:          "herdr-event-failed",
		RunID:            receipt.RunID,
		WorkspaceID:      receipt.Workspace.ID,
		WorkspaceVersion: receipt.Workspace.Version,
		Type:             "sorna-completed",
		At:               "2026-01-02T03:04:06Z",
		Role:             "verifier",
		SessionID:        "session-failed",
		ReceiptStatus:    "failed",
		Outcome:          "contract-failed",
		Reason:           "one verifier rule failed",
	}
	if appended, err := ApplyHerdrEvent(&receipt, event); err != nil || !appended {
		t.Fatalf("failed lifecycle event = %v, %v; want accepted terminal failure", appended, err)
	}
	if receipt.Status != "failed" || len(receipt.Events) != 2 || receipt.Events[1].Status != "failed" {
		t.Fatalf("receipt after failed event = %+v, want failed terminal receipt", receipt)
	}
	if appended, err := ApplyHerdrEvent(&receipt, event); err != nil || appended {
		t.Fatalf("failed lifecycle replay = %v, %v; want idempotent no-op", appended, err)
	}
	if len(receipt.Events) != 2 || receipt.Status != "failed" {
		t.Fatalf("receipt after failed replay = %+v, want unchanged failed receipt", receipt)
	}
	reopen := event
	reopen.EventID = "herdr-event-reopen"
	reopen.Type = "role-launched"
	reopen.At = "2026-01-02T03:04:07Z"
	reopen.ReceiptStatus = "running"
	if _, err := ApplyHerdrEvent(&receipt, reopen); err == nil || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("failed lifecycle reopen = %v, want terminal transition rejection", err)
	}
	if len(receipt.Events) != 2 || receipt.Status != "failed" {
		t.Fatalf("receipt after failed lifecycle reopen = %+v, want unchanged failed receipt", receipt)
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

func TestApplyHerdrEventRejectsTerminalStatusRegression(t *testing.T) {
	receipt := sentinelReceipt(t)
	if err := receipt.SetStatus("completed", time.Date(2026, time.January, 2, 3, 4, 6, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	event := HerdrEvent{
		Schema:           HerdrEventSchema,
		EventID:          "herdr-event-regression",
		RunID:            receipt.RunID,
		WorkspaceID:      receipt.Workspace.ID,
		WorkspaceVersion: receipt.Workspace.Version,
		Type:             "role-launched",
		At:               "2026-01-02T03:04:07Z",
		ReceiptStatus:    "running",
	}
	if _, err := ApplyHerdrEvent(&receipt, event); err == nil || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("terminal regression = %v, want rejection", err)
	}
	if len(receipt.Events) != 1 || receipt.Status != "completed" {
		t.Fatalf("receipt mutated after terminal regression: %+v", receipt)
	}
}

func TestApplyHerdrEventAllowsCleanupAfterTerminalStatus(t *testing.T) {
	receipt := sentinelReceipt(t)
	if err := receipt.SetStatus("completed", time.Date(2026, time.January, 2, 3, 4, 6, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	event := HerdrEvent{
		Schema:           HerdrEventSchema,
		EventID:          "herdr-event-cleanup",
		RunID:            receipt.RunID,
		WorkspaceID:      receipt.Workspace.ID,
		WorkspaceVersion: receipt.Workspace.Version,
		Type:             "cleanup-completed",
		At:               "2026-01-02T03:04:07Z",
		ReceiptStatus:    "cleaned",
	}
	if appended, err := ApplyHerdrEvent(&receipt, event); err != nil || !appended {
		t.Fatalf("cleanup event = %v, %v; want accepted transition", appended, err)
	}
	if receipt.Status != "cleaned" || len(receipt.Events) != 2 {
		t.Fatalf("receipt after cleanup = %+v, want cleaned receipt", receipt)
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

func TestApplyHerdrEventWithRootRejectsMissingArtifact(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("artifact.json", []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	receipt := sentinelReceipt(t)
	if err := receipt.AddFileArtifact("result", "verifier", "sorna-run", "artifact.json"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove("artifact.json"); err != nil {
		t.Fatal(err)
	}
	event := HerdrEvent{
		Schema:           HerdrEventSchema,
		EventID:          "herdr-event-missing-artifact",
		RunID:            receipt.RunID,
		WorkspaceID:      receipt.Workspace.ID,
		WorkspaceVersion: receipt.Workspace.Version,
		Type:             "artifact-produced",
		At:               "2026-01-02T03:04:06Z",
		ArtifactIDs:      []string{"result"},
	}
	if _, err := ApplyHerdrEventWithRoot(&receipt, event, "."); err == nil || !strings.Contains(err.Error(), "read artifact") {
		t.Fatalf("missing artifact = %v, want rejection", err)
	}
	if len(receipt.Events) != 1 {
		t.Fatalf("events = %d, want unchanged receipt", len(receipt.Events))
	}
}

func TestApplyHerdrEventWithRootRejectsArtifactSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsidePath := filepath.Join(outside, "artifact.json")
	if err := os.WriteFile(outsidePath, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(root, "artifact.json")
	if err := os.Symlink(outsidePath, linkPath); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	receipt := sentinelReceipt(t)
	if err := receipt.AddArtifact(sentinelrun.ArtifactRef{
		ID:   "result",
		Role: "verifier",
		Kind: "sorna-run",
		Ref:  ciresult.FileRef{Path: "artifact.json", SHA256: strings.Repeat("a", 64)},
	}); err != nil {
		t.Fatal(err)
	}
	event := HerdrEvent{
		Schema:           HerdrEventSchema,
		EventID:          "herdr-event-symlink-escape",
		RunID:            receipt.RunID,
		WorkspaceID:      receipt.Workspace.ID,
		WorkspaceVersion: receipt.Workspace.Version,
		Type:             "artifact-produced",
		At:               "2026-01-02T03:04:06Z",
		ArtifactIDs:      []string{"result"},
	}
	if _, err := ApplyHerdrEventWithRoot(&receipt, event, "."); err == nil || !strings.Contains(err.Error(), "escapes root") {
		t.Fatalf("symlink escape = %v, want root-containment rejection", err)
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

func TestApplyHerdrEventsWithRootRejectsConflictingReplayAtomically(t *testing.T) {
	receipt := sentinelReceipt(t)
	events := []HerdrEvent{
		{
			Schema:           HerdrEventSchema,
			EventID:          "herdr-conflict-batch",
			RunID:            receipt.RunID,
			WorkspaceID:      receipt.Workspace.ID,
			WorkspaceVersion: receipt.Workspace.Version,
			Type:             "role-launched",
			At:               "2026-01-02T03:04:06Z",
		},
		{
			Schema:           HerdrEventSchema,
			EventID:          "herdr-conflict-batch",
			RunID:            receipt.RunID,
			WorkspaceID:      receipt.Workspace.ID,
			WorkspaceVersion: receipt.Workspace.Version,
			Type:             "role-completed",
			At:               "2026-01-02T03:04:07Z",
		},
	}
	if _, err := ApplyHerdrEventsWithRoot(&receipt, events, "."); err == nil || !strings.Contains(err.Error(), "event 2") || !strings.Contains(err.Error(), "different content") {
		t.Fatalf("conflicting batch = %v, want indexed conflict rejection", err)
	}
	if len(receipt.Events) != 1 || receipt.Status != "created" {
		t.Fatalf("receipt mutated after conflicting batch: %+v", receipt)
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

func TestLoadHerdrEventRejectsRawHostEnvelope(t *testing.T) {
	t.Chdir(t.TempDir())
	rawHostEvent := []byte(`{"event":"pane_agent_status_changed","data":{"type":"pane_agent_status_changed","workspace_id":"workspace-fixture","pane_id":"workspace-fixture:pane-fixture","agent_status":"blocked"}}`)
	if err := os.WriteFile("raw-event.json", rawHostEvent, 0o644); err != nil {
		t.Fatal(err)
	}
	envelope, err := LoadHerdrHostEnvelope("raw-event.json")
	if err != nil {
		t.Fatalf("LoadHerdrHostEnvelope() = %v, want raw envelope accepted for evidence", err)
	}
	if envelope.Event != "pane_agent_status_changed" || len(envelope.Data) == 0 || len(envelope.Raw) == 0 {
		t.Fatalf("host envelope = %+v, want event, data, and raw bytes preserved", envelope)
	}
	if _, err := LoadHerdrEvent("raw-event.json"); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("LoadHerdrEvent() = %v, want raw host envelope rejected before normalization", err)
	}
}

func TestLoadHerdrHostEnvelopeRejectsMissingOrNonObjectData(t *testing.T) {
	t.Chdir(t.TempDir())
	cases := map[string]string{
		"missing-data": `{"event":"pane_agent_status_changed"}`,
		"array-data":   `{"event":"pane_agent_status_changed","data":[]}`,
	}
	for name, contents := range cases {
		t.Run(name, func(t *testing.T) {
			path := name + ".json"
			if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadHerdrHostEnvelope(path); err == nil {
				t.Fatal("LoadHerdrHostEnvelope() succeeded, want rejection")
			}
		})
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
