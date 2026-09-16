# Replay must preserve the frozen boundary

## Finding

Stored Sorna evidence needs two different follow-up operations:

1. verify the evidence bundle's checksums and structural semantics; and
2. re-execute the frozen oracle against an explicitly supplied equivalent
   subject.

Those operations must not be conflated. Verification is read-only and does
not contact a subject. Replay is a new behavioral experiment; it contacts the
URL supplied by the caller, but it does not launch a process, reopen contract
source, or rewrite the original evidence directory.

## Implementation

`evidence.Replay` verifies the bundle, loads its recorded run identity, hashes
the caller-supplied canonical oracle, and requires the oracle and contract
references to match the recorded run. It then delegates execution to
`runner.ExecuteOracle`, which accepts the frozen artifact directly.

The command is:

```sh
sorna evidence replay --oracle <frozen-oracle.json> --base-url <url> <evidence-directory>
```

The report uses `sorna.replay/v1`. It compares rule IDs, statuses, setup
outcome shapes, assertion outcome shapes, and the contract verdict. A
contract-visible difference is `drifted`. Observation hashes are compared for
diagnosis but are tracked separately: an observation can change while the
contract-visible outcome remains `matched`.

If the supplied subject cannot be evaluated, replay reports `error` (or
`inconclusive` when evaluation is incomplete) rather than calling that a
behavioral drift. An unavailable observation is not compared as a changed
observation.

## Why the URL is explicit

Replaying against the original recorded URL by default could send requests to
an unintended live system. Requiring `--base-url` makes the new experiment's
target an explicit reviewable input. Replay does not claim that the target is
equivalent; it reports what happened when the frozen oracle was run there.

## Limits

The first replay slice is an HTTP/JSON execution path. Requests may have side
effects on the supplied subject, so “read-only” applies to the stored
evidence bundle, not to the external subject. A future adapter can provide a
resettable fixture or an isolated replay environment when the protocol needs
stronger repeatability.
