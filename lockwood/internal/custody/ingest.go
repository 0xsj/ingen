package custody

import (
	"fmt"
	"io"
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
	if !custodyIDPattern.MatchString(request.CustodyID) {
		return Record{}, fmt.Errorf("invalid custody id %q", request.CustodyID)
	}
	ref, err := i.artifacts.Put(reader, store.PutOptions{
		ExpectedDigest: request.ExpectedDigest,
		MediaType:      request.MediaType,
		LogicalName:    request.LogicalName,
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
	schema := request.Schema
	if schema == "" {
		schema = SchemaV1
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
