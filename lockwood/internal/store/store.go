package store

import (
	"io"

	"ingen/lockwood/internal/artifact"
)

// PutOptions controls artifact publication. A zero MaxBytes means unlimited;
// positive limits are enforced before a backend publishes the blob.
type PutOptions struct {
	ExpectedDigest string
	MediaType      string
	LogicalName    string
	MaxBytes       int64
}

// Store is the content-addressed artifact backend contract. Implementations
// must preserve digest identity, reject mismatched expected digests, and
// verify both raw blobs and complete artifact references.
type Store interface {
	Put(reader io.Reader, options PutOptions) (artifact.Reference, error)
	Get(digest string) ([]byte, error)
	Verify(digest string) error
	VerifyReference(reference artifact.Reference) error
}
