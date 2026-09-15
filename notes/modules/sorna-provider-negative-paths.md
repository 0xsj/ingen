# Sorna provider negative paths

A source mutator must select exactly one reviewed target before it can write a
mutation.

## Origin

This note came from hardening the Go provider after the document-pipeline
operator vocabulary expanded to state, persistence, and input-validation
mutations.

## What changed

The Go document provider's unit tests now cover target-resolution failures for
the newer mutation families:

- two matching process transitions are rejected as ambiguous;
- a missing persistence assignment is rejected as an absent target;
- an unsupported validation suffix is rejected as an invalid change;
- a missing validation suffix is rejected before source preparation.

The ambiguity and missing-target cases also verify that the subject source is
unchanged. A provider must never silently choose the first AST match or leave a
partial mutation behind when it cannot select exactly one target.

## Why it matters

Mutation preparation is an experiment boundary. If the provider applies a
change to the wrong statement, or if a malformed specification is accepted,
the campaign result no longer describes the reviewed mutation. Structured
target-resolution errors make that failure explicit and preserve the
candidate/applied counts for CI consumers.

## Findings

Target selection and mutation application are separate obligations:

1. the change must be valid for the provider's declared vocabulary;
2. the source must contain exactly one matching target;
3. only then may the provider write the mutated source.

This protects the newer state, persistence, and input-validation operators.
It does not yet cover malformed provider manifests or every possible AST-shape
drift; those remain broader hardening work after the alpha vocabulary is
frozen.

## Verification

The focused provider tests and `make alpha-interface-check` pass. The latter
also covers the core, Sorna, document-pipeline, and current Nublar integration
surfaces.

## Used in

The Go document provider's AST target resolution and preparation handoff to
Sorna's campaign executor.
