package sattler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// decodeStrictJSON decodes one JSON document and rejects unknown fields and
// trailing documents. Sattler uses it only for its own manifest boundaries;
// producer-owned records keep their adapter-specific compatibility rules.
func decodeStrictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON documents are not allowed")
		}
		return err
	}
	return nil
}

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

// Validate checks the promoted machine-readable error envelope. Unlike the
// comparison projections, this shape is shared by every JSON-mode CLI
// operation and has no producer-owned extension point.
func (document ErrorDocument) Validate() error {
	if document.Schema != ErrorSchema {
		return fmt.Errorf("error document schema must be %s, got %q", ErrorSchema, document.Schema)
	}
	if strings.TrimSpace(document.Operation) == "" {
		return fmt.Errorf("error document operation is required")
	}
	if len(document.Errors) == 0 {
		return fmt.Errorf("error document needs at least one error")
	}
	for index, issue := range document.Errors {
		if strings.TrimSpace(issue.Code) == "" || strings.TrimSpace(issue.Message) == "" {
			return fmt.Errorf("error document errors[%d] needs a code and message", index)
		}
	}
	return nil
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
	document := ErrorDocumentFor(operation, err)
	if validationErr := document.Validate(); validationErr != nil {
		return validationErr
	}
	return json.NewEncoder(w).Encode(document)
}
