package custody

import (
	"bytes"
	"fmt"
	"sort"
)

// AppendEvent preserves the same validation, canonical-value, idempotency,
// and conflict semantics as the filesystem event store.
func (s *Memory) AppendEvent(event HandlingEvent) error {
	if s == nil {
		return fmt.Errorf("memory custody store is required")
	}
	event.RecordedAt = event.RecordedAt.UTC()
	encoded, err := MarshalCanonicalHandlingEvent(event)
	if err != nil {
		return fmt.Errorf("encode handling event: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.events == nil {
		s.events = make(map[string]map[string]HandlingEvent)
	}
	if s.events[event.CustodyID] == nil {
		s.events[event.CustodyID] = make(map[string]HandlingEvent)
	}
	if existing, ok := s.events[event.CustodyID][event.EventID]; ok {
		existingEncoded, err := MarshalCanonicalHandlingEvent(existing)
		if err != nil {
			return fmt.Errorf("encode existing handling event: %w", err)
		}
		if bytes.Equal(existingEncoded, encoded) {
			return nil
		}
		return fmt.Errorf("handling event %q for custody record %q already exists with different contents", event.EventID, event.CustodyID)
	}
	s.events[event.CustodyID][event.EventID] = event
	return nil
}

func (s *Memory) GetEvent(custodyID, eventID string) (HandlingEvent, error) {
	if s == nil {
		return HandlingEvent{}, fmt.Errorf("memory custody store is required")
	}
	if !custodyIDPattern.MatchString(custodyID) {
		return HandlingEvent{}, fmt.Errorf("invalid handling event custody id %q", custodyID)
	}
	if !eventIDPattern.MatchString(eventID) {
		return HandlingEvent{}, fmt.Errorf("invalid handling event id %q", eventID)
	}
	s.mu.RLock()
	events := s.events[custodyID]
	event, ok := events[eventID]
	s.mu.RUnlock()
	if !ok {
		return HandlingEvent{}, fmt.Errorf("handling event %q for custody record %q not found", eventID, custodyID)
	}
	return event, nil
}

func (s *Memory) ListEvents(custodyID string) ([]HandlingEvent, error) {
	if s == nil {
		return nil, fmt.Errorf("memory custody store is required")
	}
	if !custodyIDPattern.MatchString(custodyID) {
		return nil, fmt.Errorf("invalid handling event custody id %q", custodyID)
	}
	s.mu.RLock()
	eventsByID := s.events[custodyID]
	events := make([]HandlingEvent, 0, len(eventsByID))
	for _, event := range eventsByID {
		events = append(events, event)
	}
	s.mu.RUnlock()
	sort.Slice(events, func(i, j int) bool {
		if !events[i].RecordedAt.Equal(events[j].RecordedAt) {
			return events[i].RecordedAt.Before(events[j].RecordedAt)
		}
		return events[i].EventID < events[j].EventID
	})
	return events, nil
}
