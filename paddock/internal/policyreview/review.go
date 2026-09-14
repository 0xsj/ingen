package policyreview

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"ingen/paddock/internal/policydiff"
)

const Schema = "paddock.policy-review/v1"

type Document struct {
	Schema string              `json:"schema"`
	Status string              `json:"status"`
	Diff   policydiff.Document `json:"diff"`
}

func New(diff policydiff.Document) Document {
	status := "PASS"
	if diff.Tests == nil || diff.Tests.Status != "PASS" {
		status = "FAIL"
	}
	return Document{Schema: Schema, Status: status, Diff: diff}
}

func (d Document) Validate() error {
	if d.Schema != Schema {
		return fmt.Errorf("policy review schema must be %s, got %q", Schema, d.Schema)
	}
	if d.Status != "PASS" && d.Status != "FAIL" {
		return fmt.Errorf("policy review has unsupported status %q", d.Status)
	}
	if d.Diff.Schema != policydiff.Schema {
		return fmt.Errorf("policy review contains unsupported diff schema %q", d.Diff.Schema)
	}
	if d.Diff.Tests == nil {
		return fmt.Errorf("policy review requires policy test results")
	}
	if d.Diff.Tests.Manifest.Path == "" || d.Diff.Tests.Manifest.SHA256 == "" {
		return fmt.Errorf("policy review test manifest provenance is required")
	}
	wantStatus := "PASS"
	if d.Diff.Tests.Status != "PASS" {
		wantStatus = "FAIL"
	}
	if d.Status != wantStatus {
		return fmt.Errorf("policy review status does not match policy test status")
	}
	return nil
}

func Save(path string, document Document) error {
	if err := document.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("encode policy review: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write policy review: %w", err)
	}
	return nil
}

func Load(path string) (Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Document{}, fmt.Errorf("read policy review: %w", err)
	}
	var document Document
	if err := json.Unmarshal(data, &document); err != nil {
		return Document{}, fmt.Errorf("parse policy review: %w", err)
	}
	if err := document.Validate(); err != nil {
		return Document{}, err
	}
	return document, nil
}

func VerifyFiles(document Document) error {
	if err := document.Validate(); err != nil {
		return err
	}
	inputs := []struct {
		name string
		path string
		want string
	}{
		{name: "before policy", path: document.Diff.Before.Path, want: document.Diff.Before.SHA256},
		{name: "after policy", path: document.Diff.After.Path, want: document.Diff.After.SHA256},
		{name: "test manifest", path: document.Diff.Tests.Manifest.Path, want: document.Diff.Tests.Manifest.SHA256},
	}
	for _, input := range inputs {
		data, err := os.ReadFile(input.path)
		if err != nil {
			return fmt.Errorf("read %s %q: %w", input.name, input.path, err)
		}
		digest := sha256.Sum256(data)
		actual := hex.EncodeToString(digest[:])
		if actual != input.want {
			return fmt.Errorf("%s %q hash mismatch: expected %s, got %s", input.name, input.path, input.want, actual)
		}
	}
	return nil
}

func Text(w io.Writer, document Document) error {
	if err := document.Validate(); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "POLICY-REVIEW %s (%d changes)\n", document.Status, document.Diff.Summary.Total); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  before: %s\n  after:  %s\n", document.Diff.Before.Path, document.Diff.After.Path); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  tests: %s (%d/%d passed)\n", document.Diff.Tests.Status, document.Diff.Tests.Passed, len(document.Diff.Tests.Cases)); err != nil {
		return err
	}
	return nil
}

func JSON(w io.Writer, document Document) error {
	if err := document.Validate(); err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(document)
}
