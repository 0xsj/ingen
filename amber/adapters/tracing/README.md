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

The optional OpenTelemetry bridges apply the same projection to an existing
span. They do not create or end spans, replace trace identifiers, or require an
OpenTelemetry SDK in the base Amber package. Go uses OpenTelemetry's typed
`attribute.KeyValue` API; TypeScript accepts an OTel-compatible span shape so
`@opentelemetry/api` remains optional for library consumers.

## Implementations

- Go: [`go/adapters/tracing`](../../go/adapters/tracing/)
- Go OpenTelemetry: [`go/adapters/otel`](../../go/adapters/otel/)
- TypeScript: [`typescript/src/tracing.ts`](../../typescript/src/tracing.ts)
- TypeScript OpenTelemetry: [`typescript/src/otel.ts`](../../typescript/src/otel.ts)
- Shared attribute vector: [`conformance/tracing-v1.json`](../../conformance/tracing-v1.json)
- Shared OpenTelemetry vector: [`conformance/otel-v1.json`](../../conformance/otel-v1.json)

Both bridges are also exercised against real OpenTelemetry SDK tracers and
in-memory span recorders in
[`go/adapters/otel/otel_integration_test.go`](../../go/adapters/otel/otel_integration_test.go).
The TypeScript runtime check is in
[`typescript/src/otel.integration.test.ts`](../../typescript/src/otel.integration.test.ts).
