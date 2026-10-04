// Package roleexec adapts producer-verified Sentinel role execution evidence
// into Lockwood's existing custody records and generic digest lineage.
package roleexec

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"ingen/herdr-sentinel/evidence"
	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/custody"
)

const (
	ReportMediaType = "application/vnd.ingen.sentinel-role-execution+json"
	FileMediaType   = "application/octet-stream"
)

var custodyID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type ImportRequest struct {
	ProjectRoot    string
	ReportPath     string
	ExpectedSHA256 string
	Schema         string
	CustodyID      string
	LogicalName    string
	MaxBytes       int64
	ReceivedAt     time.Time
	Source         custody.Source
	RetentionClass string
	Redaction      string
}

type Importer struct {
	ingestor *custody.Ingestor
	records  custody.RecordStore
	verify   func(string, string, string) (evidence.Verified, error)
}

func NewImporter(ingestor *custody.Ingestor, records custody.RecordStore) (*Importer, error) {
	if ingestor == nil {
		return nil, fmt.Errorf("custody ingestor is required")
	}
	if records == nil {
		return nil, fmt.Errorf("custody record store is required")
	}
	return &Importer{ingestor: ingestor, records: records, verify: evidence.Verify}, nil
}

// Import verifies the producer report before publishing any custody record.
// The report and every verifier-returned file are stored byte-for-byte. The
// accepted report record references accepted records for all related files,
// allowing existing VerifyRecord/VerifyLineage to verify the full evidence
// graph later.
func (i *Importer) Import(request ImportRequest) (custody.Record, error) {
	if strings.TrimSpace(request.ProjectRoot) == "" {
		return custody.Record{}, fmt.Errorf("Sentinel project root is required")
	}
	if strings.TrimSpace(request.ReportPath) == "" {
		return custody.Record{}, fmt.Errorf("Sentinel role execution report path is required")
	}
	if request.MaxBytes < 0 {
		return custody.Record{}, fmt.Errorf("maximum artifact size cannot be negative")
	}
	verified, err := i.verify(request.ProjectRoot, request.ReportPath, request.ExpectedSHA256)
	if err != nil {
		return custody.Record{}, fmt.Errorf("verify Sentinel role execution evidence: %w", err)
	}
	return i.ImportVerified(request, verified)
}

