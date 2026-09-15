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
| HTTP middleware | Request context installation, response-header propagation, and reject/ignore handling added for standard HTTP runtimes | HTTP middleware tests pass in both languages |
| Messaging middleware | Generic message wrappers clone metadata, install context, and propagate outgoing provenance | Messaging middleware tests pass in both languages |
| Durable storage seam | Go file-backed snapshots and a TypeScript atomic key-value backend contract preserve storage invariants | Durable storage tests pass in both languages |
| End-to-end examples | Runnable Go and TypeScript flows compose HTTP, child derivation, storage, logging, and tracing | `make examples` passes |
| Continuous verification | GitHub Actions runs the root check gate, Go race detector, and composition examples for main pushes and pull requests | Workflow mirrors `make check`, `make race`, and `make examples` |
| Static check gate | Root lint and check targets run Go vet and TypeScript typechecking alongside tests | `make check` passes |
| Work-history queries | Stores list executions by logical work ID in deterministic history order | Shared storage history fixture passes in both SDKs |
| Causation queries | Stores find immediate children and retries by causation kind and execution ID | Shared storage causation fixture passes in both SDKs |
| Correlation queries | Stores find related executions across logical works by correlation ID | Shared storage correlation fixture passes in both SDKs |
| Package artifact contract | TypeScript exports are explicit and a clean temporary consumer installs and exercises the publishable artifact before release | `make package-check` passes |
| Trust boundary | v1 remains unsigned by default and optional validators run before context installation | Trust-hook tests pass in both SDKs |
| Decoder fuzz coverage | Core JSON and transport metadata decoders have short local fuzz targets that preserve validation and round-trip invariants | `make fuzz` passes |
| Concurrent storage hardening | Memory and file-backed stores are exercised with concurrent writers/readers and a repeatable race-detector command | `make race` passes |
| CI race gate | Pull requests and main pushes run the Go race detector after the cross-language check gate | GitHub Actions workflow includes `make race` |
| Package install smoke test | The TypeScript tarball is installed in an isolated temporary project and its public ESM entry point performs a provenance round trip | `make package-check` passes |
| Go module consumer smoke test | A temporary external Go module imports the public Amber module path and propagates a provenance header | `make module-check` passes |
| v1 foundation checkpoint | Core semantics and release checks are stable; production-specific integrations and authenticated extensions remain separately scoped | Spec stability boundary and all local gates pass |
| OpenTelemetry adapter | Optional Go and TypeScript bridges apply the stable `amber.*` tracing projection to existing OTel-compatible spans without creating span lifecycle semantics | OTel adapter tests, shared fixture, and package smoke test pass |
| OpenTelemetry SDK integration proof | The Go and TypeScript bridges are verified against real OpenTelemetry SDK tracers and in-memory span recorders without requiring a collector | `make check` and `make race` pass with the SDK-backed integration tests |
| OpenTelemetry runnable examples | Both SDK bridges have offline, runnable examples showing application-owned tracer setup, Amber enrichment, and finished-span inspection | `make examples` runs the composition and OTel examples |
| Public API compatibility check | TypeScript runtime exports are pinned to a reviewed manifest and the external Go consumer exercises all adapter package boundaries | `make check` passes the manifest, package artifact, and module consumer checks |
| Release-candidate gate | One root command combines static checks, cross-language tests, package/module consumers, race detection, fuzzing, and runnable examples | `make release-check` passes |
| PostgreSQL storage adapter | Go provides an optional PostgreSQL implementation of the append-only Store contract with JSONB values and indexed work/causation/correlation queries | PostgreSQL adapter tests and the release gate pass without a live database |
| PostgreSQL live integration | An opt-in transaction-isolated test validates the adapter against a real PostgreSQL server through pgx without making the default gate network-dependent | `AMBER_POSTGRES_DSN=... make postgres-integration` |
| Generic Go storage backend seam | Go exposes a backend-neutral key-value seam so users can supply their own persistence while Amber retains serialization and storage invariants | Key-value store tests and the release gate pass |

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
12. [HTTP middleware should own the request and response provenance boundary](012-http-boundary-middleware.md)
13. [Messaging middleware should own context and metadata at the consumer boundary](013-messaging-boundary-middleware.md)
14. [Durable storage should preserve append-only semantics behind an explicit backend seam](014-durable-storage-backend-seam.md)
15. [A runnable composition example should cross the adapters without external services](015-end-to-end-composition-example.md)
16. [CI should run the same cross-language checks and examples as local development](016-continuous-verification-workflow.md)
17. [The root check gate should combine static analysis with cross-language tests](017-static-check-gate.md)
18. [Storage needs a deterministic work-history query over execution records](018-work-history-query.md)
19. [Storage should expose immediate-cause lookup separately from work history](019-causation-query.md)
20. [Correlation queries should link related work without changing work membership](020-correlation-query.md)
21. [The TypeScript package should publish one explicit, testable entry artifact](021-package-artifact-contract.md)
22. [Amber v1 should stay unsigned by default while exposing an explicit trust seam](022-unsigned-v1-trust-seam.md)
23. [Decoder fuzzing should protect the untrusted input boundary](023-decoder-fuzz-coverage.md)
24. [Storage concurrency should be verified with the race detector](024-storage-race-coverage.md)
25. [CI should enforce race safety continuously](025-ci-race-gate.md)
26. [The package artifact should pass a clean consumer smoke test](026-package-install-smoke-test.md)
27. [The Go module should pass a clean consumer smoke test](027-go-module-consumer-smoke-test.md)
28. [The v1 foundation should have an explicit freeze boundary](028-v1-foundation-checkpoint.md)
29. [OpenTelemetry should enrich existing spans without entering the core](029-opentelemetry-adapter.md)
30. [The OpenTelemetry bridge should be proven against a real SDK span](030-opentelemetry-sdk-integration.md)
31. [OpenTelemetry integration should have a runnable application path](031-opentelemetry-examples.md)
32. [The public API should change only through an explicit compatibility check](032-public-api-compatibility.md)
33. [Release readiness should have one repeatable project gate](033-release-candidate-gate.md)
34. [PostgreSQL should implement the existing storage contract without changing v1](034-postgres-storage-adapter.md)
35. [PostgreSQL compatibility should be testable against a real database without entering the default gate](035-postgres-live-integration.md)
36. [A generic storage backend should own persistence while Amber owns invariants](036-generic-storage-backend-seam.md)

## Current open questions

- Whether the public npm package should remain `@0xsj/amber` when release
  ownership and publishing are finalized.
- Which transport-specific context formats should follow HTTP after the core
  model stabilizes.
- Whether a future authenticated transport extension should define a signed
  envelope, key rotation, replay protection, and asynchronous key lookup.
