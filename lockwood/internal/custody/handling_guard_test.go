package custody

import (
	"strings"
	"testing"
	"time"
)

func TestEvaluateHandlingGuardBlocksRedactAndDeleteDuringLegalHold(t *testing.T) {
	records := NewMemory()
	record := testRecord(t, "lockwood-handling-guard-record")
	if err := records.Put(record); err != nil {
		t.Fatal(err)
	}

	decision, err := EvaluateHandlingGuard(records, records, record, RedactAction)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != HandlingNotBlocked || len(decision.ActiveLegalHoldIDs) != 0 {
		t.Fatalf("clear handling guard = %+v", decision)
	}

	hold := testHandlingEvent("event-guard-place", LegalHoldPlacedEvent)
	hold.CustodyID = record.CustodyID
	hold.RecordedAt = time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	if err := records.AppendEvent(hold); err != nil {
		t.Fatal(err)
	}
	for _, action := range []HandlingAction{RedactAction, DeleteAction} {
		decision, err := EvaluateHandlingGuard(records, records, record, action)
		if err != nil {
			t.Fatal(err)
		}
		if decision.Status != HandlingBlocked || len(decision.ActiveLegalHoldIDs) != 1 || decision.ActiveLegalHoldIDs[0] != hold.LegalHoldID {
			t.Fatalf("held handling guard for %s = %+v", action, decision)
		}
	}

	release := testHandlingEvent("event-guard-release", LegalHoldReleasedEvent)
	release.CustodyID = record.CustodyID
	release.RecordedAt = hold.RecordedAt.Add(time.Minute)
	if err := records.AppendEvent(release); err != nil {
		t.Fatal(err)
	}
	decision, err = EvaluateHandlingGuard(records, records, record, DeleteAction)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != HandlingNotBlocked || len(decision.ActiveLegalHoldIDs) != 0 {
		t.Fatalf("released handling guard = %+v", decision)
	}
	stored, err := records.Get(record.CustodyID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Handling != record.Handling || stored.Artifact.Digest != record.Artifact.Digest {
		t.Fatalf("handling guard changed custody record: %+v", stored)
	}
}

func TestEvaluateHandlingGuardRejectsUnsupportedAction(t *testing.T) {
	record := testRecord(t, "lockwood-handling-guard-invalid")
	if _, err := EvaluateHandlingGuard(NewMemory(), NewMemory(), record, HandlingAction("export")); err == nil || !strings.Contains(err.Error(), "unsupported handling guard action") {
		t.Fatalf("unsupported action error = %v", err)
	}
}