// ImportVerified publishes a snapshot already returned by evidence.Verify.
// It exists so callers can check their output store against every selected
// input path before opening or mutating that store.
func (i *Importer) ImportVerified(request ImportRequest, verified evidence.Verified) (custody.Record, error) {
	if strings.TrimSpace(request.ProjectRoot) == "" {
		return custody.Record{}, fmt.Errorf("Sentinel project root is required")
	}
	if strings.TrimSpace(request.ReportPath) == "" {
		return custody.Record{}, fmt.Errorf("Sentinel role execution report path is required")
	}
	if request.MaxBytes < 0 {
		return custody.Record{}, fmt.Errorf("maximum artifact size cannot be negative")
	}
	if request.ExpectedSHA256 != "" && request.ExpectedSHA256 != verified.ReportSHA256 {
		return custody.Record{}, fmt.Errorf("verified role-execution report does not match expected SHA-256")
	}
	producerResult, err := verified.BuildCIResult(request.ProjectRoot)
	if err != nil {
		return custody.Record{}, fmt.Errorf("validate immutable Sentinel evidence snapshot: %w", err)
	}
	reportInput, ok := producerResult.Inputs["role-execution-report"]
	if !ok || reportInput.Path != filepath.ToSlash(filepath.Clean(request.ReportPath)) || reportInput.SHA256 != verified.ReportSHA256 {
		return custody.Record{}, fmt.Errorf("verified Sentinel snapshot report path does not match requested report path")
	}
	if rawSHA256(verified.ReportBytes) != verified.ReportSHA256 {
		return custody.Record{}, fmt.Errorf("verified role-execution report bytes do not match the verifier digest")
	}
	for _, file := range verified.Files {
		if rawSHA256(file.Bytes) != file.SHA256 {
			return custody.Record{}, fmt.Errorf("verified evidence bytes for %q do not match the verifier digest", file.Path)
		}
	}
	if request.MaxBytes > 0 {
		var total int64 = int64(len(verified.ReportBytes))
		if total > request.MaxBytes {
			return custody.Record{}, fmt.Errorf("Sentinel role execution evidence exceeds maximum size of %d bytes", request.MaxBytes)
		}
		for _, file := range verified.Files {
			if int64(len(file.Bytes)) > request.MaxBytes-total || total > request.MaxBytes {
				return custody.Record{}, fmt.Errorf("Sentinel role execution evidence exceeds maximum size of %d bytes", request.MaxBytes)
			}
			total += int64(len(file.Bytes))
		}
	}

	schema := request.Schema
	if schema == "" {
		schema = custody.SchemaV1
	}
	if request.RetentionClass == "" {
		request.RetentionClass = "default"
	}
	if request.Redaction == "" {
		request.Redaction = "none"
	}
	if request.LogicalName == "" {
		request.LogicalName = filepath.Base(filepath.Clean(request.ReportPath))
	}
	if request.Source.RunID == "" {
		request.Source.RunID = verified.Report.ExecutionID
	}
	request.Source.Path = filepath.ToSlash(filepath.Clean(request.ReportPath))

	// Preflight all generated IDs so a collision cannot leave a partly
	// published graph. The backing store itself remains append-only and does
	// not provide a multi-record transaction.
	parentIDs := make([]string, len(verified.Files))
	seenDigest := map[string]string{}
	for index, file := range verified.Files {
		if prior, ok := seenDigest[file.SHA256]; ok {
			parentIDs[index] = prior
			continue
		}
		id := relatedCustodyID(request.CustodyID, file.Kind, file.SHA256)
		parentIDs[index] = id
		seenDigest[file.SHA256] = id
	}
	if err := preflightIDs(i.records, append(append([]string(nil), parentIDs...), request.CustodyID)); err != nil {
		return custody.Record{}, err
	}
	if err := validateCandidate(request, verified, parentIDs); err != nil {
		return custody.Record{}, err
	}

	parentRefs := make([]custody.Lineage, 0, len(verified.Files))
	published := make(map[string]bool)
	publishedRefs := make([]string, 0, len(verified.Files))
	for index, file := range verified.Files {
		id := parentIDs[index]
		if published[id] {
			continue
		}
		ref, err := i.ingestor.Accept(bytes.NewReader(file.Bytes), custody.IntakeRequest{
			Schema:         schema,
			CustodyID:      id,
			ExpectedDigest: artifact.SHA256Algorithm + ":" + file.SHA256,
			MediaType:      FileMediaType,
			LogicalName:    filepath.Base(filepath.FromSlash(file.Path)),
			MaxBytes:       request.MaxBytes,
			ReceivedAt:     request.ReceivedAt,
			Producer:       custody.Producer{Tool: "sentinel", Kind: "role-execution-" + file.Kind},
			Source:         custody.Source{RunID: verified.Report.ExecutionID, Path: file.Path},
			Handling:       custody.Handling{Redaction: request.Redaction, RetentionClass: request.RetentionClass},
		})
		if err != nil {
			if len(published) > 0 {
				return custody.Record{}, fmt.Errorf("publish related Sentinel evidence %q (%s) after related records were accepted (%s); inspect these custody IDs and digests before retrying: %w", id, file.Path, strings.Join(publishedRefs, ", "), err)
			}
			return custody.Record{}, fmt.Errorf("publish related Sentinel evidence %q (%s): %w", id, file.Path, err)
		}
		published[id] = true
		parentRefs = append(parentRefs, custody.Lineage{Relation: custody.References, Digest: ref.Artifact.Digest})
		publishedRefs = append(publishedRefs, id+"="+ref.Artifact.Digest)
	}

	record, err := i.ingestor.Accept(bytes.NewReader(verified.ReportBytes), custody.IntakeRequest{
		Schema:         schema,
		CustodyID:      request.CustodyID,
		ExpectedDigest: artifact.SHA256Algorithm + ":" + verified.ReportSHA256,
		MediaType:      ReportMediaType,
		LogicalName:    request.LogicalName,
		MaxBytes:       request.MaxBytes,
		ReceivedAt:     request.ReceivedAt,
		Producer:       custody.Producer{Tool: "sentinel", Kind: "role-execution-report"},
		Source:         request.Source,
		Parents:        parentRefs,
		Handling:       custody.Handling{Redaction: request.Redaction, RetentionClass: request.RetentionClass},
	})
	if err != nil {
		if len(published) > 0 {
			return custody.Record{}, fmt.Errorf("publish Sentinel role execution report %q after related records were accepted (%s); inspect or recover these custody IDs before retrying: %w", request.CustodyID, strings.Join(publishedRefs, ", "), err)
		}
		return custody.Record{}, fmt.Errorf("publish Sentinel role execution report: %w", err)
	}
	return record, nil
}

