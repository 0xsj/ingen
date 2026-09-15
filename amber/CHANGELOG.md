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

Before publishing a release, move the completed entries into a versioned
section and record any compatibility or wire-format changes explicitly.
