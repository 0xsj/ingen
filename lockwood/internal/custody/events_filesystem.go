package custody

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// AppendEvent validates and atomically publishes one immutable handling event.
// Repeating the same event is idempotent; reusing an event ID with different
// bytes is rejected.
func (s *Filesystem) AppendEvent(event HandlingEvent) error {
	return s.withHandlingEventLock(event.CustodyID, func() error {
		return s.appendHandlingEvent(event)
	})
}

func (s *Filesystem) appendHandlingEvent(event HandlingEvent) error {
	event.RecordedAt = event.RecordedAt.UTC()
	encoded, err := MarshalCanonicalHandlingEvent(event)
	if err != nil {
		return fmt.Errorf("encode handling event: %w", err)
	}
	path, err := s.eventPath(event.CustodyID, event.EventID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create handling event directory: %w", err)
	}
	if existing, err := os.ReadFile(path); err == nil {
		if bytes.Equal(existing, encoded) {
			return nil
		}
		return fmt.Errorf("handling event %q for custody record %q already exists with different contents", event.EventID, event.CustodyID)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect existing handling event: %w", err)
	}

	temp, err := os.CreateTemp(s.tempRoot, "event-")
	if err != nil {
		return fmt.Errorf("create temporary handling event: %w", err)
	}
	tempName := temp.Name()
	defer func() { _ = os.Remove(tempName) }()
	if _, err := temp.Write(encoded); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write temporary handling event: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync temporary handling event: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary handling event: %w", err)
	}
	if err := os.Link(tempName, path); err != nil {
		if !os.IsExist(err) {
			return fmt.Errorf("publish handling event: %w", err)
		}
		existing, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read racing handling event: %w", readErr)
		}
		if !bytes.Equal(existing, encoded) {
			return fmt.Errorf("handling event %q for custody record %q already exists with different contents", event.EventID, event.CustodyID)
		}
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sync handling event directory: %w", err)
	}
	return nil
}

func (s *Filesystem) withHandlingEventLock(custodyID string, fn func() error) error {
	if s == nil {
		return fmt.Errorf("custody handling-event store is required")
	}
	if !custodyIDPattern.MatchString(custodyID) {
		return fmt.Errorf("invalid handling event custody id %q", custodyID)
	}
	if fn == nil {
		return fmt.Errorf("handling-event lock callback is required")
	}
	path := filepath.Join(s.root, "locks", "handling-events", custodyID+".lock")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create handling-event lock directory: %w", err)
	}
	return withExclusiveHandlingLock(path, fn)
}

func (s *Filesystem) GetEvent(custodyID, eventID string) (HandlingEvent, error) {
	path, err := s.eventPath(custodyID, eventID)
	if err != nil {
		return HandlingEvent{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return HandlingEvent{}, fmt.Errorf("read handling event: %w", err)
	}
	event, err := UnmarshalCanonicalHandlingEvent(data)
	if err != nil {
		return HandlingEvent{}, err
	}
	if event.CustodyID != custodyID || event.EventID != eventID {
		return HandlingEvent{}, fmt.Errorf("handling event identity does not match requested path")
	}
	return event, nil
}

func (s *Filesystem) ListEvents(custodyID string) ([]HandlingEvent, error) {
	if !custodyIDPattern.MatchString(custodyID) {
		return nil, fmt.Errorf("invalid handling event custody id %q", custodyID)
	}
	directory := filepath.Join(s.root, "events", custodyID)
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return []HandlingEvent{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list handling events: %w", err)
	}
	events := make([]HandlingEvent, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(entry.Name(), ".json") {
			return nil, fmt.Errorf("unexpected handling event entry: %s", entry.Name())
		}
		eventID := strings.TrimSuffix(entry.Name(), ".json")
		event, err := s.GetEvent(custodyID, eventID)
		if err != nil {
			return nil, fmt.Errorf("load handling event %s: %w", eventID, err)
		}
		events = append(events, event)
	}
	sort.Slice(events, func(i, j int) bool {
		if !events[i].RecordedAt.Equal(events[j].RecordedAt) {
			return events[i].RecordedAt.Before(events[j].RecordedAt)
		}
		return events[i].EventID < events[j].EventID
	})
	return events, nil
}

func (s *Filesystem) eventPath(custodyID, eventID string) (string, error) {
	if !custodyIDPattern.MatchString(custodyID) {
		return "", fmt.Errorf("invalid handling event custody id %q", custodyID)
	}
	if !eventIDPattern.MatchString(eventID) {
		return "", fmt.Errorf("invalid handling event id %q", eventID)
	}
	return filepath.Join(s.root, "events", custodyID, eventID+".json"), nil
}
