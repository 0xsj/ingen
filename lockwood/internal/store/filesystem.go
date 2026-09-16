package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"ingen/lockwood/internal/artifact"
)

type Filesystem struct {
	root     string
	tempRoot string
}

func NewFilesystem(root string) (*Filesystem, error) {
	if root == "" {
		return nil, fmt.Errorf("store root is required")
	}
	tempRoot := filepath.Join(root, "tmp")
	if err := os.MkdirAll(filepath.Join(root, "blobs", artifact.SHA256Algorithm), 0o755); err != nil {
		return nil, fmt.Errorf("create blob root: %w", err)
	}
	if err := os.MkdirAll(tempRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create temporary root: %w", err)
	}
	return &Filesystem{root: root, tempRoot: tempRoot}, nil
}

func (s *Filesystem) Put(reader io.Reader, options PutOptions) (artifact.Reference, error) {
	if reader == nil {
		return artifact.Reference{}, fmt.Errorf("artifact reader is required")
	}
	if options.MediaType == "" {
		return artifact.Reference{}, fmt.Errorf("media type is required")
	}
	if options.MaxBytes < 0 {
		return artifact.Reference{}, fmt.Errorf("maximum artifact size cannot be negative")
	}
	if options.ExpectedDigest != "" {
		if err := artifact.ValidateDigest(options.ExpectedDigest); err != nil {
			return artifact.Reference{}, err
		}
	}

	temp, err := os.CreateTemp(s.tempRoot, "artifact-")
	if err != nil {
		return artifact.Reference{}, fmt.Errorf("create temporary artifact: %w", err)
	}
	tempName := temp.Name()
	defer func() {
		_ = os.Remove(tempName)
	}()

	hasher := sha256.New()
	input := reader
	if options.MaxBytes > 0 {
		input = io.LimitReader(reader, options.MaxBytes)
	}
	size, err := io.Copy(io.MultiWriter(temp, hasher), input)
	if err != nil {
		_ = temp.Close()
		return artifact.Reference{}, fmt.Errorf("write temporary artifact: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return artifact.Reference{}, fmt.Errorf("sync temporary artifact: %w", err)
	}
	if err := temp.Close(); err != nil {
		return artifact.Reference{}, fmt.Errorf("close temporary artifact: %w", err)
	}
	if options.MaxBytes > 0 && size == options.MaxBytes {
		var extra [1]byte
		n, readErr := io.ReadFull(reader, extra[:])
		if n > 0 {
			return artifact.Reference{}, fmt.Errorf("artifact exceeds maximum size of %d bytes", options.MaxBytes)
		}
		if readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			return artifact.Reference{}, fmt.Errorf("check artifact size: %w", readErr)
		}
	}

	digest := artifact.SHA256Algorithm + ":" + hex.EncodeToString(hasher.Sum(nil))
	if options.ExpectedDigest != "" && options.ExpectedDigest != digest {
		return artifact.Reference{}, fmt.Errorf("expected digest %s, computed %s", options.ExpectedDigest, digest)
	}

	path, err := s.blobPath(digest)
	if err != nil {
		return artifact.Reference{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return artifact.Reference{}, fmt.Errorf("create artifact directory: %w", err)
	}

	if _, err := os.Stat(path); err == nil {
		if err := s.Verify(digest); err != nil {
			return artifact.Reference{}, fmt.Errorf("existing artifact failed verification: %w", err)
		}
		if err := syncDirectory(filepath.Dir(path)); err != nil {
			return artifact.Reference{}, fmt.Errorf("sync artifact directory: %w", err)
		}
		return artifact.Reference{
			Schema:      artifact.Schema,
			Digest:      digest,
			SizeBytes:   size,
			MediaType:   options.MediaType,
			LogicalName: options.LogicalName,
		}, nil
	} else if !os.IsNotExist(err) {
		return artifact.Reference{}, fmt.Errorf("inspect existing artifact: %w", err)
	}

	// The temporary file is created under the same root as the destination.
	// A hard link publishes the name atomically without replacing an existing
	// artifact if another writer wins the race.
	if err := os.Link(tempName, path); err != nil {
		if os.IsExist(err) {
			if verifyErr := s.Verify(digest); verifyErr != nil {
				return artifact.Reference{}, fmt.Errorf("racing artifact failed verification: %w", verifyErr)
			}
		} else {
			return artifact.Reference{}, fmt.Errorf("publish artifact: %w", err)
		}
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return artifact.Reference{}, fmt.Errorf("sync artifact directory: %w", err)
	}

	return artifact.Reference{
		Schema:      artifact.Schema,
		Digest:      digest,
		SizeBytes:   size,
		MediaType:   options.MediaType,
		LogicalName: options.LogicalName,
	}, nil
}

func (s *Filesystem) Get(digest string) ([]byte, error) {
	path, err := s.blobPath(digest)
	if err != nil {
		return nil, err
	}
	if err := s.Verify(digest); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read artifact: %w", err)
	}
	return data, nil
}

func (s *Filesystem) Verify(digest string) error {
	path, err := s.blobPath(digest)
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open artifact: %w", err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return fmt.Errorf("hash artifact: %w", err)
	}
	actual := artifact.SHA256Algorithm + ":" + hex.EncodeToString(hasher.Sum(nil))
	if actual != digest {
		return fmt.Errorf("artifact digest mismatch: got %s, want %s", actual, digest)
	}
	return nil
}

func (s *Filesystem) blobPath(digest string) (string, error) {
	if err := artifact.ValidateDigest(digest); err != nil {
		return "", err
	}
	hexDigest := strings.TrimPrefix(digest, artifact.SHA256Algorithm+":")
	return filepath.Join(s.root, "blobs", artifact.SHA256Algorithm, hexDigest[:2], hexDigest[2:4], hexDigest), nil
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
