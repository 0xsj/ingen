# Lockwood cleanup and enforcement boundaries

Status: operational contract with a read-only cleanup-plan projection
implemented. No deletion, retention enforcement, or distributed cleanup
coordinator is implemented.

The projection is exposed by `cleanup-plan` and its versioned output schema.
It classifies reconciliation findings but never reports deletion eligibility or
grants destructive authority.

This document separates the current read-only evidence reports from any future
operation that changes local storage or enforces a policy. Lockwood must not
turn an orphan report, legal-hold guard, descriptive actor, or trusted signing
key into deletion authority by implication.

## Current behavior

The current filesystem reconciliation path may report:

- valid blobs with no custody reference;
- dangling custody references;
- corrupt or unreadable blobs;
- age-qualified orphan `cleanup_candidates` when an explicit grace period and
  `--as-of` time are supplied;
- recognized detached artifacts protected by verified reference metadata.

This report is read-only. Modification time is a conservative age signal, not
a durable first-seen timestamp or permission to delete. `recover` may retry a
pending custody-record publication after re-verifying its existing blob; it
does not authorize cleanup of that blob.

`cleanup-plan` records the explicit `as_of` time and grace period, classifies
each valid orphan as `reported-orphan` or `cleanup-candidate`, and attaches the
fresh revalidation, policy, legal-hold, and authorization blockers that remain.
Every entry has `action_status: not-authorized`.

## Ownership boundary

| Concern | Owner | Lockwood boundary |
| --- | --- | --- |
| Retention duration and business policy | External governance/application | Consume a versioned policy or assertion; do not invent durations from labels. |
| Legal-hold authority | External governance/application | Treat visible active holds as a fail-closed local guard. |
| Delete authorization | External governance/application | Require an explicit action-bound authorization result. |
| Local blob and record mutation | Lockwood storage worker | Delete only after a fresh, exclusive eligibility check. |
| Distributed coordination | Deployment/operator layer | Provide a fencing-capable coordinator or keep destructive cleanup disabled. |
| Remote object deletion | Remote provider owner | Never infer remote deletion from local cleanup. |
| Audit and lifecycle workflow | Sentinel or owning workflow | Keep lifecycle receipts outside Lockwood unless a concrete contract is approved. |

## Cleanup states

Cleanup must distinguish these states rather than collapsing them into a
boolean:

```text
reported-orphan
    ↓ grace period and policy candidate
cleanup-candidate
    ↓ fresh exclusive revalidation
eligible-for-delete
    ↓ authorized destructive operation
deleted | deletion-failed | eligibility-lost
```

An orphan is only a blob with no currently visible local reference. A cleanup
candidate additionally satisfies the configured age signal. Neither state is
eligible for deletion until the worker rechecks all references, recovery state,
protected metadata, legal holds, policy snapshot, and authorization.

## Retention and legal holds

The current `retention_class` and handling events are descriptive custody
history. They do not calculate an expiry time or grant deletion permission.

A future enforcement policy must define:

- how a retention class maps to an expiry instant;
- which policy snapshot and evaluation time were used;
- whether the expiry clock starts at receipt, event time, or an external
  lifecycle timestamp;
- how legal holds override expiry and how a release becomes effective;
- what happens when the event stream is incomplete, damaged, concurrent, or
  unavailable.

The local rule is conservative: an active visible legal hold blocks destructive
actions; an unresolved or unreadable hold state is also a block. A
`not-blocked` guard result is not an authorization decision and must not be
used alone to delete. A future enforcement worker must evaluate the guard and
the external retention policy at one explicit evaluation time.

## Delete authorization and audit

Deletion of an artifact or custody record requires an external authorization
assertion bound to the exact local target and action. The existing authorization
handoff contract can carry a `custody-record` target, but a future deletion
policy must explicitly define whether the target is the record, the local blob,
or both.

The proposed first target is one local artifact blob, bound to its digest, the
canonical cleanup-plan digest, plan `as_of`, and the exact `cleanup-delete`
action. The policy, legal-hold, authorization-result, and receipt handoff is
specified in [`CLEANUP-AUTHORIZATION.md`](CLEANUP-AUTHORIZATION.md). This is a
design contract only; the current `cleanup-plan` remains `not-authorized` and
no cleanup-specific external authority or destructive worker is implemented.
The typed target/request/result validator and target schema only validate
caller-supplied evidence; they do not grant permission or mutate storage.

