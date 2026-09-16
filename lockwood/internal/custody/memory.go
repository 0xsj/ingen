package custody

import (
	"bytes"
	"fmt"
	"sort"
	"sync"
)

// Memory is a process-local custody-record store. It preserves the same
// validation, canonical-value, conflict, and deterministic-list semantics as
// the filesystem store, but provides no durability guarantees.
type Memory struct {
	mu           sync.RWMutex
	records      map[string]Record
	events       map[string]map[string]HandlingEvent
	eventLocksMu sync.Mutex
	eventLocks   map[string]*sync.Mutex
}

var _ RecordStore = (*Filesystem)(nil)
var _ RecordStore = (*Memory)(nil)
var _ HandlingEventStore = (*Filesystem)(nil)
var _ HandlingEventStore = (*Memory)(nil)

func NewMemory() *Memory {
	return &Memory{records: make(map[string]Record), events: make(map[string]map[string]HandlingEvent)}
}

func (s *Memory) Put(record Record) error {
	if s == nil {
		return fmt.Errorf("memory custody store is required")
	}
	record = normalizeRecord(record)
	encoded, err := MarshalCanonical(record)
	if err != nil {
		return fmt.Errorf("encode custody record: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.records == nil {
		s.records = make(map[string]Record)
	}
	if existing, ok := s.records[record.CustodyID]; ok {
		existingEncoded, err := MarshalCanonical(existing)
		if err != nil {
			return fmt.Errorf("encode existing custody record: %w", err)
		}
		if bytes.Equal(existingEncoded, encoded) {
			return nil
		}
		return fmt.Errorf("custody record %q already exists with different contents", record.CustodyID)
	}
	s.records[record.CustodyID] = cloneRecord(record)
	return nil
}

func (s *Memory) Get(custodyID string) (Record, error) {
	if s == nil {
		return Record{}, fmt.Errorf("memory custody store is required")
	}
	if !custodyIDPattern.MatchString(custodyID) {
		return Record{}, fmt.Errorf("invalid custody id %q", custodyID)
	}
	s.mu.RLock()
	record, ok := s.records[custodyID]
	s.mu.RUnlock()
	if !ok {
		return Record{}, fmt.Errorf("custody record %q not found", custodyID)
	}
	return cloneRecord(record), nil
}

func (s *Memory) List() ([]Record, error) {
	if s == nil {
		return nil, fmt.Errorf("memory custody store is required")
	}
	s.mu.RLock()
	ids := make([]string, 0, len(s.records))
	for id := range s.records {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	records := make([]Record, 0, len(ids))
	for _, id := range ids {
		records = append(records, cloneRecord(s.records[id]))
	}
	s.mu.RUnlock()
	return records, nil
}

func cloneRecord(record Record) Record {
	if record.Parents != nil {
		parents := make([]Lineage, len(record.Parents))
		copy(parents, record.Parents)
		record.Parents = parents
	}
	if record.Integrity.VerifiedAt != nil {
		verifiedAt := *record.Integrity.VerifiedAt
		record.Integrity.VerifiedAt = &verifiedAt
	}
	return record
}
