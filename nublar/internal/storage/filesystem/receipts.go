package filesystem

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ingen/nublar/internal/delivery"
)

// ReceiptStore persists immutable delivery receipts independently from run
// records. Receipt filenames are content-addressed so an altered receipt is
// detected while listing the store.
type ReceiptStore struct {
	Root string
}

func NewReceiptStore(root string) (*ReceiptStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("Nublar receipt store root must not be empty")
	}
	return &ReceiptStore{Root: root}, nil
}

// Save validates and publishes one receipt exactly once. Re-saving identical
// receipt bytes is rejected, preserving the immutable attempt history.
func (s *ReceiptStore) Save(receipt delivery.Receipt) error {
	if s == nil || strings.TrimSpace(s.Root) == "" {
		return fmt.Errorf("Nublar receipt store root must not be empty")
	}
	encoded, err := encodeReceipt(receipt)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		return fmt.Errorf("create Nublar receipt store root %s: %w", s.Root, err)
	}
	target := s.pathFor(encoded)
	temporary, err := os.CreateTemp(s.Root, ".nublar-receipt-*")
	if err != nil {
		return fmt.Errorf("create temporary Nublar receipt in %s: %w", s.Root, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(encoded); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary Nublar receipt: %w", err)
	}
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set Nublar receipt permissions: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary Nublar receipt: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary Nublar receipt: %w", err)
	}
	if err := os.Link(temporaryPath, target); err != nil {
		return fmt.Errorf("publish Nublar receipt: %w", err)
	}
	return nil
}

// List returns every canonical receipt in newest-first order. Each returned
// receipt has passed strict JSON decoding and structural validation.
func (s *ReceiptStore) List() ([]delivery.Receipt, error) {
	if s == nil || strings.TrimSpace(s.Root) == "" {
		return nil, fmt.Errorf("Nublar receipt store root must not be empty")
	}
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return []delivery.Receipt{}, nil
		}
		return nil, fmt.Errorf("read Nublar receipt store root %s: %w", s.Root, err)
	}
	items := make([]receiptEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !isCanonicalReceiptFile(entry.Name()) {
			continue
		}
		path := filepath.Join(s.Root, entry.Name())
		contents, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read Nublar receipt %s: %w", entry.Name(), err)
		}
		if filepath.Base(s.pathFor(contents)) != entry.Name() {
			return nil, fmt.Errorf("Nublar receipt content hash mismatch in %s", entry.Name())
		}
		receipt, err := decodeReceipt(path, contents)
		if err != nil {
			return nil, err
		}
		items = append(items, receiptEntry{name: entry.Name(), receipt: receipt})
	}
	sort.Slice(items, func(i, j int) bool {
		left, _ := time.Parse(time.RFC3339Nano, items[i].receipt.AttemptedAt)
		right, _ := time.Parse(time.RFC3339Nano, items[j].receipt.AttemptedAt)
		if !left.Equal(right) {
			return left.After(right)
		}
		return items[i].name < items[j].name
	})
	receipts := make([]delivery.Receipt, 0, len(items))
	for _, item := range items {
		receipts = append(receipts, item.receipt)
	}
	return receipts, nil
}

type receiptEntry struct {
	name    string
	receipt delivery.Receipt
}

func encodeReceipt(receipt delivery.Receipt) ([]byte, error) {
	var encoded bytes.Buffer
	if err := delivery.WriteReceiptJSON(&encoded, receipt); err != nil {
		return nil, fmt.Errorf("encode Nublar receipt for storage: %w", err)
	}
	return encoded.Bytes(), nil
}

func decodeReceipt(path string, contents []byte) (delivery.Receipt, error) {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var receipt delivery.Receipt
	if err := decoder.Decode(&receipt); err != nil {
		return delivery.Receipt{}, fmt.Errorf("parse Nublar receipt %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return delivery.Receipt{}, fmt.Errorf("parse Nublar receipt %s: multiple JSON values are not supported", path)
		}
		return delivery.Receipt{}, fmt.Errorf("parse Nublar receipt %s: %w", path, err)
	}
	if err := receipt.Validate(); err != nil {
		return delivery.Receipt{}, fmt.Errorf("validate Nublar receipt %s: %w", path, err)
	}
	return receipt, nil
}

func isCanonicalReceiptFile(name string) bool {
	if !strings.HasPrefix(name, "receipt-") || !strings.HasSuffix(name, ".json") {
		return false
	}
	encoded := strings.TrimSuffix(strings.TrimPrefix(name, "receipt-"), ".json")
	if len(encoded) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(encoded)
	return err == nil
}

func (s *ReceiptStore) pathFor(contents []byte) string {
	digest := sha256.Sum256(contents)
	return filepath.Join(s.Root, "receipt-"+hex.EncodeToString(digest[:])+".json")
}
