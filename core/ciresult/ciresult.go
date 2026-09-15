// Package ciresult defines the shared InGen CI result envelope.
//
// The envelope deliberately keeps producer reports opaque. A coordinator can
// combine results from Sorna, Paddock, and future tools by using the common
// status and exit-code fields without importing each producer's verifier.
package ciresult

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const Schema = "ingen.ci-result/v1"

type FileRef struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256,omitempty"`
}

type Source struct {
	Root       string `json:"root"`
	ModulePath string `json:"module_path,omitempty"`
}

// Artifact is the language-neutral result envelope. Report and Explanation
// belong to the producer and are therefore represented as raw JSON.
type Artifact struct {
	Schema      string             `json:"schema"`
	Tool        string             `json:"tool"`
	Kind        string             `json:"kind"`
	Status      string             `json:"status"`
	ExitCode    int                `json:"exit_code"`
	CreatedAt   string             `json:"created_at"`
	Source      Source             `json:"source"`
	Policy      *FileRef           `json:"policy,omitempty"`
	PolicyLock  *FileRef           `json:"policy_lock,omitempty"`
	Graph       *FileRef           `json:"graph,omitempty"`
	Baseline    *FileRef           `json:"baseline,omitempty"`
	Inputs      map[string]FileRef `json:"inputs,omitempty"`
	Report      json.RawMessage    `json:"report,omitempty"`
	Explanation json.RawMessage    `json:"explanation,omitempty"`
	Error       string             `json:"error,omitempty"`
}

func (a Artifact) Validate() error {
	if a.Schema != Schema {
		return fmt.Errorf("CI result schema must be %s, got %q", Schema, a.Schema)
	}
	if strings.TrimSpace(a.Tool) == "" || strings.TrimSpace(a.Kind) == "" {
		return fmt.Errorf("CI result tool and kind are required")
	}
	if strings.TrimSpace(a.CreatedAt) == "" {
		return fmt.Errorf("CI result created_at is required")
	}
	if _, err := time.Parse(time.RFC3339Nano, a.CreatedAt); err != nil {
		return fmt.Errorf("CI result created_at must be RFC3339: %w", err)
	}
	if strings.TrimSpace(a.Source.Root) == "" {
		return fmt.Errorf("CI result source.root is required")
	}
	if err := validateFileRef("policy", a.Policy); err != nil {
		return err
	}
	if err := validateFileRef("policy_lock", a.PolicyLock); err != nil {
		return err
	}
	if err := validateFileRef("graph", a.Graph); err != nil {
		return err
	}
	if err := validateFileRef("baseline", a.Baseline); err != nil {
		return err
	}
	for name, ref := range a.Inputs {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("CI result input name must not be empty")
		}
		if err := validateFileRef(name, &ref); err != nil {
			return err
		}
	}
	switch a.Status {
	case "passed":
		if a.ExitCode != 0 {
			return fmt.Errorf("passed CI result must have exit_code 0")
		}
	case "failed":
		if a.ExitCode != 1 {
			return fmt.Errorf("failed CI result must have exit_code 1")
		}
	case "error":
		if a.ExitCode != 2 || strings.TrimSpace(a.Error) == "" {
			return fmt.Errorf("error CI results need exit_code 2 and an error")
		}
	default:
		return fmt.Errorf("CI result has unsupported status %q", a.Status)
	}
	if a.Status != "error" {
		if !validJSONValue(a.Report) || !validJSONValue(a.Explanation) {
			return fmt.Errorf("non-error CI results need valid report and explanation JSON")
		}
	}
	return nil
}

func validateFileRef(name string, ref *FileRef) error {
	if ref == nil {
		return nil
	}
	if strings.TrimSpace(ref.Path) == "" {
		return fmt.Errorf("CI result %s path is required", name)
	}
	if ref.SHA256 != "" {
		if len(ref.SHA256) != 64 {
			return fmt.Errorf("CI result %s sha256 must be 64 hexadecimal characters", name)
		}
		if _, err := hex.DecodeString(ref.SHA256); err != nil {
			return fmt.Errorf("CI result %s sha256 is invalid: %w", name, err)
		}
	}
	return nil
}

// ValidateFileRef validates a reusable envelope file reference for consumers
// that carry the same path/hash shape in a producer-owned artifact.
func ValidateFileRef(name string, ref *FileRef) error {
	return validateFileRef(name, ref)
}

func validJSONValue(value json.RawMessage) bool {
	return len(value) > 0 && !strings.EqualFold(string(value), "null") && json.Valid(value)
}

func WriteJSON(w io.Writer, artifact Artifact) error {
	if err := artifact.Validate(); err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(artifact)
}

func SaveFile(path string, artifact Artifact) error {
	if err := artifact.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return fmt.Errorf("encode CI result: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write CI result: %w", err)
	}
	return nil
}

func LoadFile(path string) (Artifact, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Artifact{}, fmt.Errorf("read CI result %s: %w", path, err)
	}
	var artifact Artifact
	if err := json.Unmarshal(contents, &artifact); err != nil {
		return Artifact{}, fmt.Errorf("parse CI result %s: %w", path, err)
	}
	if err := artifact.Validate(); err != nil {
		return Artifact{}, fmt.Errorf("validate CI result %s: %w", path, err)
	}
	return artifact, nil
}
