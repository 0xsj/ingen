package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
)

const (
	Schema          = "lockwood.artifact/v1"
	SHA256Algorithm = "sha256"
)

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// Reference identifies immutable artifact bytes and their descriptive metadata.
// The metadata does not participate in the content digest.
type Reference struct {
	Schema      string `json:"schema"`
	Digest      string `json:"digest"`
	SizeBytes   int64  `json:"size_bytes"`
	MediaType   string `json:"media_type"`
	LogicalName string `json:"logical_name,omitempty"`
}

func DigestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return SHA256Algorithm + ":" + hex.EncodeToString(sum[:])
}

func ValidateDigest(digest string) error {
	if !digestPattern.MatchString(digest) {
		return fmt.Errorf("invalid artifact digest %q", digest)
	}
	return nil
}

func ReferenceFor(data []byte, mediaType, logicalName string) (Reference, error) {
	if mediaType == "" {
		return Reference{}, fmt.Errorf("media type is required")
	}
	return Reference{
		Schema:      Schema,
		Digest:      DigestBytes(data),
		SizeBytes:   int64(len(data)),
		MediaType:   mediaType,
		LogicalName: logicalName,
	}, nil
}