A read-only readiness projection now combines the canonical cleanup-plan digest,
the exact candidate entry, target-bound policy and hold references, and the
authorization result. Its positive state is `ready-for-revalidation`, never
`eligible-for-delete`; it always retains `action_status: not-authorized` and
the blocker requiring fresh exclusive local revalidation and coordination.

The typed preflight seam now accepts caller-supplied revalidation and
coordinator lease/fencing evidence bound to the same target and plan. Its
positive state is `ready-for-worker`, but it still reports
`action_status: not-authorized` and a blocker that no destructive worker is
implemented. It does not acquire or renew leases and does not read or mutate
the filesystem.

The canonical outcome receipt contract is also typed and validated for
requested, authorized, attempted, succeeded, failed, and `eligibility-lost`
states. Receipt persistence, tombstones, and post-delete verification remain
outside the current implementation.

The following are insufficient by themselves:

- a valid detached signing key;
- the descriptive handling `actor` field;
- an expired retention class;
- an orphan or cleanup-candidate report;
- a `not-blocked` legal-hold result;
- an operator's possession of the storage root.

A destructive workflow should produce an external authorization result and a
local transient or durable audit receipt containing at least the target digest,
action, policy snapshot digest, evaluation time, legal-hold decision, and
outcome. The receipt must distinguish requested, authorized, attempted,
succeeded, and failed states. A failed delete must not be reported as complete.

## Protected artifacts and recovery races

The following are protected from automatic orphan deletion:

- blobs referenced by accepted, quarantined, rejected, or pending custody
  records according to the selected policy;
- valid detached attestations, handling-event signatures, and provenance
  envelopes referenced by verified reference manifests;
- blobs currently being published, recovered, verified, or reconciled;
- artifacts covered by an active legal hold or unresolved hold state;
- artifacts claimed by an unexpired cleanup lease held by another worker.

Protection must be checked immediately before deletion, not only during the
initial reconciliation scan. If recovery publishes a record between the scan
and delete check, the cleanup operation must lose eligibility and leave the
blob intact.

## Race-safe cleanup

Local advisory locks protect only processes sharing the same filesystem root.
They do not coordinate separate hosts, containers, or storage workers. A
deployment that cannot provide distributed coordination must keep destructive
cleanup disabled and may produce reports only.

A future destructive worker should use this sequence:

1. Take a consistent read snapshot and record a scan ID and explicit `as_of`.
2. Produce candidates without mutating custody or blobs.
3. Acquire a local or distributed lease for the exact digest, with an expiry
   and fencing token when the coordinator supports them.
4. Revalidate references, pending recovery, detached protection, legal holds,
   policy, and authorization under exclusive local storage coordination.
5. Delete only if the lease/token is still valid and every check passes.
6. Verify the post-delete state and emit an outcome receipt; a lost lease or
   newly visible reference becomes `eligibility-lost`, never success.

Deletion must be idempotent: a missing blob after a previously recorded
successful delete is success on repeat, while a missing or damaged custody
record must not be silently treated as proof that its blob was unreferenced.

## Failure behavior

Cleanup fails closed on:

- missing or malformed policy/assertion;
- unknown, revoked, expired, or stale authorization evidence;
- active or unresolved legal holds;
- incomplete custody/reference inventory;
- inability to acquire or renew required coordination;
- provider or filesystem errors that prevent a fresh eligibility check;
- any digest, reference, or post-delete verification mismatch.

The worker may retry classified transient coordination or filesystem failures,
but it must not retry a policy, authorization, hold, or digest failure as if it
were transient. No remote provider object is deleted by this local policy.

## Explicit non-goals

This contract does not implement:

- a retention policy engine;
- deletion authorization or identity storage;
- remote lifecycle or object deletion;
- a distributed lock service;
- automatic cleanup scheduling;
- legal advice or a claim that local state is a complete compliance record.

## Open decisions

- Is the proposed local blob target sufficient, or is a custody-record tombstone
  also required as a separate, explicitly authorized workflow?
- Which external authority issues deletion and retention assertions?
- Must a successful deletion retain a tombstone and audit artifact locally?
- Which coordinator provides leases and fencing tokens across deployment
  replicas?
- What policy snapshot makes detached artifacts and pending recovery records
  protected across upgrades?

The proposed target, policy snapshot, authorization result, lifecycle, and
failure-closed receipt rules are recorded in
[`CLEANUP-AUTHORIZATION.md`](CLEANUP-AUTHORIZATION.md).
