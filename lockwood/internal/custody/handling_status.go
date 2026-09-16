package custody

import (
	"fmt"
	"sort"
	"time"
)

// HandlingRedaction is the descriptive redaction history projected from one
// immutable redaction event. It does not claim that the resulting bytes exist
// or that Lockwood performed the redaction.
type HandlingRedaction struct {
	EventID         string    `json:"event_id"`
	RecordedAt      time.Time `json:"recorded_at"`
	OriginalDigest  string    `json:"original_digest"`
	ResultingDigest string    `json:"resulting_digest"`
}

// HandlingStatus is a read-only projection of a custody record and its
// handling events. It describes recorded decisions; it is not an enforcement
// result and does not alter the underlying record or artifact.
type HandlingStatus struct {
	CustodyID          string              `json:"custody_id"`
	ArtifactDigest     string              `json:"artifact_digest"`
	EventCount         int                 `json:"event_count"`
	LastEventAt        *time.Time          `json:"last_event_at,omitempty"`
	Redaction          string              `json:"redaction"`
	RetentionClass     string              `json:"retention_class"`
	ActiveLegalHoldIDs []string            `json:"active_legal_hold_ids"`
	Redactions         []HandlingRedaction `json:"redactions"`
}

// AnalyzeHandling returns a deterministic, non-mutating handling projection.
// Redaction result digests remain descriptive because this operation does not
// retrieve, create, or replace payload artifacts.
func AnalyzeHandling(records RecordStore, events HandlingEventStore, record Record) (HandlingStatus, error) {
	status := HandlingStatus{
		CustodyID:          record.CustodyID,
		ArtifactDigest:     record.Artifact.Digest,
		Redaction:          record.Handling.Redaction,
		RetentionClass:     record.Handling.RetentionClass,
		ActiveLegalHoldIDs: make([]string, 0),
		Redactions:         make([]HandlingRedaction, 0),
	}
	if records == nil {
		return status, fmt.Errorf("custody record store is required")
	}
	if events == nil {
		return status, fmt.Errorf("handling event store is required")
	}
	if err := record.Validate(); err != nil {
		return status, fmt.Errorf("validate handling root: %w", err)
	}
	stored, err := records.Get(record.CustodyID)
	if err != nil {
		return status, fmt.Errorf("load handling root: %w", err)
	}
	if stored.CustodyID != record.CustodyID {
		return status, fmt.Errorf("handling root custody ID does not match requested record")
	}
	items, err := events.ListEvents(record.CustodyID)
	if err != nil {
		return status, err
	}
	status.EventCount = len(items)
	activeHolds := make(map[string]struct{})
	for _, event := range items {
		if event.CustodyID != record.CustodyID {
			return status, fmt.Errorf("handling event %q targets custody record %q", event.EventID, event.CustodyID)
		}
		if status.LastEventAt == nil || status.LastEventAt.Before(event.RecordedAt) {
			when := event.RecordedAt
			status.LastEventAt = &when
		}
		switch event.Type {
		case RedactionEvent:
			status.Redaction = "redacted"
			status.Redactions = append(status.Redactions, HandlingRedaction{
				EventID:         event.EventID,
				RecordedAt:      event.RecordedAt,
				OriginalDigest:  event.OriginalDigest,
				ResultingDigest: event.ResultingDigest,
			})
		case RetentionClassifiedEvent:
			status.RetentionClass = event.RetentionClass
		case LegalHoldPlacedEvent:
			activeHolds[event.LegalHoldID] = struct{}{}
		case LegalHoldReleasedEvent:
			delete(activeHolds, event.LegalHoldID)
		default:
			return status, fmt.Errorf("unsupported handling event type %q", event.Type)
		}
	}
	for holdID := range activeHolds {
		status.ActiveLegalHoldIDs = append(status.ActiveLegalHoldIDs, holdID)
	}
	sort.Strings(status.ActiveLegalHoldIDs)
	return status, nil
}
