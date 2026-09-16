package store

import (
	"io"

	"ingen/lockwood/internal/artifact"
)

type PutOptions struct {
	ExpectedDigest string
	MediaType      string
	LogicalName    string
	MaxBytes       int64
}

type Store interface {
	Put(reader io.Reader, options PutOptions) (artifact.Reference, error)
	Get(digest string) ([]byte, error)
	Verify(digest string) error
}
