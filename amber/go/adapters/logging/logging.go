// Package amberlogging provides a logging-library-neutral provenance projection.
package amberlogging

import amber "github.com/0xsj/ingen/amber"

// ToFields returns the default structured-log projection for provenance.
func ToFields(provenance amber.Provenance) (map[string]any, error) {
	if err := provenance.Validate(); err != nil {
		return nil, err
	}
	fields := map[string]any{
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
		fields["amber.mode.of_execution_id"] = mode.OfExecutionID.String()
	}
	if mode.ReplayID != "" {
		fields["amber.mode.replay_id"] = mode.ReplayID.String()
	}
	if causation, ok := provenance.Causation(); ok {
		fields["amber.causation.kind"] = causation.Kind
		fields["amber.causation.id"] = causation.ID.String()
		if causation.Type != "" {
			fields["amber.causation.type"] = causation.Type
		}
	}
	return fields, nil
}

// MergeFields returns a cloned structured-log field map with Amber fields
// applied. The existing map is never mutated.
func MergeFields(existing map[string]any, provenance amber.Provenance) (map[string]any, error) {
	fields, err := ToFields(provenance)
	if err != nil {
		return nil, err
	}
	merged := make(map[string]any, len(existing)+len(fields))
	for key, value := range existing {
		merged[key] = value
	}
	for key, value := range fields {
		merged[key] = value
	}
	return merged, nil
}
