# Executable identity is a timeline, not a startup fact

A one-time executable check protects the launch boundary, but it cannot answer
whether a process later executed a different binary. Process identity needs to
be represented as a history with visible sampling limits.

## Origin

Sorna already resolved and hashed the requested executable before launch, then
compared it with a live host observation. The adversarial replacement probe
showed why that check must remain separate from post-launch observation: a file
can change after the prepared identity was recorded.

## What changed

The Darwin access collector now samples the observed process tree while the
process is alive. For each PID it hashes the executable path reported by the
host and appends an observation only when the path or digest changes. A
synchronous sample happens during `Attach` so a short-lived process has an
initial chance to appear before the sampler starts.

Oracle evidence writes `events/executables.jsonl`; managed-subject evidence
writes `events/subject-executables.jsonl`. The manifest records the observation
count, attempted sample count, sampling interval, sampling window, and any
hashing or sampling errors. The evidence verifier checks that this metadata is
internally consistent and that the JSONL record count matches the manifest.
The top-level assurance reports `observation_coverage` as
`periodic-best-effort` or `periodic-best-effort-with-gaps`, making the blind
spot machine-readable. Oracle assurance becomes
`host-enforced-observed-with-gaps` when the history has gaps or no successful
identity observation.

## Findings

- The clean oracle and defect subject each record a stable root executable
  observation, including the path, SHA-256 digest, PID, and timestamp.
- A shell-to-`exec` test records two identities for the same PID, proving the
  stream can represent an observed transition rather than only a startup fact.
- A short-lived-transition probe uses a wider test interval and exits before
  the next tick; the root's later identity is absent, making the sampling blind
  spot executable rather than merely theoretical.
- Process exit can race with the final process-table sample. The identity may
  already be gone or unreadable by the time it is hashed; this is retained as a
  gap instead of silently becoming a green observation.
- Repeated samples are coalesced, keeping the evidence readable without losing
  transitions.
- A sample count is an attempted process-table sample, not a count of
  successfully hashed processes. Process-tree and executable-hash errors remain
  separate counters, so the verifier can see partial collection.
- The sampling interval and start/stop window make the observation claim
  bounded and inspectable. They do not turn periodic sampling into continuous
  monitoring.

## Limits

This is still parent-side observation, not independent OS attestation. Sampling
can miss a very short-lived process or an `exec` between samples, process IDs
can be reused, and a successful observation does not prove that no other
process existed outside the observed tree. A future strengthening step can
replace periodic parent-side sampling with an independently attested process
transition source, if the target platform provides one.

## Used in

- [`sorna/internal/sandbox`](../../sorna/internal/sandbox/)
- [`sorna/internal/evidence`](../../sorna/internal/evidence/)
- [`sorna run`](../../sorna/cmd/sorna/)

## Related

- [`Adversarial boundary probes test negative claims, not just green paths`](sorna-adversarial-boundary-probes.md)
- [`A live process check adds lifecycle evidence without upgrading isolation assurance`](sorna-live-process-check.md)
- [`Host access telemetry is evidence, not an absence proof`](sorna-access-event-evidence.md)
