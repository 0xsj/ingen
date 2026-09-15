// Package artifact loads shared CI result envelopes with byte-level
// provenance. The artifact is decoded and hashed from one read of the file.
package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"ingen/core/ciresult"
)

type Loaded struct {
	Artifact ciresult.Artifact
	SHA256   string
}

func LoadFile(path string) (Loaded, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Loaded{}, fmt.Errorf("read CI result %s: %w", path, err)
	}

	var result ciresult.Artifact
	if err := json.Unmarshal(contents, &result); err != nil {
		return Loaded{}, fmt.Errorf("parse CI result %s: %w", path, err)
	}
	if err := result.Validate(); err != nil {
		return Loaded{}, fmt.Errorf("validate CI result %s: %w", path, err)
	}
	digest := sha256.Sum256(contents)
	return Loaded{Artifact: result, SHA256: hex.EncodeToString(digest[:])}, nil
}
