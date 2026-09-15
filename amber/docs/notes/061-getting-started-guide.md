# Getting started guidance should show the smallest useful cross-language path

Amber has two SDKs and several optional adapters. New users need a short path
from creating a root provenance value to deriving a child without first
reading every adapter or design note.

## Origin

The root README described Amber's capabilities, and the TypeScript README had a
small example, but there was no shared Go/TypeScript adoption guide. The Go
module also lacked a package-level README.

## What

The project now provides:

- [`docs/getting-started.md`](../getting-started.md), a cross-language guide
  covering installation, root/child transitions, context, trust, storage, and
  verification;
- [`go/README.md`](../../go/README.md), which lists the Go module's adapter
  boundaries and points to the shared guide; and
- a link from [`typescript/README.md`](../../typescript/README.md) to the same
  shared walkthrough.

The root README links the guide alongside the contributor and release
runbooks.

## Why

This gives both SDKs one consistent first-use story while keeping framework,
database, and telemetry choices optional. It also directs readers to the
normative specifications and runnable examples at the point where they need
more detail.

## Example

The smallest adoption sequence is:

1. create a root value;
2. install it into the application's context boundary;
3. inspect the incoming value at a trusted boundary; and
4. derive a child for the new work.

The guide shows this sequence in both languages and then links to the HTTP,
storage, trust, and release paths.

## Gotchas

- The `go get` and `npm install` commands describe released dependency usage;
  repository contributors should follow [`CONTRIBUTING.md`](../../CONTRIBUTING.md).
- Amber transport values are structurally validated but unsigned by default;
  deployments that need trust decisions must provide their own validator.
- Adapter selection remains application-owned; the getting started guide does
  not make HTTP, PostgreSQL, OpenTelemetry, or a storage backend mandatory.

## Used in

- [`docs/getting-started.md`](../getting-started.md)
- [`go/README.md`](../../go/README.md)
- [`typescript/README.md`](../../typescript/README.md)
- [`README.md`](../../README.md)

## Related

- [Contributor and release runbooks should make the verified workflow repeatable](060-contributor-release-runbooks.md)
- [The shared contract must precede both SDKs](001-spec-first-foundation.md)
- [The first deployment vertical should prove a real Go HTTP service path](047-go-http-reference-vertical.md)
