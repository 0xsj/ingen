package custody

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Filesystem struct {
	root     string
	tempRoot string
}

func NewFilesystem(root string) (*Filesystem, error) {
	if root == "" {
		return nil, fmt.Errorf("custody root is required")
	}
	if err := os.MkdirAll(filepath.Join(root, "records"), 0o755); err != nil {
		return nil, fmt.Errorf("create custody record root: %w", err)
	}
	tempRoot := filepath.Join(root, "tmp")
	if err := os.MkdirAll(tempRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create custody temporary root: %w", err)
	}
	return &Filesystem{root: root, tempRoot: tempRoot}, nil
}

func (s *Filesystem) Put(record Record) error {
	record = normalizeRecord(record)
	encoded, err := MarshalCanonical(record)
	if err != nil {
		return fmt.Errorf("encode custody record: %w", err)
	}
	path := filepath.Join(s.root, "records", record.CustodyID+".json")

	if existing, err := os.ReadFile(path); err == nil {
		if bytes.Equal(existing, encoded) {
			if err := syncDirectory(filepath.Dir(path)); err != nil {
				return fmt.Errorf("sync custody record directory: %w", err)
			}
			return nil
		}
		return fmt.Errorf("custody record %q already exists with different contents", record.CustodyID)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect existing custody record: %w", err)
	}

	temp, err := os.CreateTemp(s.tempRoot, "record-")
	if err != nil {
		return fmt.Errorf("create temporary custody record: %w", err)
	}
	tempName := temp.Name()
	defer func() {
		_ = os.Remove(tempName)
	}()
	if _, err := temp.Write(encoded); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write temporary custody record: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync temporary custody record: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary custody record: %w", err)
	}

	if err := os.Link(tempName, path); err != nil {
		if !os.IsExist(err) {
			return fmt.Errorf("publish custody record: %w", err)
		}
		existing, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read racing custody record: %w", readErr)
		}
		if !bytes.Equal(existing, encoded) {
			return fmt.Errorf("custody record %q already exists with different contents", record.CustodyID)
		}
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sync custody record directory: %w", err)
	}
	return nil
}

func (s *Filesystem) Get(custodyID string) (Record, error) {
	if !custodyIDPattern.MatchString(custodyID) {
		return Record{}, fmt.Errorf("invalid custody id %q", custodyID)
	}
	path := filepath.Join(s.root, "records", custodyID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Record{}, fmt.Errorf("read custody record: %w", err)
	}
	var record Record
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return Record{}, fmt.Errorf("decode custody record: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return Record{}, fmt.Errorf("decode custody record: %w", err)
	}
	if err := record.Validate(); err != nil {
		return Record{}, fmt.Errorf("validate custody record: %w", err)
	}
	canonical, err := MarshalCanonical(record)
	if err != nil {
		return Record{}, fmt.Errorf("encode canonical custody record: %w", err)
	}
	if !bytes.Equal(data, canonical) {
		return Record{}, fmt.Errorf("custody record is not canonical JSON")
	}
	if record.CustodyID != custodyID {
		return Record{}, fmt.Errorf("custody record id %q does not match requested id %q", record.CustodyID, custodyID)
	}
	return record, nil
}

func (s *Filesystem) List() ([]Record, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, "records"))
	if err != nil {
		return nil, fmt.Errorf("list custody records: %w", err)
	}
	records := make([]Record, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			return nil, fmt.Errorf("unexpected directory in custody records: %s", entry.Name())
		}
		if !strings.HasSuffix(entry.Name(), ".json") {
			return nil, fmt.Errorf("unexpected custody record file: %s", entry.Name())
		}
		custodyID := strings.TrimSuffix(entry.Name(), ".json")
		record, err := s.Get(custodyID)
		if err != nil {
			return nil, fmt.Errorf("load custody record %s: %w", custodyID, err)
		}
		records = append(records, record)
	}
	return records, nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	if err := directory.Sync(); err != nil {
		_ = directory.Close()
		return err
	}
	return directory.Close()
}
