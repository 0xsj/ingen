package custody

import (
	"fmt"

	"ingen/lockwood/internal/artifact"
)

type Relation string

const (
	References  Relation = "references"
	DerivedFrom Relation = "derived-from"
	Contains    Relation = "contains"
	Verifies    Relation = "verifies"
)

type Lineage struct {
	Relation Relation `json:"relation"`
	Digest   string   `json:"digest"`
}

func (lineage Lineage) validate(childDigest string) error {
	if err := lineage.validateSyntax(); err != nil {
		return err
	}
	if lineage.Digest == childDigest {
		return fmt.Errorf("lineage cannot point an artifact at itself")
	}
	return nil
}

func (lineage Lineage) validateSyntax() error {
	switch lineage.Relation {
	case References, DerivedFrom, Contains, Verifies:
	default:
		return fmt.Errorf("unsupported lineage relation %q", lineage.Relation)
	}
	if err := artifact.ValidateDigest(lineage.Digest); err != nil {
		return fmt.Errorf("invalid lineage digest: %w", err)
	}
	return nil
}
