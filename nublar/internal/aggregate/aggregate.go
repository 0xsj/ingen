// Package aggregate contains Nublar's first coordinator slice. It consumes
// shared CI envelopes and never interprets producer-owned reports.
package aggregate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ingen/core/ciresult"
	"ingen/nublar/internal/artifact"
	"ingen/nublar/internal/output"
	"ingen/nublar/internal/workflow"
)

const Schema = "ingen.nublar-result/v1"

type Input struct {
	CheckID string            `json:"check_id,omitempty"`
	Path    string            `json:"path"`
	SHA256  string            `json:"sha256,omitempty"`
	Result  ciresult.Artifact `json:"result"`
}

// Report preserves every accepted producer artifact and adds only the
// coordinator-owned aggregate status and exit code.
type Report struct {
	Schema     string            `json:"schema"`
	WorkflowID string            `json:"workflow_id,omitempty"`
	Workflow   *ciresult.FileRef `json:"workflow,omitempty"`
	Status     string            `json:"status"`
	ExitCode   int               `json:"exit_code"`
	CreatedAt  string            `json:"created_at"`
	Results    []Input           `json:"results"`
	Warnings   []Issue           `json:"warnings,omitempty"`
	Errors     []Issue           `json:"errors,omitempty"`
}

type Issue struct {
	CheckID string `json:"check_id,omitempty"`
	Tool    string `json:"tool,omitempty"`
	Path    string `json:"path,omitempty"`
	Reason  string `json:"reason"`
}

func ComposeFiles(paths []string) (Report, error) {
	if len(paths) == 0 {
		return Report{}, fmt.Errorf("at least one CI result is required")
	}
	inputs := make([]Input, 0, len(paths))
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			return Report{}, fmt.Errorf("CI result path must not be empty")
		}
		loaded, err := artifact.LoadFile(path)
		if err != nil {
			return Report{}, err
		}
		inputs = append(inputs, Input{Path: path, SHA256: loaded.SHA256, Result: loaded.Artifact})
	}
	return Compose(inputs)
}

func Compose(inputs []Input) (Report, error) {
	if len(inputs) == 0 {
		return Report{}, fmt.Errorf("at least one CI result is required")
	}
	seen := make(map[string]bool, len(inputs))
	status := "passed"
	for index, input := range inputs {
		if strings.TrimSpace(input.Path) == "" {
			return Report{}, fmt.Errorf("CI result %d path must not be empty", index+1)
		}
		if seen[input.Path] {
			return Report{}, fmt.Errorf("CI result path %q was supplied more than once", input.Path)
		}
		seen[input.Path] = true
		if err := input.Result.Validate(); err != nil {
			return Report{}, fmt.Errorf("validate CI result %q: %w", input.Path, err)
		}
		if severity(input.Result.Status) > severity(status) {
			status = input.Result.Status
		}
	}
	report := Report{
		Schema:    Schema,
		Status:    status,
		ExitCode:  exitCode(status),
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Results:   append([]Input(nil), inputs...),
	}
	return report, nil
}

