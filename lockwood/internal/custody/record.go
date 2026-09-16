package custody

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"ingen/lockwood/internal/artifact"
)

const (
	SchemaV1 = "lockwood.custody/v1"
	SchemaV2 = "lockwood.custody/v2"
	Schema   = SchemaV1
)

type Status string

const (
	Accepted    Status = "accepted"
	Quarantined Status = "quarantined"
	Rejected    Status = "rejected"
)

type Producer struct {
	Tool    string `json:"tool"`
	Kind    string `json:"kind"`
	Version string `json:"version,omitempty"`
}

type Custodian struct {
	Tool    string `json:"tool"`
	Version string `json:"version,omitempty"`
}

type Source struct {
	RunID   string `json:"run_id,omitempty"`
	Path    string `json:"path,omitempty"`
	URI     string `json:"uri,omitempty"`
	Version string `json:"version,omitempty"`
}

type IntegrityStatus string

const (
	IntegrityVerified   IntegrityStatus = "verified"
	IntegrityFailed     IntegrityStatus = "failed"
	IntegrityNotChecked IntegrityStatus = "not-checked"
)

type Integrity struct {
	Status     IntegrityStatus `json:"status"`
	Method     string          `json:"method"`
	VerifiedAt *time.Time      `json:"verified_at,omitempty"`
}

type Handling struct {
	Redaction      string `json:"redaction"`
	RetentionClass string `json:"retention_class"`
}

type Record struct {
	Schema     string             `json:"schema"`
	CustodyID  string             `json:"custody_id"`
	Status     Status             `json:"status"`
	Artifact   artifact.Reference `json:"artifact"`
	ReceivedAt time.Time          `json:"received_at"`
	Producer   Producer           `json:"producer"`
	Custodian  Custodian          `json:"custodian"`
	Source     Source             `json:"source"`
	Integrity  Integrity          `json:"integrity"`
	Parents    []Lineage          `json:"parents"`
	Handling   Handling           `json:"handling"`
}

// RecordStore is the custody-record backend contract. Implementations must
// validate records, preserve canonical field values, reject conflicting
// custody IDs, and return records in deterministic order from List.
type RecordStore interface {
	Put(record Record) error
	Get(custodyID string) (Record, error)
	List() ([]Record, error)
}

// HandlingEventStore is the append-only event backend contract. Events are
// immutable, keyed by custody ID and event ID, and listed deterministically.
type HandlingEventStore interface {
	AppendEvent(event HandlingEvent) error
	GetEvent(custodyID, eventID string) (HandlingEvent, error)
	ListEvents(custodyID string) ([]HandlingEvent, error)
}

var custodyIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func (record Record) Validate() error {
	if record.Schema != SchemaV1 && record.Schema != SchemaV2 {
		return fmt.Errorf("unexpected custody schema %q", record.Schema)
	}
	if !custodyIDPattern.MatchString(record.CustodyID) {
		return fmt.Errorf("invalid custody id %q", record.CustodyID)
	}
	switch record.Status {
	case Accepted, Quarantined, Rejected:
	default:
		return fmt.Errorf("invalid custody status %q", record.Status)
	}
	if record.Artifact.Schema != artifact.Schema {
		return fmt.Errorf("unexpected artifact schema %q", record.Artifact.Schema)
	}
	if err := artifact.ValidateDigest(record.Artifact.Digest); err != nil {
		return err
	}
	if record.Artifact.SizeBytes < 0 {
		return fmt.Errorf("artifact size cannot be negative")
	}
	if strings.TrimSpace(record.Artifact.MediaType) == "" {
		return fmt.Errorf("artifact media type is required")
	}
	if record.ReceivedAt.IsZero() {
		return fmt.Errorf("received_at is required")
	}
	if strings.TrimSpace(record.Producer.Tool) == "" || strings.TrimSpace(record.Producer.Kind) == "" {
		return fmt.Errorf("producer tool and kind are required")
	}
	if strings.TrimSpace(record.Custodian.Tool) == "" {
		return fmt.Errorf("custodian tool is required")
	}
	if err := record.Source.validate(record.Schema); err != nil {
		return err
	}
	switch record.Integrity.Status {
	case IntegrityVerified:
		if record.Integrity.VerifiedAt == nil || record.Integrity.VerifiedAt.IsZero() {
			return fmt.Errorf("verified integrity requires verified_at")
		}
	case IntegrityFailed, IntegrityNotChecked:
	default:
		return fmt.Errorf("invalid integrity status %q", record.Integrity.Status)
	}
	if record.Integrity.Method != artifact.SHA256Algorithm {
		return fmt.Errorf("unsupported integrity method %q", record.Integrity.Method)
	}
	if record.Status == Accepted && record.Integrity.Status != IntegrityVerified {
		return fmt.Errorf("accepted custody requires verified integrity")
	}
	if record.Parents == nil {
		return fmt.Errorf("parents must be an array")
	}
	for _, parent := range record.Parents {
		if err := parent.validate(record.Artifact.Digest); err != nil {
			return err
		}
	}
	if record.Handling.Redaction != "none" && record.Handling.Redaction != "redacted" {
		return fmt.Errorf("invalid redaction status %q", record.Handling.Redaction)
	}
	if strings.TrimSpace(record.Handling.RetentionClass) == "" {
		return fmt.Errorf("retention class is required")
	}
	return nil
}

func (source Source) validate(schema string) error {
	if strings.TrimSpace(source.RunID) == "" && strings.TrimSpace(source.Path) == "" && strings.TrimSpace(source.URI) == "" {
		return fmt.Errorf("source run_id, path, or uri is required")
	}
	if schema == SchemaV1 && (strings.TrimSpace(source.URI) != "" || strings.TrimSpace(source.Version) != "") {
		return fmt.Errorf("remote source fields require custody schema %q", SchemaV2)
	}
	if strings.TrimSpace(source.Version) != "" && strings.TrimSpace(source.URI) == "" {
		return fmt.Errorf("source version requires source uri")
	}
	if strings.TrimSpace(source.URI) != "" {
		parsed, err := url.Parse(source.URI)
		if err != nil || parsed.Scheme == "" || parsed.User != nil {
			return fmt.Errorf("source uri must be an absolute credential-free uri")
		}
	}
	return nil
}
