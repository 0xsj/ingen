package custody

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"ingen/lockwood/internal/artifact"
)

const HandlingEventSchema = "lockwood.handling-event/v1"

type HandlingEventType string

const (
	RedactionEvent           HandlingEventType = "redaction"
	RetentionClassifiedEvent HandlingEventType = "retention-classified"
	LegalHoldPlacedEvent     HandlingEventType = "legal-hold-placed"
	LegalHoldReleasedEvent   HandlingEventType = "legal-hold-released"
)

var eventIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// HandlingEvent is an append-only record of a retention or redaction action.
// Actor and reason are descriptive caller claims; this contract does not
// authenticate the actor or grant authorization.
type HandlingEvent struct {
	Schema          string            `json:"schema"`
	EventID         string            `json:"event_id"`
	CustodyID       string            `json:"custody_id"`
	Type            HandlingEventType `json:"type"`
	RecordedAt      time.Time         `json:"recorded_at"`
	Actor           string            `json:"actor"`
	Reason          string            `json:"reason"`
	OriginalDigest  string            `json:"original_digest,omitempty"`
	ResultingDigest string            `json:"resulting_digest,omitempty"`
	RetentionClass  string            `json:"retention_class,omitempty"`
	LegalHoldID     string            `json:"legal_hold_id,omitempty"`
}

func (event HandlingEvent) Validate() error {
	if event.Schema != HandlingEventSchema {
		return fmt.Errorf("unexpected handling event schema %q", event.Schema)
	}
	if !eventIDPattern.MatchString(event.EventID) {
		return fmt.Errorf("invalid handling event id %q", event.EventID)
	}
	if !custodyIDPattern.MatchString(event.CustodyID) {
		return fmt.Errorf("invalid handling event custody id %q", event.CustodyID)
	}
	if event.RecordedAt.IsZero() {
		return fmt.Errorf("handling event recorded_at is required")
	}
	if strings.TrimSpace(event.Actor) == "" {
		return fmt.Errorf("handling event actor is required")
	}
	if strings.TrimSpace(event.Reason) == "" {
		return fmt.Errorf("handling event reason is required")
	}
	if event.OriginalDigest != "" {
		if err := artifact.ValidateDigest(event.OriginalDigest); err != nil {
			return fmt.Errorf("invalid handling event original digest: %w", err)
		}
	}
	if event.ResultingDigest != "" {
		if err := artifact.ValidateDigest(event.ResultingDigest); err != nil {
			return fmt.Errorf("invalid handling event resulting digest: %w", err)
		}
	}
	if event.LegalHoldID != "" && !eventIDPattern.MatchString(event.LegalHoldID) {
		return fmt.Errorf("invalid handling event legal hold id %q", event.LegalHoldID)
	}

	switch event.Type {
	case RedactionEvent:
		if event.OriginalDigest == "" || event.ResultingDigest == "" {
			return fmt.Errorf("redaction event requires original and resulting digests")
		}
		if event.OriginalDigest == event.ResultingDigest {
			return fmt.Errorf("redaction event digests must differ")
		}
		if event.RetentionClass != "" || event.LegalHoldID != "" {
			return fmt.Errorf("redaction event has unrelated handling fields")
		}
	case RetentionClassifiedEvent:
		if strings.TrimSpace(event.RetentionClass) == "" {
			return fmt.Errorf("retention event requires retention class")
		}
		if event.OriginalDigest != "" || event.ResultingDigest != "" || event.LegalHoldID != "" {
			return fmt.Errorf("retention event has unrelated handling fields")
		}
	case LegalHoldPlacedEvent, LegalHoldReleasedEvent:
		if event.LegalHoldID == "" {
			return fmt.Errorf("legal hold event requires legal hold id")
		}
		if event.OriginalDigest != "" || event.ResultingDigest != "" || event.RetentionClass != "" {
			return fmt.Errorf("legal hold event has unrelated handling fields")
		}
	default:
		return fmt.Errorf("unsupported handling event type %q", event.Type)
	}
	return nil
}

// MarshalCanonical returns compact JSON with a UTC recorded_at value and no
// trailing newline. This is the immutable local event representation.
func MarshalCanonicalHandlingEvent(event HandlingEvent) ([]byte, error) {
	event.RecordedAt = event.RecordedAt.UTC()
	if err := event.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(event)
}

// UnmarshalCanonicalHandlingEvent decodes one strictly validated event.
func UnmarshalCanonicalHandlingEvent(data []byte) (HandlingEvent, error) {
	var event HandlingEvent
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return HandlingEvent{}, fmt.Errorf("decode handling event: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return HandlingEvent{}, fmt.Errorf("handling event contains multiple JSON values")
		}
		return HandlingEvent{}, fmt.Errorf("decode handling event: %w", err)
	}
	canonical, err := MarshalCanonicalHandlingEvent(event)
	if err != nil {
		return HandlingEvent{}, err
	}
	if !bytes.Equal(data, canonical) {
		return HandlingEvent{}, fmt.Errorf("handling event is not canonical JSON")
	}
	return event, nil
}

// HandlingEventDigest returns the SHA-256 digest of an event's canonical
// representation. It identifies the event bytes, not the referenced artifact.
func HandlingEventDigest(event HandlingEvent) (string, error) {
	encoded, err := MarshalCanonicalHandlingEvent(event)
	if err != nil {
		return "", err
	}
	return artifact.DigestBytes(encoded), nil
}
