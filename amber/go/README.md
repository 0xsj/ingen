# Amber Go SDK

The Go module provides the language-level implementation of Amber's v1
provenance model, context propagation, validation, and adapter contracts.

Install the released module with:

```sh
go get github.com/0xsj/ingen/amber
```

Start with the cross-language [getting started guide](../docs/getting-started.md).
For application-owned backends and integrations, see the
[adapter authoring guide](../docs/adapter-authoring.md).
Custom storage implementations can run the reusable
[`adapters/storage/contracttest`](adapters/storage/contracttest/) helper in
their tests.
The public module also includes optional adapters under `adapters/`:

- `adapters/http` for standard `net/http` boundaries;
- `adapters/messaging` for broker-neutral message metadata;
- `adapters/logging` and `adapters/tracing` for stable projections;
- `adapters/storage` for memory, file, and generic key-value storage;
- `adapters/storage/postgres` for the optional PostgreSQL implementation; and
- `adapters/otel` for optional OpenTelemetry span enrichment.

From the repository root, run `make check` during development and
`make release-check` before a release candidate. The PostgreSQL adapter is
opt-in; its migration, readiness, and live integration sequence is documented
in [`RELEASE.md`](../RELEASE.md).
