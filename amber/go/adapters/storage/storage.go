// Package amberstorage provides an append-only provenance store contract and
// an in-memory reference implementation.
package amberstorage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	amber "github.com/0xsj/ingen/amber"
)

var (
	ErrNotFound = errors.New("amber provenance not found")
	ErrConflict = errors.New("amber provenance storage conflict")
)

// Store persists immutable provenance values by execution ID.
type Store interface {
	Put(ctx context.Context, provenance amber.Provenance) error
	Get(ctx context.Context, executionID amber.ID) (amber.Provenance, error)
	ListByWorkID(ctx context.Context, workID amber.ID) ([]amber.Provenance, error)
	ListByCausation(ctx context.Context, kind string, id amber.ID) ([]amber.Provenance, error)
	ListByCorrelationID(ctx context.Context, correlationID amber.ID) ([]amber.Provenance, error)
}

// MemoryStore is a concurrency-safe process-local Store. It is useful as a
// reference implementation and for tests; it is not durable storage.
type MemoryStore struct {
	mu      sync.RWMutex
	records map[amber.ID][]byte
}

// FileStore is a process-safe, file-backed Store. Each successful write
// replaces an atomic JSON snapshot; unlike a database adapter, it does not
// provide cross-process locking or transactional query support.
type FileStore struct {
	mu   sync.Mutex
	path string
}

type fileSnapshot struct {
	Version int                        `json:"version"`
	Records map[string]json.RawMessage `json:"records"`
}

// NewFileStore creates a Store backed by path. The parent directory must
// already exist; the file itself is created on the first successful Put.
func NewFileStore(path string) (*FileStore, error) {
	if path == "" {
		return nil, fmt.Errorf("%w: file store path cannot be empty", amber.ErrInvalidTransition)
	}
	return &FileStore{path: path}, nil
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{records: make(map[amber.ID][]byte)}
}

func (s *MemoryStore) Put(ctx context.Context, provenance amber.Provenance) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := provenance.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(provenance)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.records[provenance.ExecutionID()]; ok {
		if bytes.Equal(existing, data) {
			return nil
		}
		return fmt.Errorf("%w: execution_id %s", ErrConflict, provenance.ExecutionID())
	}
	s.records[provenance.ExecutionID()] = append([]byte(nil), data...)
	return nil
}

func (s *MemoryStore) Get(ctx context.Context, executionID amber.ID) (amber.Provenance, error) {
	if err := contextError(ctx); err != nil {
		return amber.Provenance{}, err
	}
	s.mu.RLock()
	data, ok := s.records[executionID]
	if ok {
		data = append([]byte(nil), data...)
	}
	s.mu.RUnlock()
	if !ok {
		return amber.Provenance{}, fmt.Errorf("%w: execution_id %s", ErrNotFound, executionID)
	}
	return amber.FromJSON(data)
}

func (s *MemoryStore) ListByWorkID(ctx context.Context, workID amber.ID) ([]amber.Provenance, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	data := make([][]byte, 0, len(s.records))
	for _, record := range s.records {
		data = append(data, append([]byte(nil), record...))
	}
	s.mu.RUnlock()
	return decodeHistory(data, workID)
}

func (s *MemoryStore) ListByCausation(ctx context.Context, kind string, id amber.ID) ([]amber.Provenance, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	data := make([][]byte, 0, len(s.records))
	for _, record := range s.records {
		data = append(data, append([]byte(nil), record...))
	}
	s.mu.RUnlock()
	return decodeCausationHistory(data, kind, id)
}

func (s *MemoryStore) ListByCorrelationID(ctx context.Context, correlationID amber.ID) ([]amber.Provenance, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	data := make([][]byte, 0, len(s.records))
	for _, record := range s.records {
		data = append(data, append([]byte(nil), record...))
	}
	s.mu.RUnlock()
	return decodeCorrelationHistory(data, correlationID)
}

func (s *FileStore) Put(ctx context.Context, provenance amber.Provenance) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := provenance.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(provenance)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot, err := s.readLocked()
	if err != nil {
		return err
	}
	if existing, ok := snapshot.Records[string(provenance.ExecutionID())]; ok {
		if bytes.Equal(existing, data) {
			return nil
		}
		return fmt.Errorf("%w: execution_id %s", ErrConflict, provenance.ExecutionID())
	}
	snapshot.Records[string(provenance.ExecutionID())] = append(json.RawMessage(nil), data...)
	return s.writeLocked(snapshot)
}

