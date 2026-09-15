package store

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"ingen/lockwood/internal/artifact"
)

// BlobInfo describes a blob discovered in the content-addressed store. It is
// inventory metadata only; callers should use Verify or VerifyReference before
// treating the bytes as trustworthy.
type BlobInfo struct {
	Digest     string    `json:"digest"`
	SizeBytes  int64     `json:"size_bytes"`
	ModifiedAt time.Time `json:"modified_at"`
}

// ListBlobs inventories published blobs without scanning the temporary-write
// directory. Unexpected paths fail closed so reconciliation cannot silently
// ignore malformed or misplaced data.
func (s *Filesystem) ListBlobs() ([]BlobInfo, error) {
	root := filepath.Join(s.root, "blobs", artifact.SHA256Algorithm)
	firstLevel, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("list artifact blobs: %w", err)
	}
	blobs := make([]BlobInfo, 0)
	for _, first := range firstLevel {
		if !first.IsDir() || !isLowerHexComponent(first.Name()) {
			return nil, fmt.Errorf("unexpected artifact blob directory: %s", first.Name())
		}
		secondLevel, err := os.ReadDir(filepath.Join(root, first.Name()))
		if err != nil {
			return nil, fmt.Errorf("list artifact blob partition %s: %w", first.Name(), err)
		}
		for _, second := range secondLevel {
			if !second.IsDir() || !isLowerHexComponent(second.Name()) {
				return nil, fmt.Errorf("unexpected artifact blob partition: %s/%s", first.Name(), second.Name())
			}
			files, err := os.ReadDir(filepath.Join(root, first.Name(), second.Name()))
			if err != nil {
				return nil, fmt.Errorf("list artifact blob files %s/%s: %w", first.Name(), second.Name(), err)
			}
			for _, file := range files {
				if file.IsDir() || file.Type()&os.ModeSymlink != 0 {
					return nil, fmt.Errorf("unexpected artifact blob entry: %s/%s/%s", first.Name(), second.Name(), file.Name())
				}
				if len(file.Name()) != 64 || file.Name()[:2] != first.Name() || file.Name()[2:4] != second.Name() {
					return nil, fmt.Errorf("unexpected artifact blob name: %s/%s/%s", first.Name(), second.Name(), file.Name())
				}
				digest := artifact.SHA256Algorithm + ":" + file.Name()
				if err := artifact.ValidateDigest(digest); err != nil {
					return nil, fmt.Errorf("invalid artifact blob name %s: %w", file.Name(), err)
				}
				info, err := file.Info()
				if err != nil {
					return nil, fmt.Errorf("inspect artifact blob %s: %w", digest, err)
				}
				if !info.Mode().IsRegular() {
					return nil, fmt.Errorf("artifact blob is not a regular file: %s", digest)
				}
				blobs = append(blobs, BlobInfo{
					Digest:     digest,
					SizeBytes:  info.Size(),
					ModifiedAt: info.ModTime().UTC(),
				})
			}
		}
	}
	return blobs, nil
}

func isLowerHexComponent(value string) bool {
	if len(value) != 2 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
