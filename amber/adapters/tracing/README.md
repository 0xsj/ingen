# Tracing adapter

The Amber tracing adapter projects stable provenance attributes into a generic
string-keyed attribute map. It does not depend on OpenTelemetry or another
tracing SDK; framework-specific span integration can consume this projection.

The default attributes include identity, flow, origin, depth, attempt, mode,
and causation using the `amber.*` namespace. Attribution and typed references
are omitted by default for the same privacy and cardinality reasons as the
logging projection.

Merging returns a new attribute map and never mutates an existing span-
attribute collection.

## Implementations

- Go: [`go/adapters/tracing`](../../go/adapters/tracing/)
- TypeScript: [`typescript/src/tracing.ts`](../../typescript/src/tracing.ts)
- Shared attribute vector: [`conformance/tracing-v1.json`](../../conformance/tracing-v1.json)
