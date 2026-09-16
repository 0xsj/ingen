package integrity

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"ingen/lockwood/internal/artifact"
)

// Result is the digest and byte count observed while reading an artifact.
type Result struct {
	Digest    string
	SizeBytes int64
}

// Copy streams src to dst while computing its SHA-256 digest. A positive
// maxBytes rejects input after the allowed number of bytes and before the
// caller can publish the destination. Callers should discard dst on error.
func Copy(dst io.Writer, src io.Reader, maxBytes int64) (Result, error) {
	if dst == nil {
		return Result{}, fmt.Errorf("integrity destination is required")
	}
	if src == nil {
		return Result{}, fmt.Errorf("integrity source is required")
	}
	if maxBytes < 0 {
		return Result{}, fmt.Errorf("maximum artifact size cannot be negative")
	}

	hasher := sha256.New()
	input := src
	if maxBytes > 0 {
		input = io.LimitReader(src, maxBytes)
	}
	size, err := io.Copy(io.MultiWriter(dst, hasher), input)
	if err != nil {
		return Result{}, fmt.Errorf("copy integrity input: %w", err)
	}
	if maxBytes > 0 && size == maxBytes {
		var extra [1]byte
		n, readErr := io.ReadFull(src, extra[:])
		if n > 0 {
			return Result{}, fmt.Errorf("artifact exceeds maximum size of %d bytes", maxBytes)
		}
		if readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			return Result{}, fmt.Errorf("check artifact size: %w", readErr)
		}
	}

	return Result{
		Digest:    artifact.SHA256Algorithm + ":" + hex.EncodeToString(hasher.Sum(nil)),
		SizeBytes: size,
	}, nil
}

// ReadAll reads and verifies a bounded artifact into memory. It is intended
// for adapters that must parse or preserve the complete payload.
func ReadAll(src io.Reader, maxBytes int64) ([]byte, Result, error) {
	var buffer bytes.Buffer
	result, err := Copy(&buffer, src, maxBytes)
	if err != nil {
		return nil, Result{}, err
	}
	return buffer.Bytes(), result, nil
}

// VerifyDigest checks an optional expected digest against a computed digest.
func VerifyDigest(actual, expected string) error {
	if expected == "" {
		return nil
	}
	if err := artifact.ValidateDigest(expected); err != nil {
		return err
	}
	if actual != expected {
		return fmt.Errorf("expected digest %s, computed %s", expected, actual)
	}
	return nil
}

// VerifyBytes computes a SHA-256 digest and checks it against expected.
func VerifyBytes(data []byte, expected string) error {
	return VerifyDigest(artifact.DigestBytes(data), expected)
}
