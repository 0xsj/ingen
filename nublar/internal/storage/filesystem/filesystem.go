// Package filesystem stores Nublar runs under a local directory.
package filesystem

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ingen/nublar/internal/run"
)

type Store struct {
	Root string
}

func New(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("Nublar filesystem store root must not be empty")
	}
	return &Store{Root: root}, nil
}

func (s *Store) Save(record run.Run) error {
	if s == nil || strings.TrimSpace(s.Root) == "" {
		return fmt.Errorf("Nublar filesystem store root must not be empty")
	}
	if err := record.Validate(); err != nil {
		return fmt.Errorf("validate Nublar run before storage: %w", err)
	}
	var encoded bytes.Buffer
	if err := run.WriteJSON(&encoded, record); err != nil {
		return fmt.Errorf("encode Nublar run for storage: %w", err)
	}
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		return fmt.Errorf("create Nublar store root %s: %w", s.Root, err)
	}
	target := s.pathFor(record.RunID)
	temporary, err := os.CreateTemp(s.Root, ".nublar-run-*")
	if err != nil {
		return fmt.Errorf("create temporary Nublar run in %s: %w", s.Root, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(encoded.Bytes()); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary Nublar run: %w", err)
	}
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set Nublar run permissions: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary Nublar run: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary Nublar run: %w", err)
	}
	if err := os.Link(temporaryPath, target); err != nil {
		return fmt.Errorf("publish Nublar run %q: %w", record.RunID, err)
	}
	return nil
}

func (s *Store) Load(runID string) (run.Run, error) {
	if s == nil || strings.TrimSpace(s.Root) == "" {
		return run.Run{}, fmt.Errorf("Nublar filesystem store root must not be empty")
	}
	if strings.TrimSpace(runID) == "" {
		return run.Run{}, fmt.Errorf("Nublar run ID must not be empty")
	}
	record, err := run.LoadFile(s.pathFor(runID))
	if err != nil {
		return run.Run{}, err
	}
	if record.RunID != runID {
		return run.Run{}, fmt.Errorf("Nublar run storage identity mismatch: loaded %q for %q", record.RunID, runID)
	}
	return record, nil
}

// List returns every canonical run in newest-first order. The returned runs
// have already passed the same validation used by Load.
func (s *Store) List() ([]run.Run, error) {
	if s == nil || strings.TrimSpace(s.Root) == "" {
		return nil, fmt.Errorf("Nublar filesystem store root must not be empty")
	}
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return []run.Run{}, nil
		}
		return nil, fmt.Errorf("read Nublar store root %s: %w", s.Root, err)
	}

	records := make([]run.Run, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !isCanonicalRunFile(entry.Name()) {
			continue
		}
		path := filepath.Join(s.Root, entry.Name())
		record, err := run.LoadFile(path)
		if err != nil {
			return nil, fmt.Errorf("load Nublar run %s: %w", entry.Name(), err)
		}
		if filepath.Base(s.pathFor(record.RunID)) != entry.Name() {
			return nil, fmt.Errorf("Nublar run storage identity mismatch in %s", entry.Name())
		}
		records = append(records, record)
	}

	sort.Slice(records, func(i, j int) bool {
		left, _ := time.Parse(time.RFC3339Nano, records[i].CreatedAt)
		right, _ := time.Parse(time.RFC3339Nano, records[j].CreatedAt)
		if !left.Equal(right) {
			return left.After(right)
		}
		return records[i].RunID < records[j].RunID
	})
	return records, nil
}

func isCanonicalRunFile(name string) bool {
	if !strings.HasSuffix(name, ".json") {
		return false
	}
	encoded := strings.TrimSuffix(name, ".json")
	if len(encoded) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(encoded)
	return err == nil
}

func (s *Store) pathFor(runID string) string {
	digest := sha256.Sum256([]byte(runID))
	return filepath.Join(s.Root, hex.EncodeToString(digest[:])+".json")
}
