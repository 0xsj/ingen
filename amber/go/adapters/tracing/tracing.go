// Package ambertracing provides a tracing-library-neutral provenance projection.
package ambertracing

import amber "github.com/0xsj/ingen/amber"

// ToAttributes returns the default tracing attribute projection for provenance.
func ToAttributes(provenance amber.Provenance) (map[string]any, error) {
	if err := provenance.Validate(); err != nil {
		return nil, err
	}
	attributes := map[string]any{
		"amber.version":        provenance.Version(),
		"amber.work_id":        provenance.WorkID().String(),
		"amber.execution_id":   provenance.ExecutionID().String(),
		"amber.correlation_id": provenance.CorrelationID().String(),
		"amber.origin":         string(provenance.Origin()),
		"amber.depth":          provenance.Depth(),
		"amber.attempt":        provenance.Attempt(),
		"amber.mode.kind":      string(provenance.Mode().Kind),
	}

	mode := provenance.Mode()
	if mode.OfExecutionID != "" {
		attributes["amber.mode.of_execution_id"] = mode.OfExecutionID.String()
	}
	if mode.ReplayID != "" {
		attributes["amber.mode.replay_id"] = mode.ReplayID.String()
	}
	if causation, ok := provenance.Causation(); ok {
		attributes["amber.causation.kind"] = causation.Kind
		attributes["amber.causation.id"] = causation.ID.String()
		if causation.Type != "" {
			attributes["amber.causation.type"] = causation.Type
		}
	}
	return attributes, nil
}

// MergeAttributes returns a cloned attribute map with Amber attributes
// applied. The existing map is never mutated.
func MergeAttributes(existing map[string]any, provenance amber.Provenance) (map[string]any, error) {
	attributes, err := ToAttributes(provenance)
	if err != nil {
		return nil, err
	}
	merged := make(map[string]any, len(existing)+len(attributes))
	for key, value := range existing {
		merged[key] = value
	}
	for key, value := range attributes {
		merged[key] = value
	}
	return merged, nil
}
