package custody

import (
	"strings"
	"testing"
	"time"

	"ingen/lockwood/internal/store"
)

func TestAnalyzeRedactionReportsUnanchoredThenPromotedResult(t *testing.T) {
	artifacts := store.NewMemory()
	records := NewMemory()
	original, err := artifacts.Put(strings.NewReader("status original bytes"), store.PutOptions{MediaType: "text/plain"})
	if err != nil {
		t.Fatal(err)
	}
	resulting, err := artifacts.Put(strings.NewReader("status redacted bytes"), store.PutOptions{MediaType: "text/plain", LogicalName: "redacted.txt"})
	if err != nil {
		t.Fatal(err)
	}
	source := testRecord(t, "lockwood-redaction-status-source")
	source.Artifact = original
	source.Parents = []Lineage{}
	if err := records.Put(source); err != nil {
		t.Fatal(err)
	}
	event := HandlingEvent{
		Schema:          HandlingEventSchema,
		EventID:         "event-redaction-status-0001",
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

	status, err := AnalyzeRedaction(records, artifacts, records, source.CustodyID, event.EventID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != RedactionUnanchored || !status.OriginalArtifactVerified || !status.ResultingArtifactVerified || len(status.OriginalCustodyIDs) != 1 || len(status.ResultCustodyIDs) != 0 || len(status.PromotedCustodyIDs) != 0 || len(status.Issues) != 0 {
		t.Fatalf("unanchored redaction status = %+v", status)
	}

	promoted, err := PromoteRedactionResult(artifacts, records, records, source.CustodyID, event.EventID, RedactionPromotionRequest{
		CustodyID:  "lockwood-redaction-status-result",
		Artifact:   resulting,
		ReceivedAt: time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC),
		Producer:   Producer{Tool: "redactor", Kind: "sanitized-export"},
		Source:     Source{Path: "redacted.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	status, err = AnalyzeRedaction(records, artifacts, records, source.CustodyID, event.EventID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != RedactionComplete || len(status.ResultCustodyIDs) != 1 || len(status.PromotedCustodyIDs) != 1 || status.ResultCustodyIDs[0] != promoted.CustodyID || status.PromotedCustodyIDs[0] != promoted.CustodyID || len(status.Issues) != 0 {
		t.Fatalf("promoted redaction status = %+v", status)
	}
}

func TestAnalyzeRedactionRejectsMissingAndNonRedactionEvents(t *testing.T) {
	records := NewMemory()
	artifacts := store.NewMemory()
	record := testRecord(t, "lockwood-redaction-status-errors")
	record.Parents = []Lineage{}
	ref, err := artifacts.Put(strings.NewReader("status error bytes"), store.PutOptions{MediaType: "text/plain"})
	if err != nil {
		t.Fatal(err)
	}
	record.Artifact = ref
	if err := records.Put(record); err != nil {
		t.Fatal(err)
	}
	if _, err := AnalyzeRedaction(records, artifacts, records, record.CustodyID, "missing-event"); err == nil || !strings.Contains(err.Error(), "read redaction event") {
		t.Fatalf("missing event error = %v", err)
	}
	event := testHandlingEvent("event-redaction-status-retention", RetentionClassifiedEvent)
	event.CustodyID = record.CustodyID
	if err := records.AppendEvent(event); err != nil {
		t.Fatal(err)
	}
	if _, err := AnalyzeRedaction(records, artifacts, records, record.CustodyID, event.EventID); err == nil || !strings.Contains(err.Error(), "not a redaction event") {
		t.Fatalf("non-redaction event error = %v", err)
	}
}
