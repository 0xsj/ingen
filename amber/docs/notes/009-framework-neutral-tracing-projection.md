# The tracing projection should not decide span lifecycle semantics

Amber can provide stable provenance attributes without deciding when spans are
created, how they are parented, or which tracing SDK owns them.

## Origin

The tracing slice followed the logging projection, but the project does not yet
choose OpenTelemetry or another tracing dependency. A framework-neutral surface
was needed before making that integration decision.

## What

Both SDKs expose the stable `amber.*` identity, flow, origin, depth, attempt,
mode, and causation attributes through a map projection. They omit attribution
and typed references by default. Merging returns a new map, allowing a future
span adapter to apply the attributes to its own span representation.

## Why

Having the core create or own spans would couple provenance semantics to one
tracing runtime and could cause duplicate spans or conflicting parentage in
applications that already have instrumentation. A projection keeps Amber's
responsibility limited to describing provenance and leaves span lifecycle,
sampling, status, and exporter behavior to the tracing integration.

## Example

```text
Amber Provenance -> amber.* attributes -> application span adapter -> span
```

## Gotchas

- These attributes do not create a span or establish a trace parent.
- Work, execution, and correlation IDs are useful for lookup but may have high
  cardinality; the consuming tracing backend must apply its own policy.
- Attribution and references remain available on the provenance value even
  though the default projection omits them.
- The optional OpenTelemetry adapter enriches existing spans but does not create
  span lifecycle semantics; future tracing integrations may still need explicit
  semantic-convention and cardinality decisions.

## Used in

- [`adapters/tracing/README.md`](../../adapters/tracing/README.md)
- [`go/adapters/tracing/tracing.go`](../../go/adapters/tracing/tracing.go)
- [`go/adapters/tracing/tracing_test.go`](../../go/adapters/tracing/tracing_test.go)
- [`typescript/src/tracing.ts`](../../typescript/src/tracing.ts)
- [`typescript/src/tracing.test.ts`](../../typescript/src/tracing.test.ts)
- [`conformance/tracing-v1.json`](../../conformance/tracing-v1.json)

## Related

- [The default logging projection should favor stable identity over optional domain data](008-structured-logging-projection.md)
- [The shared contract must precede both SDKs](001-spec-first-foundation.md)
