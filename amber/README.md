# Amber

Amber is InGen's portable provenance library for Go and TypeScript applications.

It provides a ready-to-use model for tracking logical work, individual
executions, causal relationships, attribution, retries, replay, and safe
context propagation. Applications should be able to adopt provenance without
hand-rolling incompatible request, correlation, causation, and execution IDs.

## Purpose

Amber answers questions such as:

- What logical work does this operation belong to?
- Which execution or retry produced this record?
- What caused this work to start?
- Who initiated it, who executed it, and on whose behalf?
- Which upstream work, event, or artifact does it relate to?
- Can this execution be replayed or inspected without rewriting history?

Amber is application-level provenance. It complements distributed tracing and
structured logging, but it does not replace them.

## Implemented capabilities

- immutable provenance values and validated transitions;
- separate logical work and execution identities;
- correlation, causation, origin, depth, and attempt tracking;
- actor, delegation, and tenant attribution;
- retry and replay semantics;
- typed references and bounded causal links;
- safe incoming-context inspection and restoration;
- Go and TypeScript implementations with shared behavior;
- transport, logging, tracing, and persistence adapters;
- cross-language conformance fixtures;
- optional OpenTelemetry bridges that enrich application-owned spans;
- an optional PostgreSQL adapter with an explicit migration and readiness
  boundary;
- generic storage seams so applications can provide their own backend.

## Boundaries

Amber describes relationships and attribution. It does not grant authority,
perform authorization, prove correctness, guarantee durable storage, or make
claims about the completeness of host-level telemetry.

The core model should remain independent of HTTP frameworks, message brokers,
databases, logging libraries, tracing SDKs, and application domains. Those
integrations belong in adapters.

## Project layout

```text
amber/
  spec/          Language-neutral model and wire contracts
  go/            Go SDK
  typescript/    TypeScript SDK
  conformance/   Cross-language fixtures and scenarios
  adapters/      HTTP, messaging, logging, tracing, and storage integrations
  examples/      Runnable composition examples
  docs/          Guides, examples, design notes, and implementation notes
```

## Status

Core and storage v1 specifications are in place under `spec/`. The Go and
TypeScript implementations, conformance fixtures, and adapters follow the
shared contracts defined there. The HTTP adapter under `adapters/http/`
includes request/response middleware for standard HTTP runtimes. The
transport-neutral messaging adapter is implemented under
`adapters/messaging/`. Framework-neutral structured logging and tracing
projections are implemented under `adapters/logging/` and
`adapters/tracing/`. Process-local, file-backed, and generic key-value storage
seams are implemented under `adapters/storage/`. The optional PostgreSQL
integration has the explicit Go package path
`go/adapters/storage/postgres/`; other database-specific adapters remain
future work.
The file and PostgreSQL persistence formats expose storage schema versions
separate from Amber's provenance wire version, leaving migrations explicit.
Runnable end-to-end composition and OpenTelemetry integration examples are
available through `make examples`.
Optional OpenTelemetry bridges enrich existing spans with the same `amber.*`
projection without making the base TypeScript package depend on an OTel runtime.
Incoming trust validators are available as opt-in hooks; v1 transport values
remain unsigned by default.
The v1 core is now at a foundation checkpoint: its core semantics are stable,
while production-specific integrations remain intentionally separate.
A release-readiness matrix records the verified, environment-dependent, and
out-of-scope portions of the project in
[`docs/release-readiness.md`](docs/release-readiness.md).

Contributor workflow and release handoff procedures are documented in
[`CONTRIBUTING.md`](CONTRIBUTING.md) and [`RELEASE.md`](RELEASE.md).

The open version and tag decisions are recorded in
[`VERSIONING.md`](VERSIONING.md).
Deferred release operations and future consumer-driven work are tracked in
[`BACKLOG.md`](BACKLOG.md).

New users can start with the cross-language
[`docs/getting-started.md`](docs/getting-started.md) guide.
Teams building their own persistence or boundary integrations can use the
[`docs/adapter-authoring.md`](docs/adapter-authoring.md) guide.

## Release readiness

The local release-candidate gate currently passes:

```sh
make release-check
```

The PostgreSQL adapter was also verified locally against PostgreSQL 16 after
applying `go/adapters/storage/postgres/migrations/001_amber_provenance.sql`.
Before a deployment, the owner must apply that migration through application
migration tooling, run `make postgres-schema-check`, and run the live
integration check against the managed PostgreSQL version. Hosted GitHub Actions
results and npm publishing still require repository and release credentials.

Implementation reasoning and verification notes are indexed in
[`docs/notes/README.md`](docs/notes/README.md).

## Verification

Run the complete Amber suite from the project root:

```sh
make test
```

This runs the Go packages, TypeScript build/tests, and shared conformance
fixtures.

The broader check gate also validates Go vet, TypeScript typechecking, the
publishable TypeScript package artifact, and an external Go module consumer:

```sh
make check
```

Run the composition examples from the project root with:

```sh
make examples
```

Run the short local fuzz pass for core and transport decoders with:

```sh
make fuzz
```

Run Go’s race detector across the SDK and adapters with:

```sh
make race
```

Run the complete release-candidate gate, including checks, race detection,
fuzzing, and examples, with:

```sh
make release-check
```

GitHub Actions runs the check gate, race detector, composition examples, and a
separate PostgreSQL service-backed integration and read-only schema readiness
job on pushes to `main` and on pull requests.
