package custody

import (
	"strings"
	"testing"
	"time"

	"ingen/lockwood/internal/store"
)

func TestPromoteRedactionResultCreatesVerifiableDerivedCustody(t *testing.T) {
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
	source := testRecord(t, "lockwood-redaction-promotion-source")
	source.Artifact = original
	source.Parents = []Lineage{}
	if err := records.Put(source); err != nil {
		t.Fatal(err)
	}
	event := HandlingEvent{
		Schema:          HandlingEventSchema,
		EventID:         "event-redaction-promotion-0001",
		CustodyID:       source.CustodyID,
		Type:            RedactionEvent,
		RecordedAt:      time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC),
		Actor:           "operator@example",
		Reason:          "removed restricted fields",
		OriginalDigest:  original.Digest,
		ResultingDigest: resulting.Digest,
	}
	if _, err := RegisterRedaction(artifacts, records, records, event); err != nil {
		t.Fatal(err)
	}

	request := RedactionPromotionRequest{
		CustodyID:  "lockwood-redaction-promotion-result",
		Artifact:   resulting,
		ReceivedAt: time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC),
		Producer:   Producer{Tool: "redactor", Kind: "sanitized-export", Version: "1"},
		Source:     Source{RunID: "redaction-run-0001", Path: "redacted.txt"},
	}
	promoted, err := PromoteRedactionResult(artifacts, records, records, source.CustodyID, event.EventID, request)
	if err != nil {
		t.Fatal(err)
	}
	if promoted.Status != Accepted || promoted.Artifact.Digest != resulting.Digest || promoted.Handling.Redaction != "redacted" {
		t.Fatalf("promoted record = %+v", promoted)
	}
	if len(promoted.Parents) != 1 || promoted.Parents[0].Relation != DerivedFrom || promoted.Parents[0].Digest != original.Digest {
		t.Fatalf("promoted lineage = %+v", promoted.Parents)
	}
	if _, err := VerifyRecord(records, artifacts, promoted.CustodyID); err != nil {
		t.Fatalf("promoted record failed verification: %v", err)
	}
	sourceAfter, err := records.Get(source.CustodyID)
	if err != nil {
		t.Fatal(err)
	}
	if sourceAfter.Artifact.Digest != original.Digest || sourceAfter.Handling != source.Handling {
		t.Fatalf("source custody record changed: before=%+v after=%+v", source, sourceAfter)
	}
	if err := artifacts.Verify(original.Digest); err != nil {
		t.Fatalf("original artifact was not preserved: %v", err)
	}

	if _, err := PromoteRedactionResult(artifacts, records, records, source.CustodyID, event.EventID, request); err != nil {
		t.Fatalf("identical promotion was not idempotent: %v", err)
	}
	conflict := request
	conflict.Producer.Tool = "different-redactor"
	if _, err := PromoteRedactionResult(artifacts, records, records, source.CustodyID, event.EventID, conflict); err == nil || !strings.Contains(err.Error(), "already exists with different contents") {
		t.Fatalf("conflicting promotion error = %v", err)
	}
}

func TestPromoteRedactionResultRejectsUnregisteredOrMismatchedResult(t *testing.T) {
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
	source := testRecord(t, "lockwood-redaction-promotion-unregistered")
	source.Artifact = original
	source.Parents = []Lineage{}
	if err := records.Put(source); err != nil {
		t.Fatal(err)
	}
	request := RedactionPromotionRequest{
		CustodyID:  "lockwood-redaction-promotion-unregistered-result",
		Artifact:   resulting,
		ReceivedAt: time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC),
		Producer:   Producer{Tool: "redactor", Kind: "sanitized-export"},
		Source:     Source{Path: "redacted.txt"},
	}
	if _, err := PromoteRedactionResult(artifacts, records, records, source.CustodyID, "event-missing", request); err == nil || !strings.Contains(err.Error(), "read redaction event") {
		t.Fatalf("missing event error = %v", err)
	}
	event := testHandlingEvent("event-redaction-promotion-mismatch", RedactionEvent)
	event.CustodyID = source.CustodyID
	event.OriginalDigest = original.Digest
	if err := records.AppendEvent(event); err != nil {
		t.Fatal(err)
	}
	request.Artifact.Digest = original.Digest
	if _, err := PromoteRedactionResult(artifacts, records, records, source.CustodyID, event.EventID, request); err == nil || !strings.Contains(err.Error(), "does not match redaction result") {
		t.Fatalf("mismatched result error = %v", err)
	}
}
