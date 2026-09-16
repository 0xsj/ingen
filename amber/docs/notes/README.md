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
| Optional PostgreSQL package boundary | PostgreSQL is available through an explicit vendor-specific Go import path while the generic storage package remains the user extension point | External Go consumer imports the optional package successfully |
| Shared Go storage contract suite | Memory, file, and generic key-value stores run the same semantic checks, while PostgreSQL history queries are required to preserve deterministic ordering | Contract tests pass and PostgreSQL queries order by Amber history fields |
| Shared TypeScript storage contract suite | TypeScript memory and key-value stores run the same append-only and deterministic-history acceptance checks | `npm test` runs the compiled storage contract suite |
| Storage schema version boundary | File and PostgreSQL persistence formats expose versions separate from Amber's provenance wire version and PostgreSQL rejects unsupported metadata versions | Storage version tests pass and `EnsureSchema` validates metadata |
| Normative storage contract | The optional storage semantics are defined separately from the core wire contract and backed by shared fixtures and language-level suites | `spec/storage-v1.md` and both SDK contract suites remain aligned |
| Generic storage concurrency | Go and TypeScript exercise concurrent duplicate writes and history reads through memory and key-value stores | Race coverage and TypeScript storage contract tests pass |
| PostgreSQL public API boundary | The optional vendor package pins its documented Store contract, schema surface, readiness method, and error sentinel | External-package compatibility test passes |
| PostgreSQL schema readiness command | Operators can verify a migrated PostgreSQL schema without creating tables or writing provenance | `AMBER_POSTGRES_DSN=... make postgres-schema-check` |
| PostgreSQL CI integration | Pull requests run the live adapter contract against a pinned PostgreSQL service while the normal release gate remains offline | Dedicated GitHub Actions PostgreSQL job passes |
| Release-readiness matrix | The project distinguishes local evidence, live-environment checks, deployment-owned decisions, and v1 non-goals before further expansion | `docs/release-readiness.md` and `make release-check` |
| Go HTTP reference vertical | A real loopback `net/http` service composes inbound propagation, child derivation, injected storage, and response propagation | `make examples` runs the service flow |
| PostgreSQL CI readiness check | The PostgreSQL service job also runs the read-only schema check after migrations are applied, covering the operator-facing readiness path | Dedicated GitHub Actions PostgreSQL job runs `make postgres-schema-check` |
| Go HTTP reference vertical tests | The reference service boundary has focused accepted-input and malformed-input tests in addition to its runnable loopback example | `go test ./examples/service` passes |
| TypeScript HTTP reference vertical | The Fetch-compatible service handler covers inbound, absent, and malformed provenance with application-owned storage and explicit child response propagation | `npm test` runs the reference service tests |
| HTTP reference trust policy | Go and TypeScript reference handlers apply an application-owned validator before child derivation and storage | Reference service tests prove trusted acceptance and untrusted rejection |
| Messaging reference vertical | Go and TypeScript broker-neutral consumers apply trust validation, derive/store children, and preserve explicit child metadata without requiring a broker | Messaging reference tests and examples pass |
| Reference vertical conformance | A shared fixture verifies the observable HTTP and messaging child semantics across Go and TypeScript, including propagation, causation, correlation, and rejection | Go example tests and the TypeScript conformance runner pass |
| PostgreSQL migration asset | The optional package ships a checked-in v1 SQL migration and verifies it matches the adapter schema constant | PostgreSQL public package contract test passes |
| PostgreSQL migration preflight | An offline Make target checks the embedded migration/schema boundary while the adapter rejects missing readiness metadata | `make postgres-migration-check` passes |
| Release metadata | The repository and TypeScript artifact carry explicit MIT licensing, repository metadata, and package smoke assertions | Package smoke test verifies README, LICENSE, and metadata |
| Local PostgreSQL verification | A disposable PostgreSQL 16 instance validates the live adapter contract and the read-only readiness path after the checked-in migration is applied | Live integration and post-migration schema checks pass locally |
| Release handoff documentation | The root README distinguishes implemented capabilities, local release evidence, and deployment-owned or credentialed actions | `make release-check` passes and release actions are listed explicitly |
| Migration-first PostgreSQL CI | CI applies the checked-in migration before readiness and live integration, so the migration asset is exercised rather than silently replaced by bootstrap DDL | PostgreSQL workflow orders migration, readiness, then integration |
| Contributor and release runbooks | Project prerequisites, verification commands, PostgreSQL sequencing, artifact checks, and release stop conditions are documented for handoff | `CONTRIBUTING.md` and `RELEASE.md` |
| Getting started guide | Go and TypeScript users have a short adoption path covering root/child values, context, trust, storage choices, and verification | `docs/getting-started.md`, `go/README.md`, and `typescript/README.md` |
| Runnable getting-started examples | The minimal Go and TypeScript adoption snippets compile and run as part of the examples gate | `make example-go-getting-started`, `make example-typescript-getting-started`, and `make examples` |
| Versioning decision point | Current package/module facts and the approved `0.1.0` synchronized release identity are recorded separately from the not-yet-created monorepo tag and package publication | `VERSIONING.md` and `RELEASE.md` |
| Release version consistency | A repository version file and TypeScript test keep the package and lockfile aligned at the approved `0.1.0` release identity | `npm test` reports release version metadata passed |
| User-defined adapter authoring | Applications have one guide for choosing generic seams, preserving storage invariants, and testing custom adapters without expanding v1 | `docs/adapter-authoring.md` and existing storage contract suites |
| Custom backend examples | Go and TypeScript provide copyable application-owned key-value backends that exercise atomic insertion, conflict protection, and deterministic history | `make examples` and custom-backend tests pass |
| Background worker vertical | Go and TypeScript show a broker-free worker deriving incoming child work, recording a retry, and reading deterministic logical-work history | `make examples` and worker tests pass |
| Reusable adapter contract tests | Custom Go and TypeScript storage implementations can run the portable semantic contract without copying the full suite | Go contracttest package and `@0xsj/amber/testing` helper pass |
| External contract-helper boundary | A temporary external Go module imports and runs the public storage contract helper, matching TypeScript installed-subpath coverage | `make module-check` passes the external test and consumer |
| Development backlog boundary | Deferred deployment actions and future consumer-driven implementation work are recorded separately from the verified local checkpoint | `BACKLOG.md` and release-readiness documents |

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
37. [Vendor-specific storage should have an explicit optional import path](037-optional-postgres-package-boundary.md)
38. [Every Go storage adapter should prove the same semantic contract](038-shared-go-storage-contract-suite.md)
39. [TypeScript storage backends should prove the same semantic contract](039-shared-typescript-storage-contract-suite.md)
40. [Storage schema versions should be separate from the Amber wire version](040-storage-schema-version-boundary.md)
41. [The storage contract should be normative but separate from the core wire contract](041-normative-storage-contract.md)
42. [The generic storage seam should be exercised under concurrent callers](042-generic-storage-concurrency.md)
43. [The optional PostgreSQL import path should have an explicit compatibility check](043-postgres-public-api-boundary.md)
44. [The PostgreSQL schema readiness check should be runnable without writes](044-postgres-schema-check-example.md)
45. [CI should run the live PostgreSQL contract without changing the offline gate](045-postgres-ci-integration-job.md)
46. [Amber needs a release-readiness matrix before more generic expansion](046-release-readiness-matrix.md)
47. [The first deployment vertical should prove a real Go HTTP service path](047-go-http-reference-vertical.md)
48. [CI should verify PostgreSQL read-only schema readiness](048-postgres-ci-schema-readiness-check.md)
49. [The Go HTTP reference vertical should have focused boundary tests](049-go-http-reference-vertical-tests.md)
50. [The TypeScript HTTP reference vertical should match the Go boundary](050-typescript-http-reference-vertical.md)
51. [The HTTP reference vertical should demonstrate application-owned trust policy](051-http-reference-trust-policy.md)
52. [The messaging reference vertical should preserve consumer-owned child metadata](052-messaging-reference-vertical.md)
53. [The reference verticals should share one observable conformance fixture](053-reference-vertical-conformance.md)
54. [PostgreSQL should ship a checked-in migration asset with readiness guidance](054-postgres-migration-asset.md)
55. [PostgreSQL migration readiness should have an offline preflight](055-postgres-migration-preflight.md)
56. [Published artifacts should carry explicit release metadata](056-release-metadata.md)
57. [Local PostgreSQL verification should follow the migration-before-readiness sequence](057-local-postgres-verification.md)
58. [Release handoff should separate local proof from deployment-owned actions](058-release-handoff.md)
59. [PostgreSQL CI should exercise the checked-in migration before adapter writes](059-postgres-ci-migration-first.md)
60. [Contributor and release runbooks should make the verified workflow repeatable](060-contributor-release-runbooks.md)
61. [Getting started guidance should show the smallest useful cross-language path](061-getting-started-guide.md)
62. [Getting-started snippets should be executable so the first-use path cannot drift](062-getting-started-examples.md)
63. [Release identity should be decided separately from wire and storage versions](063-versioning-decision.md)
64. [Release metadata should have one checked version source](064-release-version-consistency.md)
65. [User-defined adapters should reuse Amber's invariant-owning seams](065-user-defined-adapter-authoring.md)
66. [Custom backend examples should make the generic seam copyable](066-custom-backend-examples.md)
67. [A background worker should make child and retry semantics concrete](067-background-worker-reference-vertical.md)
68. [Custom storage adapters should have reusable contract-test helpers](068-reusable-storage-contract-helpers.md)
69. [Public contract-test helpers should pass an external consumer check](069-external-contract-helper-consumer.md)
70. [Deferred deployment should be separated from consumer-driven development](070-development-backlog-boundary.md)

## Current open questions

- Whether the public npm package should remain `@0xsj/amber` when release
  ownership and publishing are finalized.
- Which transport-specific context formats should follow HTTP after the core
  model stabilizes.
- Whether a future authenticated transport extension should define a signed
  envelope, key rotation, replay protection, and asynchronous key lookup.
