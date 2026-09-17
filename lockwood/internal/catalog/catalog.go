package catalog

import (
	"fmt"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/custody"
)

type Query struct {
	CustodyID      string
	Digest         string
	RunID          string
	SourceURI      string
	SourceVersion  string
	ProducerTool   string
	ProducerKind   string
	Status         custody.Status
	MediaType      string
	LogicalName    string
	RetentionClass string
	ParentDigest   string
	ParentRelation custody.Relation
}

type Catalog struct {
	records custody.RecordStore
}

func (query Query) Validate() error {
	if query.Digest != "" {
		if err := artifact.ValidateDigest(query.Digest); err != nil {
			return err
		}
	}
	if query.ParentDigest != "" {
		if err := artifact.ValidateDigest(query.ParentDigest); err != nil {
			return fmt.Errorf("invalid parent digest: %w", err)
		}
	}
	if query.Status != "" {
		switch query.Status {
		case custody.Accepted, custody.Quarantined, custody.Rejected:
		default:
			return fmt.Errorf("invalid custody status %q", query.Status)
		}
	}
	if query.ParentRelation != "" {
		switch query.ParentRelation {
		case custody.References, custody.DerivedFrom, custody.Contains, custody.Verifies:
		default:
			return fmt.Errorf("invalid parent relation %q", query.ParentRelation)
		}
	}
	return nil
}

func New(records custody.RecordStore) (*Catalog, error) {
	if records == nil {
		return nil, fmt.Errorf("custody record store is required")
	}
	return &Catalog{records: records}, nil
}

func (c *Catalog) Inspect(custodyID string) (custody.Record, error) {
	return c.records.Get(custodyID)
}

func (c *Catalog) Find(query Query) ([]custody.Record, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}
	records, err := c.records.List()
	if err != nil {
		return nil, err
	}
	matched := make([]custody.Record, 0)
	for _, record := range records {
		if matches(record, query) {
			matched = append(matched, record)
		}
	}
	return matched, nil
}

func matches(record custody.Record, query Query) bool {
	return (query.CustodyID == "" || record.CustodyID == query.CustodyID) &&
		(query.Digest == "" || record.Artifact.Digest == query.Digest) &&
		(query.RunID == "" || record.Source.RunID == query.RunID) &&
		(query.SourceURI == "" || record.Source.URI == query.SourceURI) &&
		(query.SourceVersion == "" || record.Source.Version == query.SourceVersion) &&
		(query.ProducerTool == "" || record.Producer.Tool == query.ProducerTool) &&
		(query.ProducerKind == "" || record.Producer.Kind == query.ProducerKind) &&
		(query.Status == "" || record.Status == query.Status) &&
		(query.MediaType == "" || record.Artifact.MediaType == query.MediaType) &&
		(query.LogicalName == "" || record.Artifact.LogicalName == query.LogicalName) &&
		(query.RetentionClass == "" || record.Handling.RetentionClass == query.RetentionClass) &&
		matchesParent(record, query.ParentDigest, query.ParentRelation)
}

func matchesParent(record custody.Record, digest string, relation custody.Relation) bool {
	if digest == "" && relation == "" {
		return true
	}
	for _, parent := range record.Parents {
		if (digest == "" || parent.Digest == digest) && (relation == "" || parent.Relation == relation) {
			return true
		}
	}
	return false
}
