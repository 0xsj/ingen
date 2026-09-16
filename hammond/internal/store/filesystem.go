package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"ingen/hammond/internal/governance"
)

type FileStore struct {
	root string
	mu   sync.Mutex
}

func NewFileStore(root string) (*FileStore, error) {
	if root == "" {
		return nil, fmt.Errorf("Hammond store root is required")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create Hammond store root: %w", err)
	}
	return &FileStore{root: root}, nil
}

func (s *FileStore) Register(record governance.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := validateRegistration(record); err != nil {
		return err
	}
	path := s.pathFor(record.Contract.Identity())
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("register contract identity %s: already exists", record.Contract.Identity().Key())
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check Hammond record %s: %w", path, err)
	}
	return s.write(record)
}

func (s *FileStore) AppendEvent(identity governance.ContractIdentity, event governance.Event) (governance.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.get(identity)
	if err != nil {
		return governance.Record{}, err
	}
	if !record.Contract.Identity().Equal(identity) {
		return governance.Record{}, fmt.Errorf("stored contract identity does not match requested identity")
	}
	for _, existing := range record.Events {
		if existing.ID == event.ID {
			return governance.Record{}, fmt.Errorf("event %q already exists", event.ID)
		}
	}
	if event.Type == governance.EventAmendmentCreated || event.Type == governance.EventSuperseded {
		return governance.Record{}, fmt.Errorf("%s must use its coordinated store operation", event.Type)
	}

	updated, err := record.AppendEvent(event)
	if err != nil {
		return governance.Record{}, err
	}
	if err := s.write(updated); err != nil {
		return governance.Record{}, err
	}
	return updated, nil
}

// Supersede marks an approved predecessor as superseded after its approved
// successor has been linked by an amendment event.
func (s *FileStore) Supersede(identity governance.ContractIdentity, successor governance.ContractIdentity, event governance.Event) (governance.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	predecessor, err := s.get(identity)
	if err != nil {
		return governance.Record{}, err
	}
	storedSuccessor, err := s.get(successor)
	if err != nil {
		return governance.Record{}, err
	}
	if storedSuccessor.State != governance.StateApproved {
		return governance.Record{}, fmt.Errorf("successor must be approved before predecessor is superseded")
	}
	if event.Type != governance.EventSuperseded || event.Successor == nil || !event.Successor.Equal(successor) {
		return governance.Record{}, fmt.Errorf("superseded event successor does not match stored successor")
	}
	if !predecessor.HasAmendmentLink(successor) {
		return governance.Record{}, fmt.Errorf("predecessor has no amendment link to successor")
	}
	updated, err := predecessor.AppendEvent(event)
	if err != nil {
		return governance.Record{}, err
	}
	if err := governance.ValidateLineage([]governance.Record{updated, storedSuccessor}); err != nil {
		return governance.Record{}, fmt.Errorf("validate supersession lineage: %w", err)
	}
	if err := s.write(updated); err != nil {
		return governance.Record{}, err
	}
	return updated, nil
}

