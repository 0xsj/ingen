# Release readiness should have one repeatable project gate

Individual checks are useful during development, but a release candidate needs
one explicit command that covers the project's important failure boundaries.

## Origin

Amber had separate commands for static checks, cross-language tests, package
and module consumers, race detection, fuzzing, and runnable examples. A release
reviewer still had to know which commands to combine.

## What

The root `Makefile` now provides `make release-check`. It runs the existing
`make check`, `make race`, `make fuzz`, and `make examples` targets in one
repeatable gate. No publishing, external collector, database, broker, or
network service is required.

## Why

The gate makes the current v1 stopping point concrete: the contract, SDKs,
adapters, malformed-input defenses, concurrency checks, package/module
boundaries, and runnable examples all pass together. Keeping the target
composed from existing commands avoids creating a second implementation of the
verification policy.

## Example

```sh
make release-check
```

This is the recommended final local command before packaging Amber or starting
a deployment-specific adapter.

## Gotchas

- Fuzzing is intentionally short and local; it is a smoke pass, not a claim of
  exhaustive fuzz coverage.
- The gate verifies the package artifact but does not publish it.
- Production-specific integrations remain separate decisions; this command
  does not require a database, message broker, collector, or network endpoint.

## Used in

- [`Makefile`](../../Makefile)
- [`README.md`](../../README.md)
- [`docs/notes/README.md`](README.md)

## Related

- [The v1 foundation should have an explicit freeze boundary](028-v1-foundation-checkpoint.md)
- [The public API should change only through an explicit compatibility check](032-public-api-compatibility.md)
- [CI should run the same cross-language checks and examples as local development](016-continuous-verification-workflow.md)
