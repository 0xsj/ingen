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

## Planned capabilities

- immutable provenance values and validated transitions;
- separate logical work and execution identities;
- correlation, causation, origin, depth, and attempt tracking;
- actor, delegation, and tenant attribution;
- retry and replay semantics;
- typed references and bounded causal links;
- safe incoming-context inspection and restoration;
- Go and TypeScript implementations with shared behavior;
- transport, logging, tracing, and persistence adapters;
- cross-language conformance fixtures.

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

Core specification draft v1 is in place under `spec/`. The Go and TypeScript
implementations, conformance fixtures, and adapters will follow the shared
contract defined there. The first HTTP adapter is now implemented under
`adapters/http/`, including request/response middleware for the standard HTTP
runtimes. The transport-neutral messaging adapter is implemented
under `adapters/messaging/`. A framework-neutral structured logging projection
is implemented under `adapters/logging/`, and a framework-neutral tracing
projection is implemented under `adapters/tracing/`. A process-local reference
storage adapter and a Go file-backed store are implemented under
`adapters/storage/`; database-specific adapters are still future work.
Runnable end-to-end composition examples are available through `make examples`.
Incoming trust validators are available as opt-in hooks; v1 transport values
remain unsigned by default.
The v1 core is now at a foundation checkpoint: its core semantics are stable,
while production-specific integrations remain intentionally separate.

Implementation reasoning and verification notes are indexed in
[`docs/notes/README.md`](docs/notes/README.md).

## Verification

Run the complete Amber suite from the project root:

```sh
make test
```

This runs the Go packages, TypeScript build/tests, and shared conformance
fixtures.

The broader check gate also validates Go vet, TypeScript typechecking, and the
publishable TypeScript package artifact:

```sh
make check
```

That gate also checks that an external Go module can import the local Amber
module through its public module path.

Run the full check gate, including Go vet and TypeScript typechecking, with:

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

GitHub Actions runs the check gate, race detector, and composition examples on
pushes to `main` and on pull requests.
