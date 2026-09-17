# Sorna provider handoff

Status: alpha contract freeze

This document freezes the Sorna-owned boundary between a mutation provider and
the campaign executor. The current boundary is versioned as
`ingen.mutation-provider/v1`. It is intentionally language-neutral: a provider
may be implemented in Go, TypeScript, Rust, or another language, but it must
produce the same manifest and satisfy the same review rules.

This is a freeze of the provider handoff, not a claim that mutation operators,
provider implementations, or the overall mutation specification are complete.

## Responsibilities

The provider owns language-specific preparation:

- read one canonical `ingen.mutation-plan/v1` plan;
- prepare a runnable subject variant for each planned mutation;
- declare exactly which plane/operator/target tuples it supports;
- preserve the plan identity it consumed when it can bind to exact bytes; and
- record source and binary provenance for source-level variants.

Sorna owns the language-neutral boundary and verification:

- validate and capture the provider manifest;
- compare plan mutations with provider entries and capabilities;
- require exact plan binding when the caller requests it;
- verify source and binary hashes before launch when provenance is present;
- launch isolated managed runs and classify their evidence; and
- verify the resulting evidence and campaign artifacts later.

Neither side treats this handoff as proof that the provider or oracle author
never saw implementation details. That claim requires a separate, externally
attested workflow. The handoff proves which declared inputs and prepared bytes
were supplied to Sorna.

## Frozen v1 invariants

1. The manifest has one closed top-level `mutation_provider` envelope with
   schema `ingen.mutation-provider/v1`, a non-empty ID, a positive version, and
   plan schema `ingen.mutation-plan/v1`.
2. Capabilities are exact triples of `plane`, `operator`, and `target`.
   Sorna does not infer support from an executable entry or from an operator
   name alone.
3. Every mutation in the plan must have one prepared entry and a matching
   declared capability before execution. Duplicate entries and capabilities
   are rejected. Extra entries may support another plan and are ignored for the
   current plan.
4. A prepared entry is an argv declaration: `command` is the executable and
   `args` are passed literally. Sorna performs no shell parsing. The only
   runtime substitutions permitted in arguments are `${SORA_ADDR}` and
   `${SORA_URL}`.
5. A provider may declare `plan_sha256` for exact plan-byte binding. A mismatch
   is always blocked. An absent exact binding remains usable only for local
   fixture workflows unless the caller supplies `--require-plan-binding`.
6. A provider may declare `plan_semantic_sha256`. It is compared with Sorna's
   semantic plan identity, which normalizes only the generated baseline run ID.
   A declared mismatch is blocked; an absent semantic binding is not treated as
   a match.
7. Source-level provenance is optional for legacy fixture providers. When it is
   present, `source_dir` must remain relative to the subject root and Sorna
   verifies the recorded source-tree and executable digests immediately before
   launch.
8. A provider review is a no-execution operation. A `ready` review means the
   handoff is structurally covered; it does not run a subject or decide whether
   the mutation is behaviorally killed.
9. Preparation summaries and campaign results remain separate artifacts. A
   valid preparation proves internal consistency of provider output; only the
   campaign evidence determines whether the contract detects the mutation.

The closed JSON Schema is the structural contract:
[`spec/ingen.mutation-provider-v1.schema.json`](spec/ingen.mutation-provider-v1.schema.json).
The Go loader additionally rejects unknown YAML fields, multiple documents,
and invalid provider relationships before a manifest is used.

The no-execution review report has its own closed schema at
[`spec/ingen.mutation-provider-review-v1.schema.json`](spec/ingen.mutation-provider-review-v1.schema.json).
That schema describes the emitted report shape; runtime review remains the
authority for cross-object plan and capability relationships.

## Conformance acceptance surface

The checked-in fixtures exercise the states a new provider implementation must
understand:

| Fixture | Expected review | What it proves |
| --- | --- | --- |
| `valid.yaml` | `ready` | Entries and exact capabilities cover the plan. |
| `exact-plan-drift.yaml` | `blocked` | A mismatched exact plan hash cannot run. |
| `semantic-plan-drift.yaml` | `blocked` | A declared semantic identity mismatch cannot run. |
| `unsupported-capability.yaml` | `blocked` | An entry without an exact capability is insufficient. |
| `partial-preparation.yaml` | `blocked` | Every planned mutation needs a prepared entry. |
| `malformed-entry.yaml` | load error | Structural validation happens before review. |

Run the acceptance corpus with:

```sh
make mutation-provider-conformance
```

The corpus must remain no-execution: no fixture review may start a subject
process. Changes to the v1 behavior must add or update a fixture, test, and
note explaining the compatibility decision.

## Compatibility rule

`v1` is now the alpha provider handoff for Sorna. Because the published schema
is closed, adding a manifest field or changing the meaning of an existing field
requires an explicit schema, runtime, fixture, and documentation review. A
breaking change to the handoff requires `ingen.mutation-provider/v2`; it must
not be hidden behind a provider version integer while retaining the v1 schema
identifier.

The next SDK should therefore implement this manifest and conformance surface
as-is. It should not add language-specific concepts to Sorna's campaign
executor merely to make its own preparation easier.

## First cross-language probe

The experimental TypeScript adapter under
[`providers/typescript/`](providers/typescript/) is the first consumer of this
freeze outside the Go provider path. It reads exact plan bytes, derives the
declared capabilities and prepared entries, and emits JSON for Sorna's existing
loader. It intentionally stops before source mutation, subject launch, or
campaign execution.

Run its smoke check with:

```sh
make mutation-typescript-provider-conformance
```

This is evidence that the handoff is portable, not evidence that TypeScript
mutation operators or a second behavioral provider are ready.

The smoke target also requests a mandatory exact-plan provider review and emits
the shared `ingen.ci-result/v1` envelope. This proves the cross-language
adapter reaches the same downstream CI boundary as the Go provider without
launching a subject.

The same target passes that envelope through the existing Nublar aggregate
workflow. Nublar receives the shared envelope as an opaque producer artifact;
it adds coordinator status without reinterpreting provider-review semantics.
The target also collects the envelope into a Nublar run store, reloads it, and
projects a provider-neutral decision while retaining the producer artifact in
the stored run.
