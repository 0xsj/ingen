package sattler

// StableChangeID returns the deterministic key for one observable field. It
// identifies the kind of change, not a particular before/after value.
func StableChangeID(category, field string) string {
	return category + "." + field
}

// NewChange constructs an observable change with its stable key populated.
func NewChange(category, field string, before, after any) Change {
	return Change{
		ID:       StableChangeID(category, field),
		Category: category,
		Field:    field,
		Before:   before,
		After:    after,
	}
}

// StableID returns the stored key, or derives it for a manually constructed
// Change value.
func (change Change) StableID() string {
	if change.ID != "" {
		return change.ID
	}
	return StableChangeID(change.Category, change.Field)
}