func (s *FileStore) Get(ctx context.Context, executionID amber.ID) (amber.Provenance, error) {
	if err := contextError(ctx); err != nil {
		return amber.Provenance{}, err
	}

	s.mu.Lock()
	snapshot, err := s.readLocked()
	s.mu.Unlock()
	if err != nil {
		return amber.Provenance{}, err
	}
	data, ok := snapshot.Records[string(executionID)]
	if !ok {
		return amber.Provenance{}, fmt.Errorf("%w: execution_id %s", ErrNotFound, executionID)
	}
	return amber.FromJSON(data)
}

func (s *FileStore) ListByWorkID(ctx context.Context, workID amber.ID) ([]amber.Provenance, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}

	s.mu.Lock()
	snapshot, err := s.readLocked()
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	data := make([][]byte, 0, len(snapshot.Records))
	for _, record := range snapshot.Records {
		data = append(data, append([]byte(nil), record...))
	}
	return decodeHistory(data, workID)
}

func (s *FileStore) ListByCausation(ctx context.Context, kind string, id amber.ID) ([]amber.Provenance, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}

	s.mu.Lock()
	snapshot, err := s.readLocked()
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	data := make([][]byte, 0, len(snapshot.Records))
	for _, record := range snapshot.Records {
		data = append(data, append([]byte(nil), record...))
	}
	return decodeCausationHistory(data, kind, id)
}

func (s *FileStore) ListByCorrelationID(ctx context.Context, correlationID amber.ID) ([]amber.Provenance, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}

	s.mu.Lock()
	snapshot, err := s.readLocked()
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	data := make([][]byte, 0, len(snapshot.Records))
	for _, record := range snapshot.Records {
		data = append(data, append([]byte(nil), record...))
	}
	return decodeCorrelationHistory(data, correlationID)
}

func decodeHistory(data [][]byte, workID amber.ID) ([]amber.Provenance, error) {
	history := make([]amber.Provenance, 0)
	for _, record := range data {
		provenance, err := amber.FromJSON(record)
		if err != nil {
			return nil, err
		}
		if provenance.WorkID() == workID {
			history = append(history, provenance)
		}
	}
	sortHistory(history)
	return history, nil
}

func decodeCausationHistory(data [][]byte, kind string, id amber.ID) ([]amber.Provenance, error) {
	history := make([]amber.Provenance, 0)
	for _, record := range data {
		provenance, err := amber.FromJSON(record)
		if err != nil {
			return nil, err
		}
		causation, ok := provenance.Causation()
		if ok && causation.Kind == kind && causation.ID == id {
			history = append(history, provenance)
		}
	}
	sortHistory(history)
	return history, nil
}

func decodeCorrelationHistory(data [][]byte, correlationID amber.ID) ([]amber.Provenance, error) {
	history := make([]amber.Provenance, 0)
	for _, record := range data {
		provenance, err := amber.FromJSON(record)
		if err != nil {
			return nil, err
		}
		if provenance.CorrelationID() == correlationID {
			history = append(history, provenance)
		}
	}
	sortHistory(history)
	return history, nil
}

func sortHistory(history []amber.Provenance) {
	sort.Slice(history, func(i, j int) bool {
		if history[i].Depth() != history[j].Depth() {
			return history[i].Depth() < history[j].Depth()
		}
		if history[i].Attempt() != history[j].Attempt() {
			return history[i].Attempt() < history[j].Attempt()
		}
		return history[i].ExecutionID().String() < history[j].ExecutionID().String()
	})
}

func (s *FileStore) readLocked() (fileSnapshot, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return fileSnapshot{Version: amber.Version, Records: make(map[string]json.RawMessage)}, nil
	}
	if err != nil {
		return fileSnapshot{}, fmt.Errorf("read amber file store: %w", err)
	}
	var snapshot fileSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return fileSnapshot{}, fmt.Errorf("decode amber file store: %w", err)
	}
	if snapshot.Version != amber.Version {
		return fileSnapshot{}, fmt.Errorf("%w: unsupported file store version %d", amber.ErrInvalidProvenance, snapshot.Version)
	}
	if snapshot.Records == nil {
		snapshot.Records = make(map[string]json.RawMessage)
	}
	return snapshot, nil
}

func (s *FileStore) writeLocked(snapshot fileSnapshot) error {
	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode amber file store: %w", err)
	}
	directory := filepath.Dir(s.path)
	base := filepath.Base(s.path)
	temporary, err := os.CreateTemp(directory, "."+base+".tmp-")
	if err != nil {
		return fmt.Errorf("create amber file store snapshot: %w", err)
	}
	temporaryName := temporary.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(temporaryName)
		}
	}()
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write amber file store snapshot: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync amber file store snapshot: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close amber file store snapshot: %w", err)
	}
	if err := os.Rename(temporaryName, s.path); err != nil {
		return fmt.Errorf("commit amber file store snapshot: %w", err)
	}
	cleanup = false
	return nil
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("%w: context cannot be nil", amber.ErrInvalidTransition)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
