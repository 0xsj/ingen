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

Before publishing a release, move the completed entries into a versioned
section and record any compatibility or wire-format changes explicitly.
