package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"ingen/paddock/internal/explain"
	"ingen/paddock/internal/model"
)

const Schema = "ingen.ci-result/v1"

type FileRef struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256,omitempty"`
}

type Source struct {
	Root       string `json:"root"`
	ModulePath string `json:"module_path"`
}

type Artifact struct {
	Schema      string            `json:"schema"`
	Tool        string            `json:"tool"`
	Kind        string            `json:"kind"`
	Status      string            `json:"status"`
	ExitCode    int               `json:"exit_code"`
	CreatedAt   string            `json:"created_at"`
	Source      Source            `json:"source"`
	Policy      FileRef           `json:"policy"`
	PolicyLock  *FileRef          `json:"policy_lock,omitempty"`
	Graph       *FileRef          `json:"graph,omitempty"`
	Baseline    *FileRef          `json:"baseline,omitempty"`
	Report      *model.Result     `json:"report,omitempty"`
	Explanation *explain.Document `json:"explanation,omitempty"`
	Error       string            `json:"error,omitempty"`
}

func New(result *model.Result, policy FileRef, baseline *FileRef, createdAt time.Time) Artifact {
	return NewWithInputs(result, policy, nil, nil, baseline, createdAt)
}

func NewWithPolicyLock(result *model.Result, policy FileRef, policyLock *FileRef, baseline *FileRef, createdAt time.Time) Artifact {
	return NewWithInputs(result, policy, policyLock, nil, baseline, createdAt)
}

func NewWithInputs(result *model.Result, policy FileRef, policyLock, graph *FileRef, baseline *FileRef, createdAt time.Time) Artifact {
	status := "passed"
	exitCode := 0
	if !result.OK() {
		status = "failed"
		exitCode = 1
	}
	explanation := explain.Explain(result)
	return Artifact{
		Schema:      Schema,
		Tool:        "paddock",
		Kind:        "architecture",
		Status:      status,
		ExitCode:    exitCode,
		CreatedAt:   createdAt.UTC().Format(time.RFC3339Nano),
		Source:      Source{Root: result.Root, ModulePath: result.ModulePath},
		Policy:      policy,
		PolicyLock:  policyLock,
		Graph:       graph,
		Baseline:    baseline,
		Report:      result,
		Explanation: &explanation,
	}
}

func NewError(root string, policy FileRef, baseline *FileRef, err error, createdAt time.Time) Artifact {
	return NewErrorWithInputs(root, policy, nil, nil, baseline, err, createdAt)
}

func NewErrorWithPolicyLock(root string, policy FileRef, policyLock *FileRef, baseline *FileRef, err error, createdAt time.Time) Artifact {
	return NewErrorWithInputs(root, policy, policyLock, nil, baseline, err, createdAt)
}

func NewErrorWithInputs(root string, policy FileRef, policyLock, graph, baseline *FileRef, err error, createdAt time.Time) Artifact {
	return Artifact{
		Schema:     Schema,
		Tool:       "paddock",
		Kind:       "architecture",
		Status:     "error",
		ExitCode:   2,
		CreatedAt:  createdAt.UTC().Format(time.RFC3339Nano),
		Source:     Source{Root: root},
		Policy:     policy,
		PolicyLock: policyLock,
		Graph:      graph,
		Baseline:   baseline,
		Error:      err.Error(),
	}
}

func File(path string) (FileRef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return FileRef{Path: path}, fmt.Errorf("hash %s: %w", path, err)
	}
	digest := sha256.Sum256(data)
	return FileRef{Path: path, SHA256: hex.EncodeToString(digest[:])}, nil
}

func Load(path string) (Artifact, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Artifact{}, fmt.Errorf("read CI artifact: %w", err)
	}
	var artifact Artifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return Artifact{}, fmt.Errorf("parse CI artifact: %w", err)
	}
	if err := artifact.Validate(); err != nil {
		return Artifact{}, err
	}
	return artifact, nil
}

func Save(path string, artifact Artifact) error {
	if err := artifact.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return fmt.Errorf("encode CI artifact: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write CI artifact: %w", err)
	}
	return nil
}

func (a Artifact) Validate() error {
	if a.Schema != Schema {
		return fmt.Errorf("CI artifact schema must be %s, got %q", Schema, a.Schema)
	}
	if a.Tool == "" || a.Kind == "" {
		return fmt.Errorf("CI artifact tool and kind are required")
	}
	if a.CreatedAt == "" {
		return fmt.Errorf("CI artifact created_at is required")
	}
	if a.Status != "passed" && a.Status != "failed" && a.Status != "error" {
		return fmt.Errorf("CI artifact has unsupported status %q", a.Status)
	}
	if a.Policy.Path == "" {
		return fmt.Errorf("CI artifact policy path is required")
	}
	if a.PolicyLock != nil && a.PolicyLock.Path == "" {
		return fmt.Errorf("CI artifact policy_lock path is required")
	}
	if a.PolicyLock != nil && a.PolicyLock.SHA256 == "" {
		return fmt.Errorf("CI artifact policy_lock sha256 is required")
	}
	if a.Graph != nil && a.Graph.Path == "" {
		return fmt.Errorf("CI artifact graph path is required")
	}
	if a.Graph != nil && a.Graph.SHA256 == "" {
		return fmt.Errorf("CI artifact graph sha256 is required")
	}
	if a.Status == "error" {
		if a.ExitCode != 2 || a.Error == "" {
			return fmt.Errorf("error CI artifacts need exit_code 2 and an error")
		}
		return nil
	}
	if a.Report == nil || a.Explanation == nil {
		return fmt.Errorf("successful CI artifacts need a report and explanation")
	}
	if a.Report.Schema != "paddock.report/v1" || a.Explanation.Schema != "paddock.explanation/v1" {
		return fmt.Errorf("CI artifact contains unsupported nested artifact schema")
	}
	wantStatus, wantCode := "passed", 0
	if !a.Report.OK() {
		wantStatus, wantCode = "failed", 1
	}
	if a.Status != wantStatus || a.ExitCode != wantCode {
		return fmt.Errorf("CI artifact status does not match report verdict")
	}
	return nil
}

func WriteJSON(w io.Writer, artifact Artifact) error {
	if err := artifact.Validate(); err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(artifact)
}
