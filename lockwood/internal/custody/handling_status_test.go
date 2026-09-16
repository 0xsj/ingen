package custody

import (
	"strings"
	"testing"
	"time"

	"ingen/lockwood/internal/store"
)

func TestAnalyzeHandlingProjectsEventsWithoutChangingRecord(t *testing.T) {
	records := NewMemory()
	artifacts := store.NewMemory()
	record := testRecord(t, "lockwood-handling-status-record")
	reference, err := artifacts.Put(strings.NewReader("handling status artifact"), store.PutOptions{MediaType: "text/plain"})
	if err != nil {
		t.Fatal(err)
	}
	record.Artifact = reference
	if err := records.Put(record); err != nil {
		t.Fatal(err)
	}

	retention := testHandlingEvent("event-status-retention", RetentionClassifiedEvent)
	retention.CustodyID = record.CustodyID
	retention.RecordedAt = time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	retention.RetentionClass = "regulated-7y"
	redaction := testHandlingEvent("event-status-redaction", RedactionEvent)
	redaction.CustodyID = record.CustodyID
	redaction.RecordedAt = retention.RecordedAt.Add(time.Minute)
	hold := testHandlingEvent("event-status-hold", LegalHoldPlacedEvent)
	hold.CustodyID = record.CustodyID
	hold.RecordedAt = retention.RecordedAt.Add(2 * time.Minute)
	for _, event := range []HandlingEvent{retention, redaction, hold} {
		if err := records.AppendEvent(event); err != nil {
			t.Fatal(err)
		}
	}

	status, err := AnalyzeHandling(records, records, record)
	if err != nil {
		t.Fatal(err)
	}
	if status.CustodyID != record.CustodyID || status.ArtifactDigest != record.Artifact.Digest || status.EventCount != 3 {
		t.Fatalf("handling status identity = %+v", status)
	}
	if status.RetentionClass != "regulated-7y" || status.Redaction != "redacted" || len(status.Redactions) != 1 {
		t.Fatalf("handling status classification = %+v", status)
	}
	if len(status.ActiveLegalHoldIDs) != 1 || status.ActiveLegalHoldIDs[0] != hold.LegalHoldID {
		t.Fatalf("handling status legal holds = %+v", status.ActiveLegalHoldIDs)
	}
	if status.LastEventAt == nil || !status.LastEventAt.Equal(hold.RecordedAt) {
		t.Fatalf("last event time = %v", status.LastEventAt)
	}
	stored, err := records.Get(record.CustodyID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Handling != record.Handling || stored.Artifact.Digest != record.Artifact.Digest {
		t.Fatalf("handling projection changed custody record: %+v", stored)
	}
}

func TestAnalyzeHandlingProjectsReleasedHoldsDeterministically(t *testing.T) {
	records := NewMemory()
	record := testRecord(t, "lockwood-handling-status-released")
	if err := records.Put(record); err != nil {
		t.Fatal(err)
	}
	placedA := testHandlingEvent("event-status-place-a", LegalHoldPlacedEvent)
	placedA.CustodyID = record.CustodyID
	placedA.LegalHoldID = "hold-z"
	placedA.RecordedAt = time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	placedB := placedA
	placedB.EventID = "event-status-place-b"
	placedB.LegalHoldID = "hold-a"
	placedB.RecordedAt = placedA.RecordedAt.Add(time.Minute)
	released := testHandlingEvent("event-status-release", LegalHoldReleasedEvent)
	released.CustodyID = record.CustodyID
	released.LegalHoldID = placedA.LegalHoldID
	released.RecordedAt = placedB.RecordedAt.Add(time.Minute)
	for _, event := range []HandlingEvent{placedA, placedB, released} {
		if err := records.AppendEvent(event); err != nil {
			t.Fatal(err)
		}
	}
	status, err := AnalyzeHandling(records, records, record)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.ActiveLegalHoldIDs) != 1 || status.ActiveLegalHoldIDs[0] != placedB.LegalHoldID {
		t.Fatalf("active holds = %+v", status.ActiveLegalHoldIDs)
	}
}

func TestAnalyzeHandlingRequiresStoresAndRecord(t *testing.T) {
	record := testRecord(t, "lockwood-handling-status-inputs")
	if _, err := AnalyzeHandling(nil, NewMemory(), record); err == nil || !strings.Contains(err.Error(), "custody record store") {
		t.Fatalf("missing record store error = %v", err)
	}
	if _, err := AnalyzeHandling(NewMemory(), nil, record); err == nil || !strings.Contains(err.Error(), "handling event store") {
		t.Fatalf("missing event store error = %v", err)
	}
	if _, err := AnalyzeHandling(NewMemory(), NewMemory(), Record{}); err == nil || !strings.Contains(err.Error(), "validate handling root") {
		t.Fatalf("invalid record error = %v", err)
	}
}
