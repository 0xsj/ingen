// Package store persists Hammond governance records without exposing the
// storage layout to the governance domain.
package store

import (
	"errors"

	"ingen/hammond/internal/governance"
)

var ErrNotFound = errors.New("Hammond governance record not found")

type Store interface {
	Register(record governance.Record) error
	AppendEvent(identity governance.ContractIdentity, event governance.Event) (governance.Record, error)
	CreateAmendment(identity governance.ContractIdentity, successor governance.Record, event governance.Event) (governance.Record, error)
	Supersede(identity governance.ContractIdentity, successor governance.ContractIdentity, event governance.Event) (governance.Record, error)
	Get(identity governance.ContractIdentity) (governance.Record, error)
	List() ([]governance.Record, error)
}
