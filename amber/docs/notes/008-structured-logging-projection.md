# The default logging projection should favor stable identity over optional domain data

Structured logs should expose enough provenance to correlate and diagnose work
without automatically copying potentially sensitive attribution or references.

## Origin

The logging adapter needed to integrate with arbitrary structured loggers while
preserving the core model's distinction between stable provenance fields and
application-specific data.

## What

Both SDKs project identity, flow, origin, depth, attempt, mode, and causation
into `amber.*` fields. Retry and replay source IDs are included when present.
Attribution and typed references are omitted by default and can be added by an
application that has made an explicit privacy decision. Merging returns a new
field map rather than mutating an existing record.

## Why

Logging every provenance field by default would make a low-level adapter decide
that actor IDs, tenant IDs, or domain references are safe for every application
and retention policy. Omitting optional data gives the projection a stable,
portable baseline while keeping deliberate enrichment possible. Flattened
fields also work across loggers without requiring a nested-object convention.

## Example

```text
amber.work_id        = ...
amber.execution_id   = ...
amber.correlation_id = ...
amber.origin         = retry
amber.attempt        = 2
amber.mode.kind      = retry
amber.mode.of_execution_id = ...
```

## Gotchas

- A logging projection is descriptive observability data, not a trace span and
  not proof of sender identity or authorization.
- `amber.*` keys intentionally override stale values when merged, but the
  caller's original field map remains unchanged.
- Optional attribution and references are not lost from the provenance value;
  they are simply not emitted by the default projection.
- The current tracing projection uses the same default fields, but an
  SDK-specific span adapter may still apply different naming or cardinality
  rules.

## Used in

- [`adapters/logging/README.md`](../../adapters/logging/README.md)
- [`go/adapters/logging/logging.go`](../../go/adapters/logging/logging.go)
- [`go/adapters/logging/logging_test.go`](../../go/adapters/logging/logging_test.go)
- [`typescript/src/logging.ts`](../../typescript/src/logging.ts)
- [`typescript/src/logging.test.ts`](../../typescript/src/logging.test.ts)
- [`conformance/logging-v1.json`](../../conformance/logging-v1.json)

## Related

- [The shared contract must precede both SDKs](001-spec-first-foundation.md)
- [The HTTP adapter should preserve the core JSON inside a transport-safe envelope](006-http-header-adapter.md)
