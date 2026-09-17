# Lockwood cleanup authorization and policy handoff

Status: draft contract; typed read-only cleanup target/request/result
validation is implemented. This slice defines the boundary between the
read-only `cleanup-plan` report and any future destructive worker. It does not
authorize, schedule, coordinate, or perform deletion.

The purpose of this handoff is to prevent a cleanup candidate from becoming a
deletion permission through an implicit interpretation of local state. A
future worker must receive an exact target, a versioned policy evaluation, an
explicit legal-hold decision, and an action-bound authorization result before
it can become eligible to delete anything.

## Ownership

| Concern | Owner | Required handoff |
| --- | --- | --- |
| Candidate discovery and local revalidation | Lockwood | Versioned cleanup plan and fresh eligibility result |
| Retention policy | External governance/application | Immutable policy snapshot reference and digest |
| Legal-hold decision | External governance/application | Explicit decision at the policy evaluation time |
| Delete authorization | External authority | Action-bound authorization handoff/result |
| Lease, fencing, and scheduling | Deployment/coordinator layer | Exact-target lease and fencing evidence |
| Local deletion and postcondition check | Lockwood worker | Outcome receipt; no remote deletion implied |

No owner may infer another owner's decision from a descriptive field. In
particular, `cleanup-candidate`, an expired retention label, a visible
`not-blocked` hold guard, or possession of a storage root is not authorization.

## Exact target

The proposed cleanup target for the first enforcement contract is one local
artifact blob, identified by its content digest. The target must bind all
evidence that made it a candidate:

```text
target_kind: artifact-blob
artifact_digest: sha256:<hex>
cleanup_plan_digest: sha256:<canonical-plan-bytes>
plan_as_of: <RFC3339 instant>
action: cleanup-delete
```

The target is not a wildcard, directory, prefix, custody-record query, or
remote URI. A custody-record tombstone or a remote-object deletion requires a
separate target kind and separate authorization; it must not be smuggled into
`artifact-blob` cleanup.

The plan digest is the digest of the canonical `cleanup-plan/v1` document, not
the digest of an individual log line or a re-serialized subset. The referenced
entry must be present, must be `cleanup-candidate`, and must still identify the
same artifact digest, size, and `plan_as_of`. A `reported-orphan` entry cannot
be submitted for authorization.

The existing generic authorization handoff can provide the canonical binding
pattern, but it does not yet implement this cleanup target or action. Adding
`artifact-blob` and `cleanup-delete` is a future, versioned extension and
must preserve the existing action/target/principal/evidence validation rules.

## Policy snapshot

Retention policy remains external. The caller must supply an immutable,
versioned policy snapshot with at least:

- policy identifier and version;
- canonical snapshot digest;
- retention rule used for the target;
- expiry instant and the source timestamp from which it was calculated;
- evaluation time;
- legal-hold decision and the hold-state snapshot digest;
- policy status, including whether the result is complete or indeterminate.

The evaluation is valid only when the policy snapshot and hold snapshot are
available, canonical, and evaluated at the same explicit time. Missing,
malformed, stale, revoked, or indeterminate policy/hold evidence is a
fail-closed blocker. Lockwood may validate the shape and digest of the
handoff, but it does not decide retention duration or interpret legal advice.

## Authorization request and result

A future caller-owned adapter should receive a request containing:

1. the exact cleanup target and `cleanup-delete` action;
2. the canonical cleanup-plan digest and candidate entry;
3. the policy snapshot and legal-hold snapshot digests;
4. the single evaluation time used for eligibility;
5. the current local revalidation result;
6. lease/fencing evidence, when the deployment requires coordination;
7. an opaque principal or workflow reference supplied by the external authority.

The adapter must return an explicitly bound result:

```text
decision: authorized | denied | indeterminate
target_digest: <exact target digest>
action: cleanup-delete
policy_snapshot_digest: <digest>
evaluated_at: <same evaluation time>
hold_decision: not-held | held | indeterminate
failure_code: <required for denied or indeterminate>
```

