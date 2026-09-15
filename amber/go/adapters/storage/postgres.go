package amberstorage

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	amber "github.com/0xsj/ingen/amber"
)

// SQLDB is the small database/sql surface needed by PostgresStore. *sql.DB
// and *sql.Tx both satisfy it, so callers can use the store with a pool or an
// application-managed transaction.
type SQLDB interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// PostgresSchema creates the table and indexes used by PostgresStore. It is
// safe to execute repeatedly and can also be applied by an application's
// migration system instead of calling EnsureSchema.
const PostgresSchema = `
CREATE TABLE IF NOT EXISTS amber_provenance_schema (
    schema_name TEXT PRIMARY KEY,
    schema_version INTEGER NOT NULL
);
INSERT INTO amber_provenance_schema (schema_name, schema_version)
VALUES ('amber_provenance', 1)
ON CONFLICT (schema_name) DO NOTHING;
CREATE TABLE IF NOT EXISTS amber_provenance (
    sequence BIGSERIAL PRIMARY KEY,
    execution_id TEXT NOT NULL UNIQUE,
    work_id TEXT NOT NULL,
    correlation_id TEXT NOT NULL,
    causation_kind TEXT,
    causation_id TEXT,
    value_json JSONB NOT NULL
);
CREATE INDEX IF NOT EXISTS amber_provenance_work_id_sequence_idx
    ON amber_provenance (work_id, sequence);
CREATE INDEX IF NOT EXISTS amber_provenance_causation_sequence_idx
    ON amber_provenance (causation_kind, causation_id, sequence);
CREATE INDEX IF NOT EXISTS amber_provenance_correlation_sequence_idx
    ON amber_provenance (correlation_id, sequence);`

const (
	postgresInsertQuery = `
INSERT INTO amber_provenance
    (execution_id, work_id, correlation_id, causation_kind, causation_id, value_json)
VALUES ($1, $2, $3, $4, $5, $6::jsonb)
ON CONFLICT (execution_id) DO NOTHING`
	postgresGetQuery = `
SELECT value_json
FROM amber_provenance
WHERE execution_id = $1`
	postgresWorkQuery = `
SELECT value_json
FROM amber_provenance
WHERE work_id = $1
ORDER BY
    (value_json->>'depth')::NUMERIC ASC,
    (value_json->>'attempt')::NUMERIC ASC,
    execution_id ASC`
	postgresCausationQuery = `
SELECT value_json
FROM amber_provenance
WHERE causation_kind = $1 AND causation_id = $2
ORDER BY
    (value_json->>'depth')::NUMERIC ASC,
    (value_json->>'attempt')::NUMERIC ASC,
    execution_id ASC`
	postgresCorrelationQuery = `
SELECT value_json
FROM amber_provenance
WHERE correlation_id = $1
ORDER BY
    (value_json->>'depth')::NUMERIC ASC,
    (value_json->>'attempt')::NUMERIC ASC,
    execution_id ASC`
)

const (
	// PostgresSchemaName identifies the metadata row used to validate the
	// PostgreSQL storage schema before the adapter serves data.
	PostgresSchemaName = "amber_provenance"
	// PostgresSchemaVersion identifies the PostgreSQL tables and query shape
	// created by PostgresSchema.
	PostgresSchemaVersion      = 1
	postgresSchemaVersionQuery = `
SELECT schema_version
FROM amber_provenance_schema
WHERE schema_name = $1`
)

// PostgresStore persists provenance in a PostgreSQL database through the
// database/sql package. The adapter stores the complete v1 JSON value and
// keeps query columns for indexed work, causation, and correlation lookups.
type PostgresStore struct {
	db SQLDB
}

// NewPostgresStore creates a PostgreSQL-backed Store from a database/sql
// executor. It does not open connections or create schema automatically.
func NewPostgresStore(db SQLDB) (*PostgresStore, error) {
	if db == nil {
		return nil, fmt.Errorf("%w: postgres database cannot be nil", amber.ErrInvalidTransition)
	}
	return &PostgresStore{db: db}, nil
}

// EnsureSchema creates the PostgresStore table and indexes if they do not
// already exist.
func (s *PostgresStore) EnsureSchema(ctx context.Context) error {
	if err := s.validate(); err != nil {
		return err
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, PostgresSchema); err != nil {
		return fmt.Errorf("create amber postgres schema: %w", err)
	}
	return s.CheckSchema(ctx)
}

