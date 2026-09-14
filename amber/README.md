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
  docs/          Guides, examples, design notes, and implementation notes
```

## Status

Core specification draft v1 is in place under `spec/`. The Go and TypeScript
implementations, conformance fixtures, and adapters will follow the shared
contract defined there.

Implementation reasoning and verification notes are indexed in
[`docs/notes/README.md`](docs/notes/README.md).