// CreateAmendment publishes a registered successor and the predecessor's
// amendment event together. If publishing the predecessor snapshot fails, the
// newly created successor is removed and the original predecessor is restored.
func (s *FileStore) CreateAmendment(identity governance.ContractIdentity, successor governance.Record, event governance.Event) (governance.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	predecessor, err := s.get(identity)
	if err != nil {
		return governance.Record{}, err
	}
	if err := validateRegistration(successor); err != nil {
		return governance.Record{}, fmt.Errorf("validate successor registration: %w", err)
	}
	if event.Type != governance.EventAmendmentCreated || event.Predecessor == nil || event.Successor == nil {
		return governance.Record{}, fmt.Errorf("amendment event must contain predecessor and successor identities")
	}
	if !event.Predecessor.Equal(predecessor.Contract.Identity()) {
		return governance.Record{}, fmt.Errorf("amendment event predecessor does not match stored record")
	}
	if !event.Successor.Equal(successor.Contract.Identity()) {
		return governance.Record{}, fmt.Errorf("amendment event successor does not match successor record")
	}
	updatedPredecessor, err := predecessor.AppendEvent(event)
	if err != nil {
		return governance.Record{}, err
	}
	if err := governance.ValidateLineage([]governance.Record{updatedPredecessor, successor}); err != nil {
		return governance.Record{}, fmt.Errorf("validate amendment lineage: %w", err)
	}

	predecessorPath := s.pathFor(identity)
	successorPath := s.pathFor(successor.Contract.Identity())
	if _, err := os.Stat(successorPath); err == nil {
		return governance.Record{}, fmt.Errorf("successor contract identity %s already exists", successor.Contract.Identity().Key())
	} else if !os.IsNotExist(err) {
		return governance.Record{}, fmt.Errorf("check successor record %s: %w", successorPath, err)
	}
	originalPredecessor, err := os.ReadFile(predecessorPath)
	if err != nil {
		return governance.Record{}, fmt.Errorf("read predecessor record %s: %w", predecessorPath, err)
	}
	successorData, err := marshalRecord(successor)
	if err != nil {
		return governance.Record{}, err
	}
	updatedPredecessorData, err := marshalRecord(updatedPredecessor)
	if err != nil {
		return governance.Record{}, err
	}
	if err := writeBytes(successorPath, successorData); err != nil {
		return governance.Record{}, fmt.Errorf("publish successor record: %w", err)
	}
	if err := writeBytes(predecessorPath, updatedPredecessorData); err != nil {
		_ = os.Remove(successorPath)
		restoreErr := writeBytes(predecessorPath, originalPredecessor)
		if restoreErr != nil {
			return governance.Record{}, fmt.Errorf("publish predecessor record: %w; restore failed: %v", err, restoreErr)
		}
		return governance.Record{}, fmt.Errorf("publish predecessor record: %w", err)
	}
	return updatedPredecessor, nil
}

func (s *FileStore) Get(identity governance.ContractIdentity) (governance.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.get(identity)
}

func (s *FileStore) List() ([]governance.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, fmt.Errorf("list Hammond store: %w", err)
	}
	records := make([]governance.Record, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		record, err := readRecord(filepath.Join(s.root, entry.Name()))
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].Contract.Identity().Key() < records[j].Contract.Identity().Key()
	})
	return records, nil
}

func (s *FileStore) get(identity governance.ContractIdentity) (governance.Record, error) {
	path := s.pathFor(identity)
	record, err := readRecord(path)
	if err != nil {
		if os.IsNotExist(err) {
			return governance.Record{}, fmt.Errorf("%w: %s", ErrNotFound, identity.Key())
		}
		return governance.Record{}, err
	}
	if !record.Contract.Identity().Equal(identity) {
		return governance.Record{}, fmt.Errorf("Hammond record %s contains a different contract identity", path)
	}
	return record, nil
}

func (s *FileStore) write(record governance.Record) error {
	data, err := marshalRecord(record)
	if err != nil {
		return err
	}
	return writeBytes(s.pathFor(record.Contract.Identity()), data)
}

func marshalRecord(record governance.Record) ([]byte, error) {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode Hammond record: %w", err)
	}
	return append(data, '\n'), nil
}

func writeBytes(path string, data []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".hammond-record-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary Hammond record: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set Hammond record permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary Hammond record: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary Hammond record: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary Hammond record: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish Hammond record: %w", err)
	}
	return nil
}

func (s *FileStore) pathFor(identity governance.ContractIdentity) string {
	digest := sha256.Sum256([]byte(identity.Key()))
	return filepath.Join(s.root, hex.EncodeToString(digest[:])+".json")
}

func readRecord(path string) (governance.Record, error) {
	file, err := os.Open(path)
	if err != nil {
		return governance.Record{}, err
	}
	defer file.Close()
	contents, err := io.ReadAll(file)
	if err != nil {
		return governance.Record{}, fmt.Errorf("read Hammond record %s: %w", path, err)
	}
	record, err := governance.DecodeRecord(contents)
	if err != nil {
		return governance.Record{}, fmt.Errorf("parse Hammond record %s: %w", path, err)
	}
	return record, nil
}

func validateRegistration(record governance.Record) error {
	if _, err := governance.LoadContractArtifact(record.Contract); err != nil {
		return fmt.Errorf("load registration contract: %w", err)
	}
	policy, err := governance.LoadReviewPolicy(record.Policy)
	if err != nil {
		return fmt.Errorf("load registration policy: %w", err)
	}
	if err := record.ValidateWithPolicy(policy); err != nil {
		return err
	}
	if record.State != governance.StateRegistered || len(record.Events) != 1 || record.Events[0].Type != governance.EventRegistered {
		return fmt.Errorf("Hammond registration must contain exactly one registered event")
	}
	return nil
}
