package provenance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

func validateAmberShape(data []byte) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil || object == nil {
		return fmt.Errorf("Amber context must be a JSON object")
	}
	allowed := map[string]bool{"version": true, "work_id": true, "execution_id": true, "correlation_id": true, "causation": true, "origin": true, "depth": true, "attempt": true, "attribution": true, "mode": true, "references": true}
	for key := range object {
		if !allowed[key] {
			return fmt.Errorf("Amber context contains unsupported field %q", key)
		}
	}
	for _, key := range []string{"version", "work_id", "execution_id", "correlation_id", "origin", "depth", "attempt", "mode"} {
		if _, ok := object[key]; !ok {
			return fmt.Errorf("Amber context is missing required field %q", key)
		}
	}
	if err := checkObjectFields(object["mode"], "mode", []string{"kind", "of_execution_id", "replay_id"}, []string{"kind"}); err != nil {
		return err
	}
	if raw, exists := object["causation"]; exists {
		if isNull(raw) {
			return fmt.Errorf("Amber causation cannot be null when present")
		}
		if err := checkObjectFields(raw, "causation", []string{"kind", "id", "type"}, []string{"kind", "id"}); err != nil {
			return err
		}
	}
	if raw, exists := object["attribution"]; exists {
		if isNull(raw) {
			return fmt.Errorf("Amber attribution cannot be null when present")
		}
		var attribution map[string]json.RawMessage
		if err := json.Unmarshal(raw, &attribution); err != nil || attribution == nil {
			return fmt.Errorf("Amber attribution must be an object")
		}
		if err := onlyFields(attribution, []string{"initiated_by", "executed_by", "on_behalf_of", "tenant_id"}, "attribution"); err != nil {
			return err
		}
		for _, actorName := range []string{"initiated_by", "executed_by", "on_behalf_of"} {
			if actor, ok := attribution[actorName]; ok {
				if isNull(actor) {
					return fmt.Errorf("Amber attribution.%s cannot be null", actorName)
				}
				if err := checkObjectFields(actor, "attribution."+actorName, []string{"id", "type"}, []string{"id", "type"}); err != nil {
					return err
				}
			}
		}
	}
	if raw, exists := object["references"]; exists {
		if isNull(raw) {
			return fmt.Errorf("Amber references cannot be null when present")
		}
		var references []json.RawMessage
		if err := json.Unmarshal(raw, &references); err != nil || references == nil {
			return fmt.Errorf("Amber references must be an array")
		}
		for i, reference := range references {
			if err := checkObjectFields(reference, fmt.Sprintf("references[%d]", i), []string{"type", "id", "relation"}, []string{"type", "id"}); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkObjectFields(raw json.RawMessage, name string, allowed, required []string) error {
	if isNull(raw) {
		return fmt.Errorf("Amber %s cannot be null", name)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return fmt.Errorf("Amber %s must be an object", name)
	}
	allowedSet := make(map[string]bool, len(allowed))
	for _, field := range allowed {
		allowedSet[field] = true
	}
	for field := range object {
		if !allowedSet[field] {
			return fmt.Errorf("Amber %s contains unsupported field %q", name, field)
		}
	}
	for _, field := range required {
		if _, ok := object[field]; !ok {
			return fmt.Errorf("Amber %s is missing required field %q", name, field)
		}
	}
	return nil
}

func onlyFields(object map[string]json.RawMessage, fields []string, name string) error {
	allowed := make(map[string]bool, len(fields))
	for _, field := range fields {
		allowed[field] = true
	}
	for field := range object {
		if !allowed[field] {
			return fmt.Errorf("Amber %s contains unsupported field %q", name, field)
		}
	}
	return nil
}

func isNull(raw json.RawMessage) bool { return bytes.Equal(bytes.TrimSpace(raw), []byte("null")) }

func rejectDuplicateKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := scanJSON(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("Amber context has trailing JSON")
	}
	return nil
}

func scanJSON(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token == nil {
		return fmt.Errorf("Amber v1 wire context cannot contain null values")
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			raw, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := raw.(string)
			if !ok || seen[key] {
				return fmt.Errorf("Amber context has malformed or duplicate object key")
			}
			seen[key] = true
			if err := scanJSON(decoder); err != nil {
				return err
			}
		}
		closeToken, err := decoder.Token()
		if err != nil || closeToken != json.Delim('}') {
			return fmt.Errorf("malformed Amber JSON object")
		}
		return nil
	case '[':
		for decoder.More() {
			if err := scanJSON(decoder); err != nil {
				return err
			}
		}
		closeToken, err := decoder.Token()
		if err != nil || closeToken != json.Delim(']') {
			return fmt.Errorf("malformed Amber JSON array")
		}
		return nil
	default:
		return fmt.Errorf("malformed Amber JSON delimiter")
	}
}