// CheckSchema verifies the PostgreSQL schema metadata without creating or
// altering any tables. Applications can use it as a read-only readiness check
// after migrations and distinguish a required migration with
// errors.Is(err, ErrUnsupportedSchemaVersion).
func (s *PostgresStore) CheckSchema(ctx context.Context) error {
	if err := s.validate(); err != nil {
		return err
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	rows, err := s.db.QueryContext(ctx, postgresSchemaVersionQuery, PostgresSchemaName)
	if err != nil {
		return fmt.Errorf("read amber postgres schema version: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return fmt.Errorf("read amber postgres schema version: %w", err)
		}
		return fmt.Errorf("%w: postgres schema metadata is missing", ErrUnsupportedSchemaVersion)
	}
	var version int
	if err := rows.Scan(&version); err != nil {
		return fmt.Errorf("scan amber postgres schema version: %w", err)
	}
	if version != PostgresSchemaVersion {
		return fmt.Errorf("%w: postgres schema version %d, want %d", ErrUnsupportedSchemaVersion, version, PostgresSchemaVersion)
	}
	return nil
}

func (s *PostgresStore) Put(ctx context.Context, provenance amber.Provenance) error {
	if err := s.validate(); err != nil {
		return err
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := provenance.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(provenance)
	if err != nil {
		return err
	}

	var causationKind, causationID any
	if causation, ok := provenance.Causation(); ok {
		causationKind = causation.Kind
		causationID = causation.ID.String()
	}
	result, err := s.db.ExecContext(
		ctx,
		postgresInsertQuery,
		provenance.ExecutionID().String(),
		provenance.WorkID().String(),
		provenance.CorrelationID().String(),
		causationKind,
		causationID,
		string(data),
	)
	if err != nil {
		return fmt.Errorf("insert amber provenance: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect amber provenance insert: %w", err)
	}
	if rowsAffected > 0 {
		return nil
	}

	existing, err := s.Get(ctx, provenance.ExecutionID())
	if err != nil {
		return err
	}
	existingData, err := json.Marshal(existing)
	if err != nil {
		return err
	}
	if bytes.Equal(existingData, data) {
		return nil
	}
	return fmt.Errorf("%w: execution_id %s", ErrConflict, provenance.ExecutionID())
}

func (s *PostgresStore) Get(ctx context.Context, executionID amber.ID) (amber.Provenance, error) {
	if err := s.validate(); err != nil {
		return amber.Provenance{}, err
	}
	if err := contextError(ctx); err != nil {
		return amber.Provenance{}, err
	}
	rows, err := s.db.QueryContext(ctx, postgresGetQuery, executionID.String())
	if err != nil {
		return amber.Provenance{}, fmt.Errorf("query amber provenance: %w", err)
	}
	records, err := scanPostgresRows(rows)
	if err != nil {
		return amber.Provenance{}, err
	}
	if len(records) == 0 {
		return amber.Provenance{}, fmt.Errorf("%w: execution_id %s", ErrNotFound, executionID)
	}
	return records[0], nil
}

func (s *PostgresStore) ListByWorkID(ctx context.Context, workID amber.ID) ([]amber.Provenance, error) {
	return s.list(ctx, postgresWorkQuery, workID.String())
}

func (s *PostgresStore) ListByCausation(ctx context.Context, kind string, id amber.ID) ([]amber.Provenance, error) {
	return s.list(ctx, postgresCausationQuery, kind, id.String())
}

func (s *PostgresStore) ListByCorrelationID(ctx context.Context, correlationID amber.ID) ([]amber.Provenance, error) {
	return s.list(ctx, postgresCorrelationQuery, correlationID.String())
}

func (s *PostgresStore) list(ctx context.Context, query string, args ...any) ([]amber.Provenance, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query amber provenance history: %w", err)
	}
	return scanPostgresRows(rows)
}

func (s *PostgresStore) validate() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("%w: postgres database cannot be nil", amber.ErrInvalidTransition)
	}
	return nil
}

func scanPostgresRows(rows *sql.Rows) ([]amber.Provenance, error) {
	defer rows.Close()
	records := make([]amber.Provenance, 0)
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("scan amber provenance: %w", err)
		}
		provenance, err := amber.FromJSON(data)
		if err != nil {
			return nil, fmt.Errorf("decode amber provenance from postgres: %w", err)
		}
		records = append(records, provenance)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read amber provenance rows: %w", err)
	}
	return records, nil
}
