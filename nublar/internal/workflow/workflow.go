// Package workflow defines Nublar's declarative result-collection workflow.
package workflow

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"ingen/core/ciresult"
)

const Schema = "ingen.nublar-workflow/v1"

type Document struct {
	Schema string  `yaml:"schema" json:"schema"`
	ID     string  `yaml:"id" json:"id,omitempty"`
	Checks []Check `yaml:"checks" json:"checks"`
}

type Check struct {
	ID       string `yaml:"id" json:"id"`
	Tool     string `yaml:"tool" json:"tool"`
	Result   string `yaml:"result" json:"result"`
	Required *bool  `yaml:"required,omitempty" json:"required,omitempty"`
}

func (c Check) IsRequired() bool {
	return c.Required == nil || *c.Required
}

func LoadFile(path string) (Document, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Document{}, fmt.Errorf("read Nublar workflow %s: %w", path, err)
	}
	return loadBytes(path, contents)
}

// LoadFileWithReference parses and hashes one read of the workflow file. The
// reference therefore identifies the bytes that were actually validated.
func LoadFileWithReference(path string) (Document, ciresult.FileRef, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Document{}, ciresult.FileRef{Path: path}, fmt.Errorf("read Nublar workflow %s: %w", path, err)
	}
	document, err := loadBytes(path, contents)
	if err != nil {
		return Document{}, ciresult.FileRef{Path: path}, err
	}
	return document, FileReferenceFromBytes(path, contents), nil
}

func loadBytes(path string, contents []byte) (Document, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	decoder.KnownFields(true)
	var document Document
	if err := decoder.Decode(&document); err != nil {
		return Document{}, fmt.Errorf("parse Nublar workflow %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Document{}, fmt.Errorf("parse Nublar workflow %s: multiple documents are not supported", path)
		}
		return Document{}, fmt.Errorf("parse Nublar workflow %s: %w", path, err)
	}
	if err := Validate(document); err != nil {
		return Document{}, fmt.Errorf("validate Nublar workflow %s: %w", path, err)
	}
	return document, nil
}

func Validate(document Document) error {
	if document.Schema != Schema {
		return fmt.Errorf("workflow schema must be %s, got %q", Schema, document.Schema)
	}
	if strings.TrimSpace(document.ID) == "" {
		return fmt.Errorf("workflow id must not be empty")
	}
	if len(document.Checks) == 0 {
		return fmt.Errorf("workflow needs at least one check")
	}
	seen := make(map[string]bool, len(document.Checks))
	seenPaths := make(map[string]bool, len(document.Checks))
	for index, check := range document.Checks {
		if strings.TrimSpace(check.ID) == "" {
			return fmt.Errorf("workflow check %d needs an id", index+1)
		}
		if seen[check.ID] {
			return fmt.Errorf("workflow check ID %q was duplicated", check.ID)
		}
		seen[check.ID] = true
		if strings.TrimSpace(check.Tool) == "" {
			return fmt.Errorf("workflow check %q needs a tool", check.ID)
		}
		if err := validateRelativePath(check.Result); err != nil {
			return fmt.Errorf("workflow check %q result: %w", check.ID, err)
		}
		normalizedPath := filepath.Clean(check.Result)
		if seenPaths[normalizedPath] {
			return fmt.Errorf("workflow result path %q was duplicated", check.Result)
		}
		seenPaths[normalizedPath] = true
	}
	return nil
}

// FileReference identifies the exact workflow bytes used by a coordinator.
// This is raw-file provenance; it is not yet a semantic workflow seal.
func FileReference(path string) (ciresult.FileRef, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return ciresult.FileRef{Path: path}, fmt.Errorf("hash Nublar workflow %s: %w", path, err)
	}
	return FileReferenceFromBytes(path, contents), nil
}

func FileReferenceFromBytes(path string, contents []byte) ciresult.FileRef {
	digest := sha256.Sum256(contents)
	return ciresult.FileRef{Path: path, SHA256: hex.EncodeToString(digest[:])}
}

func validateRelativePath(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("result path must not be empty")
	}
	clean := filepath.Clean(raw)
	if filepath.IsAbs(raw) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("result path must stay inside the workflow root: %q", raw)
	}
	return nil
}
