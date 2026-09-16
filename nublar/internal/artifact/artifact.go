// Package artifact loads shared CI result envelopes with byte-level
// provenance. The artifact is decoded and hashed from one read of the file.
package artifact

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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

	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var result ciresult.Artifact
	if err := decoder.Decode(&result); err != nil {
		return Loaded{}, fmt.Errorf("parse CI result %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Loaded{}, fmt.Errorf("parse CI result %s: multiple JSON values are not supported", path)
		}
		return Loaded{}, fmt.Errorf("parse CI result %s: %w", path, err)
	}
	if err := result.Validate(); err != nil {
		return Loaded{}, fmt.Errorf("validate CI result %s: %w", path, err)
	}
	digest := sha256.Sum256(contents)
	return Loaded{Artifact: result, SHA256: hex.EncodeToString(digest[:])}, nil
}
