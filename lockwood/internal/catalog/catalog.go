package catalog

import (
	"fmt"

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
	records *custody.Filesystem
}

func New(records *custody.Filesystem) (*Catalog, error) {
	if records == nil {
		return nil, fmt.Errorf("custody record store is required")
	}
	return &Catalog{records: records}, nil
}

func (c *Catalog) Inspect(custodyID string) (custody.Record, error) {
	return c.records.Get(custodyID)
}

func (c *Catalog) Find(query Query) ([]custody.Record, error) {
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
