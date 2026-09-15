# Sorna provider schema checkpoint

A provider schema can standardize the handoff structure without pretending to
standardize language-specific mutation semantics.

## Origin

This note came from freezing the first provider boundary before considering
additional SDKs or language implementations.

## What changed

Sorna now publishes the provider-manifest contract as
[`sorna/spec/ingen.mutation-provider-v1.schema.json`](../../sorna/spec/ingen.mutation-provider-v1.schema.json).
It describes the versioned provider envelope, exact capability tuples,
prepared executable entries, optional source provenance, and target-resolution
counts.

The schema is an external contract for CI tools and future providers. The Go
campaign package continues to perform runtime validation because it also needs
semantic checks such as plan coverage, capability matching, path safety, and
hash binding.

## Why it matters

The provider boundary is intentionally language-neutral. A TypeScript,
container, or future remote provider should be able to produce the same
reviewable manifest without importing Sorna's Go implementation. A versioned
schema gives those producers and consumers a shared structural vocabulary.

The schema does not make an operator safe or prove that a provider used a
particular source transformation. Those claims still require Sorna's plan
binding, preparation provenance, and evidence checks.

## Verification

The campaign package tests the schema's required envelope and capability
definitions. Existing provider validation tests continue to reject missing
capabilities and provider/plan drift.

The alpha vocabulary remains intentionally open-ended at the operator string:
providers declare exact tuples, while Sorna does not pretend that every future
language or mutation family can be enumerated in this first structural schema.

## Used in

The Sorna provider manifest, Go provider preparation, provider review, and
future language-neutral CI consumers.
