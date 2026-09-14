# Conformance starts with shared invalid values

Testing only each SDK's happy path can hide wire drift, so the first shared
fixtures must include malformed values that every implementation is required to
reject.

## Origin

While checking the first Go JSON round trip, the default encoder emitted
CamelCase names for nested Go structs even though Amber's wire contract uses
snake_case. The implementation passed its own round trip until the canonical
field names were asserted explicitly.

## What

`conformance/v1.json` contains fixed valid and invalid v1 wire values. Go tests
and the TypeScript test command both decode every valid value and reject every
invalid value. The fixture uses fixed UUIDs only for semantic comparison; new
transitions still generate fresh UUIDv4 values.

## Why

An implementation can serialize and deserialize its own incorrect shape
perfectly and still fail when communicating with the other SDK. Shared invalid
cases are particularly valuable because they test the contract's boundaries:
missing required IDs, origin/mode disagreement, self-referential replay
sources, and unsupported causation kinds.

## Example

The canonical wire form is `initiated_by`, not Go's default `InitiatedBy`; the
Go model therefore uses explicit JSON tags and the conformance test checks the
emitted nested keys.

## Gotchas

- A fixture proves decoding and validation behavior, not that a random ID
  generator is collision-free.
- A passing round trip proves internal consistency, not cross-language
  compatibility; both implementations must consume the same fixture.
- The fixtures do not yet exercise transport adapters; the current transition
  cases cover `Child`, `Retry`, and `Replay` with deterministic injected IDs.

## Used in

- [`conformance/v1.json`](../../conformance/v1.json)
- [`conformance/README.md`](../../conformance/README.md)
- [`go/conformance_test.go`](../../go/conformance_test.go)
- [`typescript/test/conformance.test.mjs`](../../typescript/test/conformance.test.mjs)
- [`go/provenance_test.go`](../../go/provenance_test.go)

## Related

- [The shared contract must precede both SDKs](001-spec-first-foundation.md)
- [Go's JSON map ordering and canonical bytes](../../../notes/language/go-json-maps-and-canonical-bytes.md)
