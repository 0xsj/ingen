# Amber implementation notes

These notes capture the reasoning, surprises, and verification behind Amber's
implementation. The specification remains the source of required behavior;
these notes explain why the implementation has its current shape.

Notes use the InGen format: a claim line followed by `Origin`, `What`, `Why`,
`Example`, `Gotchas`, `Used in`, and `Related`. They are written as the project
evolves, not as a replacement for the code or specification.

## Implementation milestones

| Milestone | Result | Verification |
| --- | --- | --- |
| Project initialization | Go module and TypeScript package metadata created in their SDK directories | `go test ./...`, `npm run typecheck` |
| Shared model | v1 specification defines identities, transitions, attribution, references, and the JSON contract | Go and TypeScript implementations consume the same field names |
| Core SDKs | `Start`, `Child`, `Retry`, `Replay`, validation, and JSON round trips implemented in both languages | Go tests and TypeScript tests pass |
| Context safety | Explicit immutable/scoped context propagation and restoration added to both SDKs | Context tests pass in both languages |
| Conformance | Shared valid/invalid wire fixtures and deterministic transition traces added and consumed by both SDKs | Conformance tests pass in both languages |

When a later change alters one of these results, update the relevant note and
this milestone table in the same change.

## Reading order

1. [The shared contract must precede both SDKs](001-spec-first-foundation.md)
2. [A provenance value is a history node, not a mutable request bag](002-immutable-transitions.md)
3. [Context propagation should be scoped by construction](003-scoped-context-and-restoration.md)
4. [Conformance starts with shared invalid values](004-shared-conformance-fixtures.md)

## Current open questions

- Whether the public npm package should remain `@0xsj/amber` when release
  ownership and publishing are finalized.
- Whether incoming adapters should expose a strict reject policy, an absent
  context policy, or both.
- Which transport-specific context formats belong in adapters after the core
  model stabilizes.
