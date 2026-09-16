package custody

import (
	"fmt"
	"strings"
	"time"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/store"
)

// RedactionPromotionRequest describes the metadata for a new custody record
// that will anchor an already registered redaction result.
type RedactionPromotionRequest struct {
	Schema     string
	CustodyID  string
	Artifact   artifact.Reference
	ReceivedAt time.Time
	Producer   Producer
	Source     Source
	Handling   Handling
}

// PromoteRedactionResult verifies a registered redaction event and creates an
// accepted custody record for its resulting artifact. The new record points
// to the event's source artifact with a derived-from parent. Existing source
// records and artifacts are never modified.
func PromoteRedactionResult(artifacts store.Store, records RecordStore, events HandlingEventStore, sourceCustodyID, eventID string, request RedactionPromotionRequest) (Record, error) {
	if artifacts == nil {
		return Record{}, fmt.Errorf("artifact store is required")
	}
	if records == nil {
		return Record{}, fmt.Errorf("custody record store is required")
	}
	if events == nil {
		return Record{}, fmt.Errorf("handling event store is required")
	}
	event, err := events.GetEvent(sourceCustodyID, eventID)
	if err != nil {
		return Record{}, fmt.Errorf("read redaction event: %w", err)
	}
	if event.Type != RedactionEvent {
		return Record{}, fmt.Errorf("event %q is not a redaction event", eventID)
	}
	if err := event.Validate(); err != nil {
		return Record{}, fmt.Errorf("validate redaction event: %w", err)
	}
	if request.CustodyID == sourceCustodyID {
		return Record{}, fmt.Errorf("promoted custody ID must differ from source custody ID")
	}
	if request.ReceivedAt.IsZero() {
		return Record{}, fmt.Errorf("promotion received_at is required")
	}
	if request.Artifact.Digest != event.ResultingDigest {
		return Record{}, fmt.Errorf("promotion artifact digest %s does not match redaction result %s", request.Artifact.Digest, event.ResultingDigest)
	}
	if err := artifacts.Verify(event.OriginalDigest); err != nil {
		return Record{}, fmt.Errorf("verify redaction source artifact: %w", err)
	}
	if err := artifacts.VerifyReference(request.Artifact); err != nil {
		return Record{}, fmt.Errorf("verify redaction result artifact: %w", err)
	}

	sourceRecord, err := records.Get(sourceCustodyID)
	if err != nil {
		return Record{}, fmt.Errorf("read source custody record: %w", err)
	}
	if err := sourceRecord.Validate(); err != nil {
		return Record{}, fmt.Errorf("validate source custody record: %w", err)
	}
	if sourceRecord.Status != Accepted {
		return Record{}, fmt.Errorf("source custody record %q is not accepted", sourceCustodyID)
	}
	if err := artifacts.VerifyReference(sourceRecord.Artifact); err != nil {
		return Record{}, fmt.Errorf("verify source custody record artifact: %w", err)
	}
	if _, err := findAcceptedArtifactRecord(artifacts, records, event.OriginalDigest); err != nil {
		return Record{}, err
	}

	if request.Schema == "" {
		request.Schema = sourceRecord.Schema
	}
	if request.Schema != SchemaV1 && request.Schema != SchemaV2 {
		return Record{}, fmt.Errorf("unexpected custody schema %q", request.Schema)
	}
	if !custodyIDPattern.MatchString(request.CustodyID) {
		return Record{}, fmt.Errorf("invalid custody id %q", request.CustodyID)
	}
	if strings.TrimSpace(request.Artifact.MediaType) == "" {
		return Record{}, fmt.Errorf("promotion artifact media type is required")
	}
	if strings.TrimSpace(request.Producer.Tool) == "" || strings.TrimSpace(request.Producer.Kind) == "" {
		return Record{}, fmt.Errorf("promotion producer tool and kind are required")
	}
	if err := request.Source.validate(request.Schema); err != nil {
		return Record{}, err
	}
	if request.Handling.Redaction == "" {
		request.Handling.Redaction = "redacted"
	}
	if request.Handling.Redaction != "redacted" {
		return Record{}, fmt.Errorf("promoted redaction result requires redaction status %q", "redacted")
	}
	if request.Handling.RetentionClass == "" {
		request.Handling.RetentionClass = "default"
	}
	verifiedAt := request.ReceivedAt.UTC()
	record := Record{
		Schema:     request.Schema,
		CustodyID:  request.CustodyID,
		Status:     Accepted,
		Artifact:   request.Artifact,
		ReceivedAt: request.ReceivedAt.UTC(),
		Producer:   request.Producer,
		Custodian:  Custodian{Tool: "lockwood"},
		Source:     request.Source,
		Integrity: Integrity{
			Status:     IntegrityVerified,
			Method:     artifact.SHA256Algorithm,
			VerifiedAt: &verifiedAt,
		},
		Parents: []Lineage{{
			Relation: DerivedFrom,
			Digest:   event.OriginalDigest,
		}},
		Handling: request.Handling,
	}
	if err := record.Validate(); err != nil {
		return Record{}, fmt.Errorf("validate promoted custody record: %w", err)
	}
	if err := records.Put(record); err != nil {
		return Record{}, fmt.Errorf("publish promoted custody record: %w", err)
	}
	return record, nil
}

func findAcceptedArtifactRecord(artifacts store.Store, records RecordStore, digest string) (Record, error) {
	items, err := records.List()
	if err != nil {
		return Record{}, fmt.Errorf("list custody records for redaction source: %w", err)
	}
	for _, record := range items {
		if record.Status == Accepted && record.Artifact.Digest == digest {
			if err := artifacts.VerifyReference(record.Artifact); err != nil {
				return Record{}, fmt.Errorf("verify accepted redaction source record %q: %w", record.CustodyID, err)
			}
			return record, nil
		}
	}
	return Record{}, fmt.Errorf("no accepted custody record anchors redaction source artifact %s", digest)
}
