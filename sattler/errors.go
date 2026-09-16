package sattler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrorSchema identifies the stable machine-readable error envelope emitted
// by Sattler commands when JSON output was requested.
const ErrorSchema = "ingen.sattler-error/v0"

// ErrorIssue is one actionable failure in an Sattler operation.
type ErrorIssue struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

// ErrorDocument is the machine-readable failure envelope for CLI operations.
type ErrorDocument struct {
	Schema    string       `json:"schema"`
	Operation string       `json:"operation"`
	Errors    []ErrorIssue `json:"errors"`
}

// ManifestValidationError preserves stable issue codes for comparison
// manifest failures while keeping the ordinary error string useful to humans.
type ManifestValidationError struct {
	Issues []ErrorIssue
}

func (e *ManifestValidationError) Error() string {
	if len(e.Issues) == 0 {
		return "comparison manifest validation failed"
	}
	messages := make([]string, 0, len(e.Issues))
	for _, issue := range e.Issues {
		messages = append(messages, issue.Message)
	}
	return strings.Join(messages, "; ")
}

// ErrorDocumentFor converts an operation failure into Sattler's stable JSON
// error shape. Non-manifest failures remain intentionally generic because the
// producer-specific adapters own their detailed validation semantics.
func ErrorDocumentFor(operation string, err error) ErrorDocument {
	document := ErrorDocument{
		Schema:    ErrorSchema,
		Operation: operation,
		Errors: []ErrorIssue{{
			Code:    "operation-failed",
			Message: err.Error(),
		}},
	}
	var validationErr *ManifestValidationError
	if errors.As(err, &validationErr) {
		document.Errors = append([]ErrorIssue(nil), validationErr.Issues...)
	}
	return document
}

// WriteErrorJSON writes a stable machine-readable operation failure.
func WriteErrorJSON(w io.Writer, operation string, err error) error {
	if err == nil {
		return fmt.Errorf("cannot write a nil Sattler error")
	}
	return json.NewEncoder(w).Encode(ErrorDocumentFor(operation, err))
}
