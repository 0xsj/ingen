# The OpenTelemetry bridge should be proven against a real SDK span

An adapter test double can verify that Amber emits the intended keys, but it
cannot prove that the values survive a real tracing SDK's span lifecycle.

## Origin

The OpenTelemetry adapter now enriches existing spans, so the next boundary to
verify is the runtime integration between Amber, an SDK tracer, and an ended
span record.

## What

The Go OTel adapter test creates an SDK tracer provider with
`tracetest.NewSpanRecorder`, starts a real SDK span, applies Amber attributes
through `SetCurrentSpanAttributes`, ends the span, and inspects the recorder's
ended span. The TypeScript test uses `BasicTracerProvider`,
`SimpleSpanProcessor`, and `InMemorySpanExporter` to perform the equivalent
check against a real JS SDK span. Both tests use in-memory recording only; they
do not configure a collector, network destination, or global tracer provider.

## Why

This keeps both adapter contracts concrete at the point where applications
actually observe telemetry: an ended SDK span. It also preserves the adapters'
ownership boundary. Amber enriches the span, while the application remains
responsible for tracer setup, sampling, lifecycle, and export configuration.

## Example

```go
ctx, span := tracer.Start(ctx, "process-order")
defer span.End()

if err := amberotel.SetCurrentSpanAttributes(ctx, provenance); err != nil {
	return err
}
```

## Gotchas

- The integration proof uses each SDK's in-memory testing recorder; production
  applications still choose their own exporter and collector configuration.
- The SDK dependencies are development/test dependencies. The Go core remains
  independent of OpenTelemetry, and the TypeScript bridge remains structural
  with no runtime OTel dependency.
- Amber attributes enrich an existing OTel span; they do not replace OTel trace
  or span identifiers.

## Used in

- [`go/adapters/otel/otel_integration_test.go`](../../go/adapters/otel/otel_integration_test.go)
- [`go/adapters/otel/otel.go`](../../go/adapters/otel/otel.go)
- [`typescript/src/otel.integration.test.ts`](../../typescript/src/otel.integration.test.ts)
- [`typescript/src/otel.ts`](../../typescript/src/otel.ts)
- [`docs/notes/029-opentelemetry-adapter.md`](029-opentelemetry-adapter.md)

## Related

- [OpenTelemetry should enrich existing spans without entering the core](029-opentelemetry-adapter.md)
- [The tracing projection should not decide span lifecycle semantics](009-framework-neutral-tracing-projection.md)
