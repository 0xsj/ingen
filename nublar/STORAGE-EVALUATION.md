# Nublar storage evaluation

Status: evaluated and deferred, 2026-09-17

This evaluation checks whether the validated GitHub Actions consumer needs a
database, hosted API, or stronger multi-process filesystem coordination. It
uses the live handoff proof as the current scale and retention evidence rather
than assuming a future hosted workload.

## Observed consumer shape

| Concern | Current evidence | Decision |
| --- | --- | --- |
| Writers | One Nublar consumer job owns one local run store per workflow attempt. | Filesystem publication is sufficient. |
| Concurrency | No process shares a run store across runners; producer and consumer hand off through workflow artifacts. | No cross-process locking requirement is established. |
| Retention | The live workflow uploads the Nublar bundle with a 14-day artifact retention period. | No Nublar-owned garbage collection or remote retention is required. |
| Record size | The live Nublar run artifact was approximately 31 KiB; the decision was approximately 1.9 KiB and the delivery receipt 233 bytes. | No storage pressure is established. |
| Recovery | A failed or expired runner can be rerun by the external workflow; each attempt receives a new opaque Nublar `run_id`. | No durable recovery service is required by the current consumer. |
| Queries | The consumer needs same-job `show`, `list`, and correlation inspection only. | The existing deterministic local history is sufficient. |
| Authorization | The runner already controls access to its workspace and GitHub artifact upload. | No Nublar storage authorization contract is needed yet. |

The evidence comes from
[live workflow run 35204322692](https://github.com/0xsj/ingen/actions/runs/35204322692),
which completed the producer handoff, Nublar collection, GitHub Check delivery,
and artifact upload successfully.

## Decision

Keep the filesystem-backed run and receipt stores as the active Nublar
storage boundary. Do not add a database, hosted Nublar API, remote receipt
queue, retention worker, or cross-process locking in response to the current
GitHub Actions consumer.

This does not change the immutable run or receipt contracts. A workflow can
continue to upload the local records as artifacts, and a rerun can create a
new local run while retaining the external correlation tuple and attempt
number.

## Re-entry criteria

Reopen the storage phase only when a concrete consumer demonstrates one or more
of the following:

- multiple processes or machines must write to one logical run store;
- history must outlive the workflow artifact retention window;
- a measured run or receipt volume makes local filesystem history costly to
  query or retain;
- users need cross-runner search, pagination, or authenticated remote reads;
- crash recovery, locking, or migration behavior becomes an operational
  requirement.

Before implementation, record measured volume, retention, concurrency,
recovery, query, and authorization expectations in a new consumer request and
add an executable acceptance proof.

## Explicit non-goals

- Selecting a hosted database or API without a measured consumer need.
- Moving producer execution or artifact custody into Nublar storage.
- Replacing GitHub's workflow artifact retention or access controls.
