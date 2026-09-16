package custody

import (
	"bytes"
	"fmt"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/store"
)

// RegisterRedaction verifies a caller-produced resulting artifact and records
// its relationship to the current redaction source as one append-only event.
// It never edits or deletes the original artifact or custody record.
func RegisterRedaction(artifacts store.Store, records RecordStore, events HandlingEventStore, event HandlingEvent) (HandlingEvent, error) {
	if artifacts == nil {
		return HandlingEvent{}, fmt.Errorf("artifact store is required")
	}
	if records == nil {
		return HandlingEvent{}, fmt.Errorf("custody record store is required")
	}
	if events == nil {
		return HandlingEvent{}, fmt.Errorf("handling event store is required")
	}
	if event.Type != RedactionEvent {
		return HandlingEvent{}, fmt.Errorf("redaction registration requires a redaction event")
	}
	event.RecordedAt = event.RecordedAt.UTC()
	if err := event.Validate(); err != nil {
		return HandlingEvent{}, fmt.Errorf("validate redaction event: %w", err)
	}
	var registered HandlingEvent
	err := withHandlingEventLock(events, event.CustodyID, func() error {
		var err error
		registered, err = registerRedactionLocked(artifacts, records, events, event)
		return err
	})
	return registered, err
}

func registerRedactionLocked(artifacts store.Store, records RecordStore, events HandlingEventStore, event HandlingEvent) (HandlingEvent, error) {
	items, err := events.ListEvents(event.CustodyID)
	if err != nil {
		return HandlingEvent{}, err
	}
	for _, existing := range items {
		if existing.EventID != event.EventID {
			continue
		}
		existingDigest, digestErr := HandlingEventDigest(existing)
		if digestErr != nil {
			return HandlingEvent{}, fmt.Errorf("digest existing redaction event: %w", digestErr)
		}
		eventDigest, digestErr := HandlingEventDigest(event)
		if digestErr != nil {
			return HandlingEvent{}, fmt.Errorf("digest redaction event: %w", digestErr)
		}
		if bytes.Equal([]byte(existingDigest), []byte(eventDigest)) {
			if err := artifacts.Verify(event.OriginalDigest); err != nil {
				return HandlingEvent{}, fmt.Errorf("verify original redaction artifact: %w", err)
			}
			if err := artifacts.Verify(event.ResultingDigest); err != nil {
				return HandlingEvent{}, fmt.Errorf("verify resulting redaction artifact: %w", err)
			}
			return existing, nil
		}
		return HandlingEvent{}, fmt.Errorf("handling event %q for custody record %q already exists with different contents", event.EventID, event.CustodyID)
	}

	record, err := records.Get(event.CustodyID)
	if err != nil {
		return HandlingEvent{}, fmt.Errorf("read custody record: %w", err)
	}
	status, err := AnalyzeHandling(records, events, record)
	if err != nil {
		return HandlingEvent{}, err
	}
	guard, err := EvaluateHandlingGuard(records, events, record, RedactAction)
	if err != nil {
		return HandlingEvent{}, err
	}
	if guard.Status == HandlingBlocked {
		return HandlingEvent{}, fmt.Errorf("redaction blocked by active legal hold: %v", guard.ActiveLegalHoldIDs)
	}
	expectedOriginal := record.Artifact.Digest
	var lastRedactionAt = record.ReceivedAt
	if len(status.Redactions) > 0 {
		last := status.Redactions[len(status.Redactions)-1]
		expectedOriginal = last.ResultingDigest
		lastRedactionAt = last.RecordedAt
	}
	if event.OriginalDigest != expectedOriginal {
		return HandlingEvent{}, fmt.Errorf("redaction original digest %s does not match current source %s", event.OriginalDigest, expectedOriginal)
	}
	if !event.RecordedAt.After(lastRedactionAt) {
		return HandlingEvent{}, fmt.Errorf("redaction event time must be after the current redaction source time")
	}
	if err := artifacts.Verify(event.OriginalDigest); err != nil {
		return HandlingEvent{}, fmt.Errorf("verify original redaction artifact: %w", err)
	}
	if err := artifacts.Verify(event.ResultingDigest); err != nil {
		return HandlingEvent{}, fmt.Errorf("verify resulting redaction artifact: %w", err)
	}
	if err := appendHandlingEventWhileLocked(events, event); err != nil {
		return HandlingEvent{}, fmt.Errorf("append redaction event: %w", err)
	}
	return event, nil
}

// ValidateRedactionDigests performs the digest-shape checks used by callers
// before a resulting artifact is published.
func ValidateRedactionDigests(originalDigest, resultingDigest string) error {
	if err := artifact.ValidateDigest(originalDigest); err != nil {
		return fmt.Errorf("invalid original redaction digest: %w", err)
	}
	if err := artifact.ValidateDigest(resultingDigest); err != nil {
		return fmt.Errorf("invalid resulting redaction digest: %w", err)
	}
	if originalDigest == resultingDigest {
		return fmt.Errorf("redaction digests must differ")
	}
	return nil
}