// ComposeWorkflow resolves the checks declared by a workflow against root.
// Missing required results and producer identity mismatches become an error
// result; missing optional results are retained as warnings.
func ComposeWorkflow(document workflow.Document, root string) Report {
	report := Report{
		Schema:     Schema,
		WorkflowID: document.ID,
		Status:     "passed",
		CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}
	inputs := make([]Input, 0, len(document.Checks))
	for _, check := range document.Checks {
		path := filepath.Join(root, check.Result)
		loaded, err := artifact.LoadFile(path)
		if err != nil {
			issue := Issue{CheckID: check.ID, Tool: check.Tool, Path: check.Result, Reason: err.Error()}
			if check.IsRequired() || !errors.Is(err, os.ErrNotExist) {
				report.Errors = append(report.Errors, issue)
			} else {
				report.Warnings = append(report.Warnings, issue)
			}
			continue
		}
		result := loaded.Artifact
		if result.Tool != check.Tool {
			report.Errors = append(report.Errors, Issue{
				CheckID: check.ID,
				Tool:    check.Tool,
				Path:    check.Result,
				Reason:  fmt.Sprintf("result tool is %q, workflow expects %q", result.Tool, check.Tool),
			})
			continue
		}
		inputs = append(inputs, Input{CheckID: check.ID, Path: check.Result, SHA256: loaded.SHA256, Result: result})
	}
	report.Results = inputs
	if len(report.Errors) > 0 || len(report.Results) == 0 {
		report.Status = "error"
		report.ExitCode = 2
		if len(report.Errors) == 0 {
			report.Errors = append(report.Errors, Issue{Reason: "workflow produced no result artifacts"})
		}
	} else {
		for _, input := range report.Results {
			if severity(input.Result.Status) > severity(report.Status) {
				report.Status = input.Result.Status
			}
		}
		report.ExitCode = exitCode(report.Status)
	}
	return report
}

// ComposeWorkflowFile loads a workflow, records its exact bytes, and resolves
// its declared checks against root.
func ComposeWorkflowFile(path, root string) (Report, error) {
	document, reference, err := workflow.LoadFileWithReference(path)
	if err != nil {
		return Report{}, err
	}
	report := ComposeWorkflow(document, root)
	report.Workflow = &reference
	return report, nil
}

func (r Report) Validate() error {
	if r.Schema != Schema {
		return fmt.Errorf("Nublar report schema must be %s, got %q", Schema, r.Schema)
	}
	if err := ciresult.ValidateFileRef("workflow", r.Workflow); err != nil {
		return err
	}
	if _, err := time.Parse(time.RFC3339Nano, r.CreatedAt); err != nil {
		return fmt.Errorf("Nublar report created_at must be RFC3339: %w", err)
	}
	if len(r.Results) == 0 && len(r.Errors) == 0 {
		return fmt.Errorf("Nublar report needs at least one result or error")
	}
	if severity(r.Status) < 0 {
		return fmt.Errorf("Nublar report has unsupported status %q", r.Status)
	}
	if r.ExitCode != exitCode(r.Status) {
		return fmt.Errorf("Nublar report status %q must have exit_code %d", r.Status, exitCode(r.Status))
	}
	seen := make(map[string]bool, len(r.Results))
	for _, input := range r.Results {
		if strings.TrimSpace(input.Path) == "" {
			return fmt.Errorf("Nublar report result path must not be empty")
		}
		if seen[input.Path] {
			return fmt.Errorf("Nublar report result path %q was duplicated", input.Path)
		}
		seen[input.Path] = true
		if err := input.Result.Validate(); err != nil {
			return fmt.Errorf("Nublar report result %q: %w", input.Path, err)
		}
		if input.SHA256 != "" {
			if err := ciresult.ValidateFileRef("result", &ciresult.FileRef{Path: input.Path, SHA256: input.SHA256}); err != nil {
				return err
			}
		}
	}
	for _, issue := range append(append([]Issue(nil), r.Warnings...), r.Errors...) {
		if strings.TrimSpace(issue.Reason) == "" {
			return fmt.Errorf("Nublar report issue reason must not be empty")
		}
	}
	return nil
}

func hashFile(path string) (string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:]), nil
}

func WriteJSON(w io.Writer, report Report) error {
	if err := report.Validate(); err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func severity(status string) int {
	switch status {
	case "passed":
		return 0
	case "failed":
		return 1
	case "error":
		return 2
	default:
		return -1
	}
}

func exitCode(status string) int {
	code, err := ciresult.ExitCodeForStatus(status)
	if err != nil {
		return 2
	}
	return code
}

func SaveFile(path string, report Report) error {
	var data bytes.Buffer
	if err := WriteJSON(&data, report); err != nil {
		return fmt.Errorf("write Nublar report %s: %w", path, err)
	}
	if err := output.WriteFile(path, data.Bytes()); err != nil {
		return err
	}
	return nil
}
