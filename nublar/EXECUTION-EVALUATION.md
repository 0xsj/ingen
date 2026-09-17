# Nublar execution evaluation

Status: evaluated and deferred, 2026-09-17

This evaluation checks whether Nublar should take ownership of launching,
scheduling, or retrying producer workflows. The current consumer evidence is
the live GitHub Actions handoff, where an external producer job creates the
shared envelopes and a separate Nublar job collects them.

## Observed ownership

| Concern | Current owner or evidence | Decision |
| --- | --- | --- |
| Producer commands | The external workflow invokes Sorna targets on `macos-latest`. | Keep execution outside Nublar. |
| Workspace and platform | GitHub Actions provisions the producer workspace and runner; Nublar runs in a separate consumer job. | No Nublar workspace manager is required. |
| Sequencing | The consumer job waits for the producer job and downloads its declared artifacts. | The existing artifact handoff is sufficient. |
| Process status | Producer job completion and uploaded envelopes are external inputs; Nublar composes envelope statuses. | Do not infer producer meaning from process exit codes. |
| Retry and rerun | GitHub Actions owns reruns; Nublar records a new opaque `run_id` with the external attempt tuple. | No Nublar scheduler or producer retry loop is required. |
| Evidence | Nublar validates, hashes, preserves, and projects complete producer envelopes. | Producer reports remain opaque and producer-owned. |

The boundary was exercised in
[live workflow run 35204322692](https://github.com/0xsj/ingen/actions/runs/35204322692),
which passed with separate producer and Nublar consumer jobs.

## Decision

Keep Nublar as a consumer and coordinator after producer publication. The
workflow declaration remains an expected-artifact collection policy, not a
process execution plan. Do not add an execution engine, scheduler, producer
retry queue, workspace isolation layer, or process-level provenance model.

This preserves the current ownership boundary documented in
[`EXECUTION-BOUNDARY.md`](EXECUTION-BOUNDARY.md): external systems own
command resolution, environment, sequencing, timeouts, retries, logs, and
process exit handling; Nublar owns strict envelope collection and the final
coordinator decision.

## Re-entry criteria

Reopen the execution phase only when a named consumer explicitly requires
Nublar to launch producers and supplies:

- ownership transfer and lifecycle states;
- workspace, platform, credential, and isolation rules;
- cancellation, timeout, retry, and cleanup semantics;
- scheduler capacity, fairness, and concurrency expectations;
- process-level provenance and acceptance proof for producer failures;
- evidence that external execution cannot satisfy the requirement.

Until then, adding producer commands to the workflow schema or inferring
results from process status would expand Nublar beyond the validated contract.

## Explicit non-goals

- Moving Sorna, Paddock, Sentinel, or another producer into Nublar.
- Treating a producer exit code as a substitute for its validated envelope.
- Adding scheduling or orchestration because it may be useful in a hosted
  future.
