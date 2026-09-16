# A replay report must validate its own interpretation

## Finding

The replay report is not a verdict merely because it is valid JSON. Its top
level status, behavior status, per-case comparisons, observation counts, and
contract verdicts must agree with one another.

Without that boundary, a modified report could say `matched` while containing
an `outcome-drift` rule, or claim one changed observation while none of its
rules carry a changed observation hash.

## Implementation

`ValidateReplayResult` enforces the `sorna.replay/v1` shape, identity fields,
unique case/rule IDs, allowed comparison states, request and observation hash
semantics, request and observation counts, and top-level status consistency.
In particular, a report cannot claim `matched` when request intent changed or
when a request fingerprint was unavailable.

`LoadReplayReport` parses a saved report with unknown-field rejection and then
applies the same validation. The CLI surface is:

```sh
sorna evidence replay verify <replay-report.json>
```

The loader intentionally does not require canonical whitespace. Replay
reports are diagnostic JSON outputs; integrity of the consumed oracle and
evidence remains bound through their source artifacts and CI input hashes.

## Why this is separate from evidence verification

`evidence verify` checks the immutable evidence bundle's checksums and bundle
semantics. Replay-report verification checks the producer's interpretation of
a new execution. One does not replace the other.
