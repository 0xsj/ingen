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
    ON amber_provenance (correlation_id, sequence);
