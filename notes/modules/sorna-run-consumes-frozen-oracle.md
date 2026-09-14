# A subject run consumes the frozen oracle, not the contract source

The contract is an input to oracle generation. Once the oracle is frozen, the
subject runner should consume that artifact directly instead of reopening the
contract and regenerating its cases during evaluation.

## Origin

The first oracle slice produced a verified `oracle.json`, but the runner still
accepted the sealed contract as its primary input. That left a semantic bypass:
the workflow could freeze one artifact and then evaluate a newly interpreted
version of the contract.

## What changed

`runner.ExecuteOracle` now evaluates the canonical `ingen.oracle/v1` cases. It
records the oracle schema and canonical SHA-256 in the run record, and the run
evidence manifest carries the same reference. The CLI exposes this through
`sorna run --oracle <path>`.

The older `runner.Execute` contract API remains as a compatibility wrapper for
unit tests and legacy callers. The managed Make targets freeze first and then
invoke the runner with only `oracle.json` as its semantic test input.

## Why this matters

The frozen case ID, concrete generated input, subject operation, and
expectation are now the authority for the run. A runner cannot accidentally
re-expand a generator or observe a changed contract source after the freeze.
The contract hash remains visible through the oracle reference, while the
oracle hash identifies the exact artifact consumed.

## Findings

- Canonical JSON is part of the artifact identity; `oracle.LoadFile` rejects a
  valid-but-non-canonical representation.
- The run record can bind to an oracle without embedding the oracle itself. The
  separate oracle evidence bundle remains the artifact that stores and checks
  its bytes.
- Policy loading during a run is still evidence plumbing. It checks the
  supplied policy hash against the oracle when both are present, but it does
  not yet isolate the subject process.
- The old contract API is intentionally retained while integrations migrate;
  the new CLI workflow does not use it when `--oracle` is supplied.

## Used in

- [`sorna/internal/runner`](../../sorna/internal/runner/)
- [`sorna/internal/oracle`](../../sorna/internal/oracle/)
- [`sorna run`](../../sorna/cmd/sorna/)
- [`sorna/internal/evidence`](../../sorna/internal/evidence/)

## Related

- [`A frozen oracle is a separate artifact from a subject run`](sorna-oracle-freeze.md)
- [`A checksum-verified bundle proves artifact integrity, not isolation or correctness`](sorna-evidence-bundle.md)
- [`Fair mutation comparison requires the same sealed contract`](../concepts/fair-mutation-comparison-requires-the-same-contract.md)
