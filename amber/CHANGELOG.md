# Changelog

All notable changes to Amber are documented here.

## Unreleased

- Added immutable provenance transitions, scoped contexts, and shared v1
  conformance fixtures for Go and TypeScript.
- Added HTTP and messaging propagation, logging and tracing projections, and
  append-only storage adapters.
- Added HTTP and messaging middleware, durable storage seams, work/causation/
  correlation queries, end-to-end examples, and continuous verification.
- Added package metadata and artifact checks for the TypeScript distribution.
- Added short local Go fuzz targets for core JSON and transport metadata
  decoding, with validation and round-trip invariants.
- Added concurrent storage coverage and a repeatable Go race-detector target.
- Added the Go race-detector target to the continuous integration workflow.
- Added a clean-consumer smoke test for the TypeScript package tarball.
- Added an external-consumer smoke test for the Go module boundary.
- Defined the v1 foundation checkpoint and clarified the non-lossless unknown
  JSON-field compatibility policy.
- Added optional Go and TypeScript OpenTelemetry span-enrichment adapters.
- Added Go SDK-backed OpenTelemetry integration coverage using an in-memory
  span recorder.
- Added TypeScript SDK-backed OpenTelemetry integration coverage using an
  in-memory span exporter.
- Added runnable offline Go and TypeScript OpenTelemetry integration examples.
- Added public API compatibility coverage for the TypeScript package and all
  Go adapter package boundaries.
- Added a single `make release-check` gate combining checks, race detection,
  fuzzing, and runnable examples.
- Added an optional Go PostgreSQL storage adapter with JSONB persistence,
  indexed history queries, and append-only conflict semantics.
- Added an opt-in live PostgreSQL integration check using pgx and a rolled-back
  transaction; the default release gate remains offline.
- Added a generic Go key-value storage seam so applications can provide their
  own backend while Amber retains serialization and append-only invariants.
- Added an explicit optional Go PostgreSQL package import path for the
  vendor-specific storage integration.
- Added a shared Go storage contract suite and aligned PostgreSQL history
  queries with deterministic Amber ordering.
- Added a TypeScript storage contract suite for memory and key-value stores.
- Added explicit file and PostgreSQL storage schema versions separate from the
  Amber provenance wire version, including PostgreSQL mismatch detection.
- Added a read-only PostgreSQL `CheckSchema` readiness check and a shared
  unsupported-schema-version error sentinel.
- Added a separate normative `storage-v1` contract for immutable writes,
  deterministic history queries, backend seams, and migration boundaries.
- Added concurrent duplicate-write and history-read coverage for the generic
  Go and TypeScript storage seams.
- Added an external-package compatibility check for the optional PostgreSQL
  API boundary and schema readiness surface.
- Added an opt-in read-only PostgreSQL schema-check example and Make target.
- Added a separate GitHub Actions PostgreSQL service job for live adapter
  integration without changing the offline release gate.
- Extended the PostgreSQL service job to verify the read-only schema readiness
  command after the live integration check.
- Added a release-readiness matrix separating local evidence, live checks,
  deployment decisions, and explicit v1 non-goals.
- Added a runnable Go `net/http` reference service that composes inbound
  propagation, storage injection, and response propagation.
- Added focused accepted-input and malformed-input tests for the Go HTTP
  reference service boundary.
- Added a TypeScript Fetch-compatible reference service handler and aligned
  explicit child response headers with the Go HTTP boundary behavior.
- Added application-owned trust-validator coverage to both HTTP reference
  handlers, proving trusted acceptance and untrusted rejection before storage.

Before publishing a release, move the completed entries into a versioned
section and record any compatibility or wire-format changes explicitly.
