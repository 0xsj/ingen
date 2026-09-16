package ciresult

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	coreciresult "ingen/core/ciresult"
	"ingen/lockwood/internal/custody"
)

const MediaType = "application/vnd.ingen.ci-result+json"

type ImportRequest struct {
	Schema         string
	CustodyID      string
	ExpectedDigest string
	LogicalName    string
	MaxBytes       int64
	ReceivedAt     time.Time
	Source         custody.Source
	RetentionClass string
	Redaction      string
}

type Importer struct {
	ingestor *custody.Ingestor
}

func NewImporter(ingestor *custody.Ingestor) (*Importer, error) {
	if ingestor == nil {
		return nil, fmt.Errorf("custody ingestor is required")
	}
	return &Importer{ingestor: ingestor}, nil
}

func (i *Importer) Import(path string, request ImportRequest) (custody.Record, error) {
	if strings.TrimSpace(path) == "" {
		return custody.Record{}, fmt.Errorf("CI result path is required")
	}
	data, err := readLimitedFile(path, request.MaxBytes)
	if err != nil {
		return custody.Record{}, fmt.Errorf("read CI result: %w", err)
	}
	var result coreciresult.Artifact
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return custody.Record{}, fmt.Errorf("parse CI result: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return custody.Record{}, fmt.Errorf("parse CI result: multiple JSON values")
		}
		return custody.Record{}, fmt.Errorf("parse CI result: %w", err)
	}
	if err := result.Validate(); err != nil {
		return custody.Record{}, fmt.Errorf("validate CI result: %w", err)
	}
	if request.LogicalName == "" {
		request.LogicalName = filepath.Base(filepath.Clean(path))
	}
	if request.RetentionClass == "" {
		request.RetentionClass = "default"
	}
	if request.Redaction == "" {
		request.Redaction = "none"
	}
	if request.Source.RunID == "" && request.Source.Path == "" && request.Source.URI == "" {
		request.Source.Path = filepath.Clean(path)
	}
	schema := request.Schema
	if schema == "" {
		schema = custody.SchemaV1
	}
	record, err := i.ingestor.Accept(bytes.NewReader(data), custody.IntakeRequest{
		Schema:         schema,
		CustodyID:      request.CustodyID,
		ExpectedDigest: request.ExpectedDigest,
		MediaType:      MediaType,
		LogicalName:    request.LogicalName,
		MaxBytes:       request.MaxBytes,
		ReceivedAt:     request.ReceivedAt,
		Producer:       custody.Producer{Tool: result.Tool, Kind: result.Kind},
		Source:         request.Source,
		Handling:       custody.Handling{Redaction: request.Redaction, RetentionClass: request.RetentionClass},
	})
	if err != nil {
		return custody.Record{}, fmt.Errorf("ingest CI result: %w", err)
	}
	return record, nil
}

func readLimitedFile(path string, maxBytes int64) ([]byte, error) {
	if maxBytes < 0 {
		return nil, fmt.Errorf("maximum artifact size cannot be negative")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := io.Reader(file)
	if maxBytes > 0 {
		reader = io.LimitReader(file, maxBytes)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if maxBytes > 0 && int64(len(data)) == maxBytes {
		var extra [1]byte
		n, readErr := io.ReadFull(file, extra[:])
		if n > 0 {
			return nil, fmt.Errorf("CI result exceeds maximum size of %d bytes", maxBytes)
		}
		if readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			return nil, fmt.Errorf("check CI result size: %w", readErr)
		}
	}
	return data, nil
}
