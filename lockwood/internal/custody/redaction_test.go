package custody

import (
	"strings"
	"testing"
	"time"

	"ingen/lockwood/internal/store"
)

func TestRegisterRedactionVerifiesArtifactsAndPreservesOriginal(t *testing.T) {
	artifacts := store.NewMemory()
	records := NewMemory()
	original, err := artifacts.Put(strings.NewReader("original bytes"), store.PutOptions{MediaType: "text/plain", LogicalName: "original.txt"})
	if err != nil {
		t.Fatal(err)
	}
	resulting, err := artifacts.Put(strings.NewReader("redacted bytes"), store.PutOptions{MediaType: "text/plain", LogicalName: "redacted.txt"})
	if err != nil {
		t.Fatal(err)
	}
	record := testRecord(t, "lockwood-redaction-registration")
	record.Artifact = original
	record.Parents = []Lineage{}
	if err := records.Put(record); err != nil {
		t.Fatal(err)
	}
	event := HandlingEvent{
		Schema:          HandlingEventSchema,
		EventID:         "event-redaction-registration-0001",
		CustodyID:       record.CustodyID,
		Type:            RedactionEvent,
		RecordedAt:      time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC),
		Actor:           "operator@example",
		Reason:          "removed restricted fields",
		OriginalDigest:  original.Digest,
		ResultingDigest: resulting.Digest,
	}
	registered, err := RegisterRedaction(artifacts, records, records, event)
	if err != nil {
		t.Fatal(err)
	}
	if registered.EventID != event.EventID || registered.OriginalDigest != original.Digest || registered.ResultingDigest != resulting.Digest {
		t.Fatalf("registered redaction = %+v", registered)
	}
	if err := artifacts.Verify(original.Digest); err != nil {
		t.Fatalf("original artifact was not preserved: %v", err)
	}
	stored, err := records.Get(record.CustodyID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Artifact.Digest != original.Digest || stored.Handling != record.Handling {
		t.Fatalf("redaction registration changed custody record: %+v", stored)
	}
	if _, err := RegisterRedaction(artifacts, records, records, event); err != nil {
		t.Fatalf("identical redaction registration was not idempotent: %v", err)
	}

	conflict := event
	conflict.Reason = "different reason"
	if _, err := RegisterRedaction(artifacts, records, records, conflict); err == nil || !strings.Contains(err.Error(), "different contents") {
		t.Fatalf("conflicting redaction registration error = %v", err)
	}
}

func TestRegisterRedactionRequiresCurrentSourceAndNoLegalHold(t *testing.T) {
	artifacts := store.NewMemory()
	records := NewMemory()
	original, err := artifacts.Put(strings.NewReader("original bytes"), store.PutOptions{MediaType: "text/plain"})
	if err != nil {
		t.Fatal(err)
	}
	resulting, err := artifacts.Put(strings.NewReader("redacted bytes"), store.PutOptions{MediaType: "text/plain"})
	if err != nil {
		t.Fatal(err)
	}
	record := testRecord(t, "lockwood-redaction-registration-guard")
	record.Artifact = original
	record.Parents = []Lineage{}
	if err := records.Put(record); err != nil {
		t.Fatal(err)
	}
	base := HandlingEvent{
		Schema:          HandlingEventSchema,
		EventID:         "event-redaction-registration-guard-0001",
		CustodyID:       record.CustodyID,
		Type:            RedactionEvent,
		RecordedAt:      time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC),
		Actor:           "operator@example",
		Reason:          "removed restricted fields",
		OriginalDigest:  "sha256:" + strings.Repeat("a", 64),
		ResultingDigest: resulting.Digest,
	}
	if _, err := RegisterRedaction(artifacts, records, records, base); err == nil || !strings.Contains(err.Error(), "does not match current source") {
		t.Fatalf("wrong current source error = %v", err)
	}

	hold := testHandlingEvent("event-redaction-registration-hold", LegalHoldPlacedEvent)
	hold.CustodyID = record.CustodyID
	if err := records.AppendEvent(hold); err != nil {
		t.Fatal(err)
	}
	blocked := base
	blocked.EventID = "event-redaction-registration-blocked"
	blocked.RecordedAt = base.RecordedAt.Add(time.Minute)
	blocked.OriginalDigest = original.Digest
	if _, err := RegisterRedaction(artifacts, records, records, blocked); err == nil || !strings.Contains(err.Error(), "blocked by active legal hold") {
		t.Fatalf("held redaction error = %v", err)
	}
}

func TestRegisterRedactionRejectsMissingResultArtifact(t *testing.T) {
	artifacts := store.NewMemory()
	records := NewMemory()
	original, err := artifacts.Put(strings.NewReader("original bytes"), store.PutOptions{MediaType: "text/plain"})
	if err != nil {
		t.Fatal(err)
	}
	record := testRecord(t, "lockwood-redaction-registration-missing-result")
	record.Artifact = original
	record.Parents = []Lineage{}
	if err := records.Put(record); err != nil {
		t.Fatal(err)
	}
	event := testHandlingEvent("event-redaction-registration-missing-result", RedactionEvent)
	event.CustodyID = record.CustodyID
	event.RecordedAt = time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	event.OriginalDigest = original.Digest
	event.ResultingDigest = "sha256:" + strings.Repeat("c", 64)
	if _, err := RegisterRedaction(artifacts, records, records, event); err == nil || !strings.Contains(err.Error(), "verify resulting redaction artifact") {
		t.Fatalf("missing result artifact error = %v", err)
	}
	if events, err := records.ListEvents(record.CustodyID); err != nil || len(events) != 0 {
		t.Fatalf("failed registration left handling events: events=%+v err=%v", events, err)
	}
}
