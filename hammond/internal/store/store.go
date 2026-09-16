// Package store persists Hammond governance records without exposing the
// storage layout to the governance domain.
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"ingen/hammond/internal/governance"
)

var ErrNotFound = errors.New("Hammond governance record not found")
var ErrConflict = errors.New("Hammond governance record revision conflict")

// RecordRevision returns the deterministic revision token for a materialized
// record. It is a caller-supplied precondition for conditional mutations, not
// a field persisted in the governance artifact.
func RecordRevision(record governance.Record) (string, error) {
	data, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

type Store interface {
	Register(record governance.Record) error
	AppendEvent(identity governance.ContractIdentity, event governance.Event) (governance.Record, error)
	CreateAmendment(identity governance.ContractIdentity, successor governance.Record, event governance.Event) (governance.Record, error)
	Supersede(identity governance.ContractIdentity, successor governance.ContractIdentity, event governance.Event) (governance.Record, error)
	Get(identity governance.ContractIdentity) (governance.Record, error)
	List() ([]governance.Record, error)
}

// ConditionalStore adds optimistic concurrency to Hammond mutation paths.
// Callers obtain a revision from Get and must retry from a fresh record after
// ErrConflict; Hammond does not invent a semantic event merge.
type ConditionalStore interface {
	Store
	AppendEventIfRevision(identity governance.ContractIdentity, expectedRevision string, event governance.Event) (governance.Record, error)
	CreateAmendmentIfRevision(identity governance.ContractIdentity, expectedRevision string, successor governance.Record, event governance.Event) (governance.Record, error)
	SupersedeIfRevision(identity governance.ContractIdentity, expectedRevision string, successor governance.ContractIdentity, event governance.Event) (governance.Record, error)
}
