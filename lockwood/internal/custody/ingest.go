package custody

import (
	"fmt"
	"io"
	"strings"
	"time"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/store"
)

type IntakeRequest struct {
	Schema         string
	CustodyID      string
	ExpectedDigest string
	MediaType      string
	LogicalName    string
	MaxBytes       int64
	ReceivedAt     time.Time
	Producer       Producer
	Source         Source
	Parents        []Lineage
	Handling       Handling
}

type IntakeError struct {
	Record   Record
	Artifact artifact.Reference
	Err      error
}

func (e *IntakeError) Error() string {
	return fmt.Sprintf("publish custody record for %s: %v", e.Artifact.Digest, e.Err)
}

func (e *IntakeError) Unwrap() error {
	return e.Err
}

type Ingestor struct {
	artifacts *store.Filesystem
	records   *Filesystem
}

func NewIngestor(artifacts *store.Filesystem, records *Filesystem) (*Ingestor, error) {
	if artifacts == nil {
		return nil, fmt.Errorf("artifact store is required")
	}
	if records == nil {
		return nil, fmt.Errorf("custody record store is required")
	}
	return &Ingestor{artifacts: artifacts, records: records}, nil
}

func (i *Ingestor) Accept(reader io.Reader, request IntakeRequest) (Record, error) {
	schema := request.Schema
	if schema == "" {
		schema = SchemaV1
	}
	request.Schema = schema
	if err := request.validate(); err != nil {
		return Record{}, err
	}
	ref, err := i.artifacts.Put(reader, store.PutOptions{
		ExpectedDigest: request.ExpectedDigest,
		MediaType:      request.MediaType,
		LogicalName:    request.LogicalName,
		MaxBytes:       request.MaxBytes,
	})
	if err != nil {
		return Record{}, fmt.Errorf("store artifact: %w", err)
	}

	receivedAt := request.ReceivedAt
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	}
	parents := request.Parents
	if parents == nil {
		parents = []Lineage{}
	}
	record := Record{
		Schema:     schema,
		CustodyID:  request.CustodyID,
		Status:     Accepted,
		Artifact:   ref,
		ReceivedAt: receivedAt,
		Producer:   request.Producer,
		Custodian:  Custodian{Tool: "lockwood"},
		Source:     request.Source,
		Integrity: Integrity{
			Status:     IntegrityVerified,
			Method:     artifact.SHA256Algorithm,
			VerifiedAt: timePtr(time.Now().UTC()),
		},
		Parents:  parents,
		Handling: request.Handling,
	}
	if err := i.records.Put(record); err != nil {
		return Record{Artifact: ref}, &IntakeError{Record: record, Artifact: ref, Err: err}
	}
	return record, nil
}

func timePtr(value time.Time) *time.Time {
	return &value
}

func (request IntakeRequest) validate() error {
	if !custodyIDPattern.MatchString(request.CustodyID) {
		return fmt.Errorf("invalid custody id %q", request.CustodyID)
	}
	if request.Schema != SchemaV1 && request.Schema != SchemaV2 {
		return fmt.Errorf("unexpected custody schema %q", request.Schema)
	}
	if request.MaxBytes < 0 {
		return fmt.Errorf("maximum artifact size cannot be negative")
	}
	if strings.TrimSpace(request.MediaType) == "" {
		return fmt.Errorf("artifact media type is required")
	}
	if strings.TrimSpace(request.Producer.Tool) == "" || strings.TrimSpace(request.Producer.Kind) == "" {
		return fmt.Errorf("producer tool and kind are required")
	}
	if err := request.Source.validate(request.Schema); err != nil {
		return err
	}
	for _, parent := range request.Parents {
		if err := parent.validateSyntax(); err != nil {
			return err
		}
	}
	if request.Handling.Redaction != "none" && request.Handling.Redaction != "redacted" {
		return fmt.Errorf("invalid redaction status %q", request.Handling.Redaction)
	}
	if strings.TrimSpace(request.Handling.RetentionClass) == "" {
		return fmt.Errorf("retention class is required")
	}
	return nil
}
