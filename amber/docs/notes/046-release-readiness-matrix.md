# Amber needs a release-readiness matrix before more generic expansion

The foundation is broad enough that the next decision should be deployment
specific, not another unrequested core abstraction.

## Origin

Amber now has stable core and storage specifications, two SDKs, propagation and
observability adapters, optional PostgreSQL support, reusable contract suites,
consumer checks, local release gates, and a hosted PostgreSQL integration job.
The remaining gaps are primarily environment and deployment decisions.

## What

[`docs/release-readiness.md`](../release-readiness.md) records three classes of
state:

- functionality verified locally by `make release-check`;
- PostgreSQL checks configured but dependent on a live DSN or hosted CI; and
- deployment-owned choices and explicit v1 non-goals.

It establishes `make release-check` as the local stopping point and recommends
choosing one deployment vertical before adding more core semantics or vendor
integrations.

## Why

This prevents “next step” work from turning into indefinite foundation growth.
It also makes the remaining PostgreSQL verification honest: the workflow is
configured, but a local run still requires an isolated database and credentials.

## Gotchas

- A passing local release gate does not prove database connectivity, migration
  compatibility, authorization, or application correctness.
- GitHub Actions configuration is not the same as an observed hosted-run result
  in this workspace.
- The matrix is a checkpoint, not a release approval; deployment owners still
  decide operational policies and package publication.

## Used in

- [`docs/release-readiness.md`](../release-readiness.md)
- [`README.md`](../../README.md)
- [`spec/v1.md`](../../spec/v1.md)
- [`spec/storage-v1.md`](../../spec/storage-v1.md)
- [`Makefile`](../../Makefile)
- [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml)

## Related

- [The v1 foundation should have an explicit freeze boundary](028-v1-foundation-checkpoint.md)
- [Release readiness should have one repeatable project gate](033-release-candidate-gate.md)
- [CI should run the live PostgreSQL contract without changing the offline gate](045-postgres-ci-integration-job.md)
