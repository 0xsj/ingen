# OpenTelemetry integration should have a runnable application path

Tests prove an adapter contract, but an example shows where application code
owns tracer setup, span lifecycle, and export configuration.

## Origin

Both Amber OpenTelemetry bridges now pass real SDK-backed integration tests.
The next usability boundary is a copyable, runnable flow that does not require
an external collector or observability backend.

## What

The Go and TypeScript OpenTelemetry examples each create an application-owned
SDK tracer provider with an in-memory recorder, start a `process-order` span,
apply Amber provenance attributes, end the span, and verify the finished span
contains `amber.execution_id`. `make examples` runs the original composition
examples plus both OTel examples.

## Why

This makes the intended ownership boundary visible in executable code. Amber
does not initialize a global provider, choose sampling, manage export, or
create the span. The application does those things and calls Amber to enrich
the span it already owns.

## Example

```sh
make examples
```

The OTel examples print a successful finished-span count and confirm that the
Amber execution attribute was recorded.

## Gotchas

- The examples use in-memory SDK recorders so they remain deterministic and
  offline; production applications still configure their own exporter and
  collector.
- The TypeScript example uses development-only OTel dependencies. The
  published Amber runtime remains free of an OTel import and does not require
  an OTel SDK to load.
- The examples pass an explicit span to Amber. Context managers and active-span
  lookup are application/runtime concerns.

## Used in

- [`go/examples/otel/main.go`](../../go/examples/otel/main.go)
- [`typescript/src/otel-example.ts`](../../typescript/src/otel-example.ts)
- [`Makefile`](../../Makefile)
- [`examples/README.md`](../../examples/README.md)

## Related

- [The OpenTelemetry bridge should be proven against a real SDK span](030-opentelemetry-sdk-integration.md)
- [OpenTelemetry should enrich existing spans without entering the core](029-opentelemetry-adapter.md)
- [A runnable composition example should cross the adapters without external services](015-end-to-end-composition-example.md)
