# CI should enforce race safety continuously

Concurrency checks are most valuable when every change runs them, not only
when a maintainer remembers a local hardening command.

## Origin

Amber's race-detector target and concurrent storage tests were available to
contributors, but the GitHub Actions workflow only ran the cross-language check
gate and composition examples.

## What

The CI workflow now runs `make check`, `make race`, and `make examples` for
pushes to `main` and pull requests. `make race` covers the Go SDK and all Go
adapters, including concurrent storage tests. Fuzzing remains a local,
opt-in command because its runtime is intentionally variable.

## Why

This makes race safety part of the repository's normal regression contract. A
future adapter or storage change cannot silently bypass concurrency checks just
because the ordinary tests happen to pass.

## Example

```yaml
- name: Run Go race-detector checks
  run: make race
```

## Gotchas

- The workflow does not run the time-variable fuzz target on every build.
- CI still does not provision external databases, brokers, tracing backends,
  or cross-process file-store coordination.
- Hosted-runner availability and timing remain outside Amber's code-level
  guarantees.

## Used in

- [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml)
- [`Makefile`](../../Makefile)
- [`README.md`](../../README.md)
- [`docs/notes/016-continuous-verification-workflow.md`](016-continuous-verification-workflow.md)

## Related

- [CI should run the same cross-language checks and examples as local development](016-continuous-verification-workflow.md)
- [Storage concurrency should be verified with the race detector](024-storage-race-coverage.md)
- [Decoder fuzzing should protect the untrusted input boundary](023-decoder-fuzz-coverage.md)
