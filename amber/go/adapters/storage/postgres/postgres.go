// Package amberpostgres exposes Amber's optional PostgreSQL storage adapter.
//
// The implementation is kept alongside the storage contract for now, while
// this package gives applications an explicit vendor-specific import path.
// User-defined backends should import adapters/storage instead.
package amberpostgres

import amberstorage "github.com/0xsj/ingen/amber/adapters/storage"

// SQLDB is the database/sql executor accepted by PostgresStore.
type SQLDB = amberstorage.SQLDB

// PostgresStore is the optional PostgreSQL implementation of the generic
// storage contract.
type PostgresStore = amberstorage.PostgresStore

// PostgresSchema is the idempotent PostgreSQL table and index schema.
const PostgresSchema = amberstorage.PostgresSchema

// PostgresSchemaName identifies the PostgreSQL schema metadata row.
const PostgresSchemaName = amberstorage.PostgresSchemaName

// PostgresSchemaVersion identifies the PostgreSQL schema expected by the
// adapter.
const PostgresSchemaVersion = amberstorage.PostgresSchemaVersion

// ErrUnsupportedSchemaVersion indicates that the database schema needs an
// application-managed migration before this adapter can use it.
var ErrUnsupportedSchemaVersion = amberstorage.ErrUnsupportedSchemaVersion

// NewPostgresStore creates a PostgreSQL-backed store from a database/sql
// executor. The application supplies the driver and connection lifecycle.
func NewPostgresStore(db SQLDB) (*PostgresStore, error) {
	return amberstorage.NewPostgresStore(db)
}

// Compile-time reference ensures the alias remains compatible with the parent
// package's storage contract.
var _ amberstorage.Store = (*PostgresStore)(nil)