`authorized` is valid only when every binding matches the request and the
hold decision is `not-held`. `denied` and `indeterminate` are distinct: the
former is an explicit policy/authority refusal, while the latter means the
worker cannot establish a safe decision. Neither permits a retry that skips
the failed check.

The authorization result is an input to eligibility, not proof that the local
state is unchanged. Immediately before mutation, the worker must re-check the
artifact digest and size, all custody/reference manifests, pending recovery,
detached protection, hold state, policy freshness, and lease/fencing token.

## Lifecycle and receipt

The lifecycle is explicit and monotonic:

```text
requested
  -> authorized
  -> attempted
  -> succeeded | failed | eligibility-lost
```

An authorization result may end the flow at `denied` or `indeterminate`; it
must never be recorded as `attempted` or `succeeded`. A newly visible reference,
active/unresolved hold, policy drift, target mismatch, or lost lease produces
`eligibility-lost` and leaves the artifact intact.

The future receipt must bind at least:

- receipt schema and receipt identifier;
- exact target digest and action;
- cleanup-plan, policy, and hold snapshot digests;
- evaluation time and attempt time;
- authorization decision and external authority reference;
- lease/fencing reference, when used;
- outcome: `requested`, `authorized`, `attempted`, `succeeded`, `failed`, or
  `eligibility-lost`;
- postcondition verification result and failure code, when applicable.

Receipts must be append-only or externally durably owned. A failed filesystem
operation is not success, and a missing blob is success only when a prior
receipt proves that this exact target was successfully deleted. A receipt for
one target cannot be replayed for another target, plan, action, or policy
evaluation.

## Failure-closed matrix

| Condition | Result | Destructive action |
| --- | --- | --- |
| Candidate is missing, changed, or not `cleanup-candidate` | target mismatch / eligibility-lost | blocked |
| Policy or hold snapshot is missing, malformed, stale, or indeterminate | policy/hold failure | blocked |
| Hold is active | denied | blocked |
| Authorization is denied, expired, revoked, or mismatched | denied | blocked |
| Authorization adapter cannot decide | indeterminate | blocked |
| Reference inventory or recovery state is incomplete | eligibility-lost | blocked |
| Lease/fencing cannot be acquired or is lost | eligibility-lost | blocked |
| Delete or postcondition verification fails | failed | never report success |

Only explicitly classified transient coordination or filesystem failures may
be retried, and each retry must repeat the full target and eligibility
binding. Policy, hold, authorization, and digest failures are not converted
into transient retries.

## Current implementation boundary

Implemented today:

- read-only `cleanup-plan` projection with explicit `as_of` and grace inputs;
- `not-authorized` action status on every projected entry;
- typed `artifact-blob` target and `cleanup-delete` action binding to the
  canonical cleanup-plan digest and `plan_as_of`;
- typed policy and legal-hold snapshot references bound to the exact target and
  shared evaluation time;
- typed cleanup authorization request/result validation, including required
  policy evidence and fail-closed legal-hold decisions;
- strict canonical JSON request/result transport with digests;
- read-only readiness evaluation that stops at `ready-for-revalidation` and
  keeps `action_status: not-authorized`;
- typed target-bound fresh-revalidation and lease/fencing evidence validators;
- a worker-preflight projection that stops at `ready-for-worker` while keeping
  deletion not-authorized;
- typed canonical lifecycle/outcome receipt validation for requested,
  authorized, attempted, succeeded, failed, and `eligibility-lost` states;
- versioned cleanup target, policy-reference, hold-reference, request, and
  result/readiness/revalidation/lease/preflight/receipt schemas with
  representative fixtures;
- typed, caller-owned generic authorization handoff and result validation;
- read-only cleanup planning with no storage mutation.

Deferred until the external authority and destructive workflow are approved:

- external retention, legal-hold, identity, and authorization adapters;
- live lease/fencing coordinator integration and acquisition;
- destructive worker and post-delete verification;
- durable receipt persistence, tombstones, and remote deletion.
