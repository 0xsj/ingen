# The shared contract must precede both SDKs

A language-neutral provenance contract prevents the Go and TypeScript SDKs from
becoming two plausible but incompatible interpretations of Amber.

## Origin

Amber began as a project scaffold whose README explicitly placed the
language-neutral specification before the SDK implementations. The first slice
turned that ordering into `spec/v1.md` before adding executable code.

## What

The v1 specification defines the shared vocabulary—logical work, concrete
executions, correlation, causation, origin, depth, attempts, attribution,
retries, replays, and typed references—along with invariants, transitions, and
the canonical JSON field names. The SDKs implement that contract; they do not
define separate language-specific semantics.

## Why

Starting with one SDK would make its types and defaults the accidental source
of truth. A later TypeScript implementation could appear equivalent while
disagreeing on retry identity, replay attempts, optional attribution, or wire
field spelling. Keeping the contract first makes those disagreements visible
and gives conformance tests something stable to measure.

## Example

Both SDKs represent a retry as the same logical work with a fresh execution,
an execution causation link, `origin: "retry"`, and `mode.kind: "retry"`.

## Gotchas

- The specification describes provenance relationships; it does not grant
  authority, perform authorization, or prove that history is complete.
- A versioned wire contract is not the same thing as a transport adapter.
- Production random IDs make transition outputs unsuitable for exact-output
  fixtures; the SDKs therefore allow deterministic ID factories for conformance
  and controlled tooling.

## Used in

- [`spec/v1.md`](../../spec/v1.md)
- [`go/provenance.go`](../../go/provenance.go)
- [`typescript/src/provenance.ts`](../../typescript/src/provenance.ts)
- [`conformance/v1.json`](../../conformance/v1.json)

## Related

- [A provenance value is a history node, not a mutable request bag](002-immutable-transitions.md)
- [Conformance starts with shared invalid values](004-shared-conformance-fixtures.md)
