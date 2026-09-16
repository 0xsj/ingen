package run

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"ingen/core/ciresult"
	"ingen/nublar/internal/artifact"
	"ingen/nublar/internal/workflow"
)

// CollectWorkflowFile loads one workflow and collects its declared result
// files into a versioned Run. Workflow and result hashes are computed from the
// same bytes that are parsed and validated.
func CollectWorkflowFile(workflowPath, root, runID string) (Run, error) {
	return CollectWorkflowFileWithCorrelation(workflowPath, root, runID, nil)
}

// CollectWorkflowFileWithCorrelation collects one workflow and associates it
// with an optional external CI attempt.
func CollectWorkflowFileWithCorrelation(workflowPath, root, runID string, correlation *Correlation) (Run, error) {
	document, workflowRef, err := workflow.LoadFileWithReference(workflowPath)
	if err != nil {
		return Run{}, err
	}
	return CollectWorkflowWithCorrelation(document, workflowRef, root, runID, correlation)
}

// CollectWorkflow resolves checks under root. Collection failures are recorded
// in the returned run so CI consumers receive a reviewable error artifact.
func CollectWorkflow(document workflow.Document, workflowRef ciresult.FileRef, root, runID string) (Run, error) {
	return CollectWorkflowWithCorrelation(document, workflowRef, root, runID, nil)
}

// CollectWorkflowWithCorrelation resolves checks under root and associates
// the resulting run with an optional external CI attempt.
func CollectWorkflowWithCorrelation(document workflow.Document, workflowRef ciresult.FileRef, root, runID string, correlation *Correlation) (Run, error) {
	if err := workflow.Validate(document); err != nil {
		return Run{}, fmt.Errorf("validate Nublar workflow before collection: %w", err)
	}
	if err := validateFileRef("workflow", workflowRef); err != nil {
		return Run{}, fmt.Errorf("validate Nublar workflow reference before collection: %w", err)
	}
	if correlation != nil {
		if err := correlation.Validate(); err != nil {
			return Run{}, fmt.Errorf("validate Nublar correlation before collection: %w", err)
		}
	}
	var storedCorrelation *Correlation
	if correlation != nil {
		value := *correlation
		storedCorrelation = &value
	}
	if runID == "" {
		generated, err := NewID()
		if err != nil {
			return Run{}, err
		}
		runID = generated
	}
	createdAt := time.Now().UTC()
	r := Run{
		Schema:      Schema,
		RunID:       runID,
		Workflow:    Workflow{ID: document.ID, File: workflowRef},
		Correlation: storedCorrelation,
		Status:      "passed",
		CreatedAt:   createdAt.Format(time.RFC3339Nano),
		Checks:      make([]Check, 0, len(document.Checks)),
	}

	presentResults := 0
	for _, declared := range document.Checks {
		check := Check{
			ID:       declared.ID,
			Tool:     declared.Tool,
			Path:     declared.Result,
			Required: declared.IsRequired(),
		}
		path := filepath.Join(root, declared.Result)
		loaded, err := artifact.LoadFile(path)
		if err != nil {
			check.Reason = err.Error()
			if errors.Is(err, os.ErrNotExist) && !check.Required {
				check.Status = "missing"
				r.Warnings = append(r.Warnings, Issue{
					CheckID: check.ID,
					Tool:    check.Tool,
					Path:    check.Path,
					Reason:  check.Reason,
				})
			} else {
				check.Status = "error"
				r.Errors = append(r.Errors, Issue{
					CheckID: check.ID,
					Tool:    check.Tool,
					Path:    check.Path,
					Reason:  check.Reason,
				})
			}
			r.Checks = append(r.Checks, check)
			continue
		}
		if loaded.Artifact.Tool != check.Tool {
			check.Status = "error"
			check.Reason = fmt.Sprintf("producer tool is %q, workflow expects %q", loaded.Artifact.Tool, check.Tool)
			r.Errors = append(r.Errors, Issue{
				CheckID: check.ID,
				Tool:    check.Tool,
				Path:    check.Path,
				Reason:  check.Reason,
			})
			r.Checks = append(r.Checks, check)
			continue
		}

		check.Status = loaded.Artifact.Status
		check.Result = &Result{
			Ref: ciresult.FileRef{
				Path:   check.Path,
				SHA256: loaded.SHA256,
			},
			Artifact: loaded.Artifact,
		}
		presentResults++
		r.Checks = append(r.Checks, check)
	}

	if presentResults == 0 && len(r.Errors) == 0 {
		r.Errors = append(r.Errors, Issue{Reason: "workflow produced no result artifacts"})
	}
	r.Status = decision(r)
	r.ExitCode, _ = ciresult.ExitCodeForStatus(r.Status)
	r.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return r, nil
}

func decision(r Run) string {
	if len(r.Errors) > 0 {
		return "error"
	}
	status := "passed"
	for _, check := range r.Checks {
		if check.Status == "error" {
			return "error"
		}
		if check.Result != nil && severity(check.Result.Artifact.Status) > severity(status) {
			status = check.Result.Artifact.Status
		}
	}
	return status
}
