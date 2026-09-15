# OpenTelemetry should enrich existing spans without entering the core

OpenTelemetry is valuable operationally, but Amber provenance and trace
telemetry remain different contracts.

## Origin

Amber already exposed a stable tracing projection, but applications still had
to manually convert its map into OpenTelemetry attributes. The next adapter
should make that integration convenient without making the core model depend on
a tracing SDK or taking ownership of span lifecycle.

## What

The Go adapter in [`go/adapters/otel`](../../go/adapters/otel/) converts the
stable `amber.*` projection into typed OpenTelemetry `attribute.KeyValue`
values, applies them to an existing span, and can enrich the span explicitly
stored in a Go context. The TypeScript adapter in
[`typescript/src/otel.ts`](../../typescript/src/otel.ts) accepts the small
`setAttributes` surface shared by OpenTelemetry spans, leaving
`@opentelemetry/api` optional for consumers.

Both adapters validate provenance first and never create, end, or replace
spans. They do not map Amber IDs onto OTel trace or span IDs. Depth and attempt
are numeric OTel attributes; the Go adapter returns an error if a valid Amber
`uint64` value cannot fit OTel's signed `int64` attribute representation.
The shared OTel fixture is consumed by both SDKs so the typed Go values and the
TypeScript projection remain semantically aligned.

The Go adapter also has an integration test backed by the real OpenTelemetry
SDK's in-memory span recorder. It starts and ends an SDK span, applies Amber's
attributes through the context helper, and verifies the ended span contains
the expected values without requiring an exporter or collector.

## Why

Amber captures application-level work and causal history, while OTel captures
runtime trace telemetry. Enriching an existing span makes both views searchable
without allowing instrumentation details to redefine Amber semantics. Keeping
the TypeScript bridge structural follows OTel's library guidance: a library can
use the API surface without initializing an SDK or exporter.

## Example

```go
ctx, span := tracer.Start(ctx, "process-order")
defer span.End()
if err := amberotel.SetCurrentSpanAttributes(ctx, provenance); err != nil {
    return err
}
```

```ts
setProvenanceAttributes(activeSpan, provenance);
```

## Gotchas

- The adapter does not create spans, choose names, manage sampling, or export
  telemetry.
- Go's OTel package is an opt-in adapter dependency; the Amber core does not
  import it.
- The TypeScript bridge deliberately does not import `@opentelemetry/api`, so
  applications should provide their own OTel API/SDK setup.
- Amber's application IDs are not substitutes for OTel trace and span IDs.

## Used in

- [`go/adapters/otel/otel.go`](../../go/adapters/otel/otel.go)
- [`go/adapters/otel/otel_test.go`](../../go/adapters/otel/otel_test.go)
- [`go/adapters/otel/otel_integration_test.go`](../../go/adapters/otel/otel_integration_test.go)
- [`typescript/src/otel.ts`](../../typescript/src/otel.ts)
- [`typescript/src/otel.test.ts`](../../typescript/src/otel.test.ts)
- [`conformance/otel-v1.json`](../../conformance/otel-v1.json)
- [`adapters/tracing/README.md`](../../adapters/tracing/README.md)

## Related

- [The tracing projection should not decide span lifecycle semantics](009-framework-neutral-tracing-projection.md)
- [The v1 foundation should have an explicit freeze boundary](028-v1-foundation-checkpoint.md)
- [The package artifact should pass a clean consumer smoke test](026-package-install-smoke-test.md)
