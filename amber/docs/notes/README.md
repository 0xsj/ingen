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
| Incoming boundaries | Explicit reject/ignore policy, byte bound, and safe context installation added to both SDKs | Incoming-policy tests pass in both languages |
| HTTP adapter | `Amber-Provenance` base64url header, cloned outgoing requests, and shared header vector added | HTTP adapter tests pass in both languages |
| Messaging adapter | Transport-neutral envelope, cloned metadata propagation, and shared messaging vector added | Messaging adapter tests pass in both languages |
| Logging projection | Stable `amber.*` fields and cloned structured-log map merging added with optional domain data omitted by default | Logging projection tests pass in both languages |
| Tracing projection | Framework-neutral `amber.*` attributes and cloned span-attribute map merging added | Tracing projection tests pass in both languages |
| Storage contract | Append-only execution-keyed store, idempotent writes, conflict detection, and missing-record behavior added | Storage tests pass in both languages |
| Root verification | One project-level command runs both SDK suites and all shared fixtures | `make test` passes |

When a later change alters one of these results, update the relevant note and
this milestone table in the same change.

## Reading order

1. [The shared contract must precede both SDKs](001-spec-first-foundation.md)
2. [A provenance value is a history node, not a mutable request bag](002-immutable-transitions.md)
3. [Context propagation should be scoped by construction](003-scoped-context-and-restoration.md)
4. [Conformance starts with shared invalid values](004-shared-conformance-fixtures.md)
5. [Incoming handling must make the failure policy explicit](005-explicit-incoming-policy.md)
6. [The HTTP adapter should preserve the core JSON inside a transport-safe envelope](006-http-header-adapter.md)
7. [Message metadata should reuse the transport envelope without sharing mutable maps](007-messaging-metadata-adapter.md)
8. [The default logging projection should favor stable identity over optional domain data](008-structured-logging-projection.md)
9. [The tracing projection should not decide span lifecycle semantics](009-framework-neutral-tracing-projection.md)
10. [Storage must distinguish idempotent re-insertion from overwriting history](010-append-only-storage-contract.md)
11. [A project needs one verification command that crosses its language boundaries](011-root-verification-command.md)

## Current open questions

- Whether the public npm package should remain `@0xsj/amber` when release
  ownership and publishing are finalized.
- Which transport-specific context formats should follow HTTP after the core
  model stabilizes.