func rawSHA256(contents []byte) string {
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:])
}

func validateCandidate(request ImportRequest, verified evidence.Verified, parentIDs []string) error {
	if !custodyID.MatchString(request.CustodyID) {
		return fmt.Errorf("invalid custody id %q", request.CustodyID)
	}
	for index, id := range parentIDs {
		if !custodyID.MatchString(id) {
			return fmt.Errorf("invalid related custody id %q for evidence %q", id, verified.Files[index].Path)
		}
	}
	schema := request.Schema
	if schema == "" {
		schema = custody.SchemaV1
	}
	retention := request.RetentionClass
	if retention == "" {
		retention = "default"
	}
	redaction := request.Redaction
	if redaction == "" {
		redaction = "none"
	}
	now := time.Now().UTC()
	placeholder := artifact.Reference{Schema: artifact.Schema, Digest: "sha256:" + strings.Repeat("0", 64), SizeBytes: 0, MediaType: "application/octet-stream"}
	seenParentIDs := map[string]bool{}
	for index, file := range verified.Files {
		if seenParentIDs[parentIDs[index]] {
			continue
		}
		seenParentIDs[parentIDs[index]] = true
		candidate := custody.Record{
			Schema: schema, CustodyID: parentIDs[index], Status: custody.Accepted, Artifact: placeholder,
			ReceivedAt: now, Producer: custody.Producer{Tool: "sentinel", Kind: "role-execution-" + file.Kind},
			Custodian: custody.Custodian{Tool: "lockwood"}, Source: custody.Source{RunID: verified.Report.ExecutionID, Path: file.Path},
			Integrity: custody.Integrity{Status: custody.IntegrityVerified, Method: artifact.SHA256Algorithm, VerifiedAt: &now},
			Parents:   []custody.Lineage{}, Handling: custody.Handling{Redaction: redaction, RetentionClass: retention},
		}
		if err := candidate.Validate(); err != nil {
			return fmt.Errorf("invalid related custody record for %q: %w", file.Path, err)
		}
	}
	mainCandidate := custody.Record{
		Schema: schema, CustodyID: request.CustodyID, Status: custody.Accepted, Artifact: placeholder,
		ReceivedAt: now, Producer: custody.Producer{Tool: "sentinel", Kind: "role-execution-report"},
		Custodian: custody.Custodian{Tool: "lockwood"}, Source: request.Source,
		Integrity: custody.Integrity{Status: custody.IntegrityVerified, Method: artifact.SHA256Algorithm, VerifiedAt: &now},
		Parents:   lineageForFiles(verified.Files), Handling: custody.Handling{Redaction: redaction, RetentionClass: retention},
	}
	if err := mainCandidate.Validate(); err != nil {
		return fmt.Errorf("invalid report custody metadata: %w", err)
	}
	return nil
}

func lineageForFiles(files []evidence.File) []custody.Lineage {
	parents := make([]custody.Lineage, 0, len(files))
	seen := make(map[string]bool, len(files))
	for _, file := range files {
		digest := artifact.SHA256Algorithm + ":" + file.SHA256
		if seen[digest] {
			continue
		}
		seen[digest] = true
		parents = append(parents, custody.Lineage{Relation: custody.References, Digest: digest})
	}
	return parents
}

func preflightIDs(records custody.RecordStore, ids []string) error {
	existing, err := records.List()
	if err != nil {
		return fmt.Errorf("list custody records for role execution import preflight: %w", err)
	}
	present := make(map[string]bool, len(existing))
	for _, record := range existing {
		present[record.CustodyID] = true
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if present[id] {
			return fmt.Errorf("custody ID %q already exists", id)
		}
	}
	return nil
}

func relatedCustodyID(parent, kind, digest string) string {
	var safe strings.Builder
	for _, char := range kind {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' {
			safe.WriteRune(char)
		} else {
			safe.WriteByte('-')
		}
	}
	fullDigest := strings.TrimPrefix(digest, "sha256:")
	return parent + "-related-" + safe.String() + "-" + fullDigest
}
