// Package storage defines persistence for completed Nublar runs.
package storage

import "ingen/nublar/internal/run"

// Store persists immutable Nublar run records.
type Store interface {
	Save(record run.Run) error
	Load(runID string) (run.Run, error)
	List() ([]run.Run, error)
}
