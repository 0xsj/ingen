package governance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// DecodeRecord parses one strict JSON governance record and applies domain
// validation before returning it to a caller.
func DecodeRecord(data []byte) (Record, error) {
	var record Record
	if err := decodeStrict(data, &record); err != nil {
		return Record{}, fmt.Errorf("decode Hammond record: %w", err)
	}
	if err := record.Validate(); err != nil {
		return Record{}, err
	}
	return record, nil
}

// DecodeEvent parses one strict JSON event. Contextual checks are applied when
// the event is appended to a specific record.
func DecodeEvent(data []byte) (Event, error) {
	var event Event
	if err := decodeStrict(data, &event); err != nil {
		return Event{}, fmt.Errorf("decode Hammond event: %w", err)
	}
	return event, nil
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not allowed")
		}
		return fmt.Errorf("trailing JSON is not allowed: %w", err)
	}
	return nil
}
