# Sorna alpha readiness

Status: ready for the current document-pipeline alpha slice; not a claim of
production-grade isolation, universal language support, or contract
completeness.

## What the readiness check covers

Run:

```sh
make sorna-alpha-check
```

The check is intentionally small enough to run as a release checkpoint while
still crossing the important Sorna boundaries:

| Check | What it establishes |
| --- | --- |
| `go test ./sorna/...` | Package-level behavior, validation, lifecycle, provider, evidence, replay, mutation, and host-specific sandbox tests pass. |
| `go vet ./sorna/...` | The Sorna packages pass Go's static analysis. |
| `jq empty sorna/spec/*.json` | Every published Sorna JSON schema is syntactically valid. |
| `sorna-replay-matrix-ci-result-fresh` | The six intentional replay regressions can be regenerated without existing artifacts, aggregated against the reviewable manifest, and independently verified. |

The fresh replay check is an integration proof for the current document lab,
not a replacement for the package tests or mutation campaign checks.

## Public alpha surface

Sorna currently exposes these command families:

- `contract validate|seal`
- `policy validate|seal`
- `sandbox exec`
- `oracle freeze|generate`
- `run`
- `evidence verify|replay|replay verify|replay matrix`
- `gate`
- `mutation validate|plan|contract|provider|run|verify|list`

The separate `sorna-go-provider` helper prepares source-level Go variants and
hands them to Sorna through the language-neutral provider manifest. Sorna owns
the contract, oracle, policy, public-boundary execution, evidence, and
verification semantics; the provider owns language-specific source mutation.

## Published artifact contracts

The current alpha freezes these Sorna-owned identities at the repository
boundary:

- `ingen.contract/v1`
- `ingen.policy/v1`
- `ingen.oracle/v1`
- `ingen.mutation-catalogue/v1`
- `ingen.mutation-plan/v1`
- `ingen.mutation-provider-review/v1`
- `ingen.mutation-preparation/v1`
- `ingen.mutation-campaign-result/v1`
- `sorna.replay-matrix/v1`
- `sorna.replay-matrix-explanation/v1`
- `sorna.replay-matrix-manifest/v1`

The shared `ingen.ci-result/v1` envelope is the consumer-facing handoff. The
contract, capability policy, frozen oracle, provider manifest, and
replay-matrix shapes have published JSON schemas under [`spec/`](spec/). The
remaining producer artifacts are currently defined by their specifications and
runtime validation rather than all having separate published JSON Schema files.

## Claims this checkpoint supports

- A contract can be materialized into a frozen oracle before subject execution.
- A separate subject can be evaluated through its public HTTP boundary.
- Evidence bundles and replay inputs can be hashed and independently checked.
- Deliberate implementation and contract changes can be classified without
  treating every red as the same kind of failure.
- A reviewable replay expectation manifest can be bound into a CI artifact and
  verified later.

## Claims it does not support

- A cryptographic or externally attested proof that an oracle author never saw
  implementation details. Current policy enforcement and telemetry are useful
  evidence, not that proof.
- Proof that the contract is correct or complete. Mutation killing measures
  sensitivity to the declared contract.
- Universal mutation operators or SDK support. The end-to-end source provider
  is currently Go and intentionally narrow.
- Complete process-tree or between-sample access observability. macOS
  executable telemetry remains periodic best effort and reports its gaps.

## Stopping point

This document is the stopping point for the current Sorna alpha slice. New
operators, providers, or producer artifacts should wait for an interface
review unless they close a concrete failure in the existing proof. The next
larger avenues are independently attested oracle generation, schemas for the
remaining producer artifacts, broader language providers, and CI packaging.
