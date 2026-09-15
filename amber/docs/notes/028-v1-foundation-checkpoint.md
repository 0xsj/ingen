# The v1 foundation should have an explicit freeze boundary

Amber is ready for production-specific vertical work when the core contract is
stable, verified, and clear about what it does not promise.

## Origin

The v1 model, both SDKs, adapters, storage seams, trust boundary, examples, and
release checks were implemented incrementally. Without an explicit checkpoint,
it would be easy to keep adding generic fields or integrations before a real
deployment target supplied the next requirement.

## What

The v1 stability boundary now treats the fields, transitions, enum values,
incoming policies, size limits, and unsigned-by-default trust model as stable
core surface. Unknown JSON fields are accepted for forward compatibility but
may be omitted on re-encode because the current SDKs are not lossless decoders.

The current release-readiness checks are:

- shared Go and TypeScript conformance and behavior tests;
- static checks and package/module consumer smoke tests;
- Go race-detector coverage;
- local decoder fuzz coverage; and
- runnable composition examples.

## Why

This is the right stopping point for generic core expansion. It gives future
work a stable target and prevents a database adapter, broker integration, or
authenticated envelope from quietly redefining the core contract.

## Example

```text
Stable core -> choose one deployment vertical -> implement adapter -> verify
```

## Gotchas

- The checkpoint is not a claim that every production integration exists.
- PostgreSQL, broker-specific behavior, collector configuration, authenticated
  envelopes, key rotation, replay protection, and package publication remain
  separate decisions.
- A future lossless implementation may preserve unknown fields, but it must
  advertise and test that behavior explicitly.
- New core fields or transitions should wait for a concrete consumer need and a
  reviewed protocol extension.

## Used in

- [`spec/v1.md`](../../spec/v1.md)
- [`spec/README.md`](../../spec/README.md)
- [`README.md`](../../README.md)
- [`docs/notes/README.md`](README.md)
- [`CHANGELOG.md`](../../CHANGELOG.md)

## Related

- [The shared contract must precede both SDKs](001-spec-first-foundation.md)
- [The TypeScript package should publish one explicit, testable entry artifact](021-package-artifact-contract.md)
- [The Go module should pass a clean consumer smoke test](027-go-module-consumer-smoke-test.md)
