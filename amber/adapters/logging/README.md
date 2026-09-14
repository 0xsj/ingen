# Logging adapter

The Amber logging adapter projects stable provenance fields into a structured
log record without requiring a logging library.

## Default fields

The projection includes `amber.version`, `amber.work_id`,
`amber.execution_id`, `amber.correlation_id`, `amber.origin`, `amber.depth`,
`amber.attempt`, and `amber.mode.kind`. It adds source IDs for retry/replay
modes and cause fields when those values are present.

Attribution and typed references are omitted by default. Applications can add
those values deliberately when their logging and privacy policy permits it.

Merging returns a new field map; it does not mutate a caller's existing record.

## Implementations

- Go: [`go/adapters/logging`](../../go/adapters/logging/)
- TypeScript: [`typescript/src/logging.ts`](../../typescript/src/logging.ts)
- Shared field vector: [`conformance/logging-v1.json`](../../conformance/logging-v1.json)
