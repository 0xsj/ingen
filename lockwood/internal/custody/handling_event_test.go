package custody

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"ingen/lockwood/internal/artifact"
)

func testHandlingEvent(eventID string, eventType HandlingEventType) HandlingEvent {
	event := HandlingEvent{
		Schema:     HandlingEventSchema,
		EventID:    eventID,
		CustodyID:  "lockwood-handling-event-record",
		Type:       eventType,
		RecordedAt: time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC),
		Actor:      "operator@example",
		Reason:     "policy decision recorded",
	}
	switch eventType {
	case RedactionEvent:
		event.OriginalDigest = "sha256:" + strings.Repeat("a", 64)
		event.ResultingDigest = "sha256:" + strings.Repeat("b", 64)
	case RetentionClassifiedEvent:
		event.RetentionClass = "regulated-7y"
	case LegalHoldPlacedEvent, LegalHoldReleasedEvent:
		event.LegalHoldID = "hold-2026-0001"
	}
	return event
}

func TestHandlingEventCanonicalEncodingAndValidation(t *testing.T) {
	event := testHandlingEvent("event-redaction-0001", RedactionEvent)
	encoded, err := MarshalCanonicalHandlingEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.HasSuffix(encoded, []byte("\n")) {
		t.Fatal("canonical handling event has trailing newline")
	}
	decoded, err := UnmarshalCanonicalHandlingEvent(encoded)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := HandlingEventDigest(decoded)
	if err != nil || digest == "" {
		t.Fatalf("handling event digest = %q, err = %v", digest, err)
	}
	for _, eventType := range []HandlingEventType{RetentionClassifiedEvent, LegalHoldPlacedEvent, LegalHoldReleasedEvent} {
		if err := testHandlingEvent("event-"+string(eventType), eventType).Validate(); err != nil {
			t.Fatalf("valid %s event rejected: %v", eventType, err)
		}
	}
	if _, err := UnmarshalCanonicalHandlingEvent(append([]byte(" \n"), encoded...)); err == nil || !strings.Contains(err.Error(), "not canonical JSON") {
		t.Fatalf("noncanonical event error = %v", err)
	}
}

func TestHandlingEventRejectsInvalidTypeFields(t *testing.T) {
	valid := testHandlingEvent("event-invalid-0001", RedactionEvent)
	tests := []struct {
		name string
		edit func(*HandlingEvent)
		want string
	}{
		{name: "missing actor", edit: func(event *HandlingEvent) { event.Actor = "" }, want: "actor is required"},
		{name: "same redaction digest", edit: func(event *HandlingEvent) { event.ResultingDigest = event.OriginalDigest }, want: "digests must differ"},
		{name: "retention missing class", edit: func(event *HandlingEvent) {
			event.Type = RetentionClassifiedEvent
			event.OriginalDigest = ""
			event.ResultingDigest = ""
		}, want: "requires retention class"},
		{name: "hold missing id", edit: func(event *HandlingEvent) {
			event.Type = LegalHoldPlacedEvent
			event.OriginalDigest = ""
			event.ResultingDigest = ""
			event.LegalHoldID = ""
		}, want: "requires legal hold id"},
		{name: "unsupported type", edit: func(event *HandlingEvent) {
			event.Type = "unknown"
			event.OriginalDigest = ""
			event.ResultingDigest = ""
		}, want: "unsupported handling event type"},
		{name: "invalid digest", edit: func(event *HandlingEvent) { event.OriginalDigest = "not-a-digest" }, want: "invalid handling event original digest"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := valid
			test.edit(&event)
			if err := event.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestFilesystemHandlingEventStoreIsAppendOnlyAndDurable(t *testing.T) {
	root := t.TempDir()
	store, err := NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	first := testHandlingEvent("event-retention-0001", RetentionClassifiedEvent)
	second := testHandlingEvent("event-hold-0001", LegalHoldPlacedEvent)
	second.RecordedAt = second.RecordedAt.Add(time.Minute)
	for _, event := range []HandlingEvent{first, second} {
		if err := store.AppendEvent(event); err != nil {
			t.Fatal(err)
		}
		if err := store.AppendEvent(event); err != nil {
			t.Fatalf("identical append was not idempotent: %v", err)
		}
	}
	conflict := first
	conflict.Reason = "different reason"
	if err := store.AppendEvent(conflict); err == nil || !strings.Contains(err.Error(), "different contents") {
		t.Fatalf("conflicting append error = %v", err)
	}
	listed, err := store.ListEvents(first.CustodyID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0].EventID != first.EventID || listed[1].EventID != second.EventID {
		t.Fatalf("listed handling events = %+v", listed)
	}
	reopened, err := NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.GetEvent(first.CustodyID, first.EventID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Reason != first.Reason || got.Type != first.Type {
		t.Fatalf("reopened handling event = %+v", got)
	}
	path := filepath.Join(root, "events", first.CustodyID, first.EventID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("handling event file is empty")
	}
}

func TestMemoryHandlingEventStoreIsDeterministicAndConcurrent(t *testing.T) {
	store := NewMemory()
	event := testHandlingEvent("event-memory-0001", RetentionClassifiedEvent)
	const writers = 16
	errs := make(chan error, writers)
	var wait sync.WaitGroup
	wait.Add(writers)
	for range writers {
		go func() {
			defer wait.Done()
			errs <- store.AppendEvent(event)
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	listed, err := store.ListEvents(event.CustodyID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].EventID != event.EventID {
		t.Fatalf("concurrent handling events = %+v", listed)
	}

	second := testHandlingEvent("event-memory-0002", LegalHoldPlacedEvent)
	if err := store.AppendEvent(second); err != nil {
		t.Fatal(err)
	}
	listed, err = store.ListEvents(event.CustodyID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0].EventID != event.EventID || listed[1].EventID != second.EventID {
		t.Fatalf("deterministic handling events = %+v", listed)
	}
}

func TestHandlingEventStoreRejectsUnsafeIDs(t *testing.T) {
	store := NewMemory()
	event := testHandlingEvent("../escape", RetentionClassifiedEvent)
	if err := store.AppendEvent(event); err == nil || !strings.Contains(err.Error(), "invalid handling event id") {
		t.Fatalf("unsafe event ID error = %v", err)
	}
	if _, err := store.GetEvent(event.CustodyID, "../escape"); err == nil || !strings.Contains(err.Error(), "invalid handling event id") {
		t.Fatalf("unsafe GetEvent error = %v", err)
	}
}

func TestHandlingEventUsesArtifactDigestValidation(t *testing.T) {
	event := testHandlingEvent("event-redaction-0002", RedactionEvent)
	event.ResultingDigest = artifact.SHA256Algorithm + ":" + strings.Repeat("c", 64)
	if err := event.Validate(); err != nil {
		t.Fatal(err)
	}
}
