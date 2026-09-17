# Lockwood roadmap

Status: active local custody vertical. Updated 2026-09-17.

Lockwood's core responsibility is to preserve what happened: immutable
artifacts, custody metadata, append-only handling history, lineage, and
detached verification evidence. It should remain a custody boundary rather
than becoming the system that performs redaction, decides policy, or asserts
human identity by implication.

The detailed boundary is in [`CUSTODY-SPEC.md`](CUSTODY-SPEC.md), the detached
signature contracts are in [`SIGNATURE-SPEC.md`](SIGNATURE-SPEC.md), and the
planning tree is in [`PROPOSED-TREE.md`](PROPOSED-TREE.md).

The draft next-slice handoff contract is in
[`AUTHORIZATION-HANDOFF.md`](AUTHORIZATION-HANDOFF.md).

## Completed foundation

### 1. Artifact identity and local storage — complete

- SHA-256 content-addressed artifact references.
- Atomic, durable filesystem publication with verification.
- In-memory store for tests and short-lived workflows.
- Size limits, expected-digest checks, reference manifests, and deterministic
  inventory.
- Read-only reconciliation for corrupt blobs, dangling records, orphan grace,
  and recognized detached-artifact protection.

### 2. Custody records and lineage — complete

- Canonical custody record schemas v1 and v2.
- Credential-free remote source URI/version metadata in v2.
- Accepted, quarantined, and rejected states with integrity requirements.
- Canonical record digests and explicit lineage relations.
- Lineage verification with unresolved-parent and cycle detection.
- Recovery and reconciliation paths that preserve original records and blobs.

### 3. Intake adapters — complete

- Deterministic Sorna evidence-bundle import.
- Validated `ingen.ci-result/v1` import.
- Shared streaming integrity and size-limit behavior across intake paths.

### 4. Append-only handling history — complete

- Redaction, retention-classification, and legal-hold events.
- Deterministic event storage and idempotent/conflict-safe append behavior.
- Read-only handling status projection.
- Read-only legal-hold guard and explicit handling-action policy snapshot.
- Local process/filesystem coordination for concurrent event mutations.

### 5. Redaction custody lifecycle — complete

- Caller-produced redaction result registration.
- Original artifact and custody-record preservation.
- Accepted result promotion with explicit `derived-from` lineage.
- Read-only redaction status and traceability projection.
- Clear boundary that Lockwood does not perform or semantically validate the
  payload transformation.

### 6. Detached signatures and provenance — complete

- Detached Ed25519 custody-record attestations.
- Detached handling-event signatures.
- Detached redaction-promotion provenance envelopes binding:
  source record, event, original/result artifacts, and promoted record.
- Canonical JSON encoding, schemas, fixtures, and content-addressed envelope
  publication.
- Canonical relationship digest for redaction-promotion authorization handoff,
  separate from detached signer and envelope identity.
- Explicit trust registry with active/revoked keys and validity windows.
- Direct and trusted verification receipts.
- Sign, import, inspect, link-inspect, find, trusted-find, and verify CLI paths.

### 7. Verification and documentation — complete

- Normal and race-enabled Lockwood test suites passing.
- Contract, schema, CLI, concurrency, recovery, and tamper tests.
- README, custody specification, signature specification, proposed tree, and
  handling-event notes updated as the implementation expanded.

## Current assurance boundary

Lockwood can establish that particular bytes, records, events, relationships,
and signatures exist and verify under explicitly supplied inputs. It does not
currently establish:

- the human identity behind a key or descriptive `actor` field;
- authorization to perform a handling action or endorse a result;
- correctness or safety of a redaction transformation;
- deletion, retention, or legal-hold enforcement;
- authenticity of remote objects without a provider-authenticated retrieval
  result; the local remote seam verifies bytes but does not authenticate a
  provider.

These are deliberate boundaries, not missing checks to add implicitly.

Identity boundary: Amber supplies portable provenance and identity
relationships. Lockwood should consume an explicitly defined external
identity/authorization reference or snapshot; it should not create a human
identity directory or silently take ownership of Sentinel lifecycle audit.

## Candidate next steps

### P0. Align status and planning documents — complete

The opening Lockwood status language now identifies the implemented local
custody vertical. Deferred-area sections remain explicit and distinguish the
working foundation from future expansion.

### P1. Define the identity and authorization handoff — contract and local seam complete

Design an explicit cross-module authorization boundary before destructive
workflows are introduced:

- define the external principal or identity relationship supplied by Amber or
  another governance owner;
- map trusted key IDs to that external relationship without treating a key ID
  as a human identity;
- define authorization for handling actions and redaction-provenance
  endorsements;
- decide whether the existing handling-event policy should extend to
  provenance targets or whether a separate policy snapshot is clearer;
- define revocation, evaluation time, policy snapshot digests, and transient
  authorization receipts;
- specify how an externally authenticated actor is distinguished from a
  descriptive event claim;
- keep Sentinel-side artifact registration and lifecycle audit outside this
  boundary unless a concrete handoff contract is approved.

Authority-specific identity, signature, policy, and revocation integration
remain deferred to the consuming application.

### Authorization handoff contract and implementation record

The contract is in [`AUTHORIZATION-HANDOFF.md`](AUTHORIZATION-HANDOFF.md). Its
authority-neutral shape records:

1. A short contract defining which party authenticates the actor, which party
   authorizes the action, and what Lockwood records as an external reference.
2. A versioned policy or assertion shape with canonical encoding, snapshot
   digest, evaluation time, revocation behavior, and fail-closed rules.
3. Fixtures covering a valid handoff, an unknown/revoked key, an expired
   assertion, a mismatched principal relationship, and a stale policy.
4. A transient receipt that keeps key verification, external identity
   reference, authorization decision, and custody/provenance verification
   distinct.

The target-binding foundation is now implemented: redaction-promotion
relationships have a stable canonical digest that can be referenced by a
future external authorization assertion. Identity and authorization remain
out of scope until that external contract is defined. The handoff draft now
also proposes a versioned envelope with exact action binding, opaque principal
references, external assertion/policy digests, canonical validity windows, and
revocation snapshot semantics. It now also defines the caller-owned verification
adapter boundary and the fail-closed fixture matrix; implementation remains
authority-neutral by default, with any concrete external authority deferred to
the consuming application. The typed read-only handoff validator now enforces
canonical JSON, target/action binding, digest shape, and validity windows;
the typed adapter-result validator now enforces local result binding and
explicit non-authorized failure codes. External signature, identity, policy,
and revocation verification remain outside Lockwood. A read-only orchestration
entry point now validates requests, delegates to the caller-owned verifier, and
returns a transient receipt without persisting authorization state. Typed target
constructors now derive custody-record, handling-event, and redaction-promotion
target identities from validated local objects, and the handoff builder derives
the action from the target kind before validation.

Out of scope for this slice: human identity storage, key-management service
integration, Sentinel registration changes, payload redaction, deletion,
retention enforcement, remote object retrieval, and distributed locking.

### P1. Define remote/object-storage semantics

The provider-neutral contract is now in
[`REMOTE-OBJECT-SPEC.md`](REMOTE-OBJECT-SPEC.md). It defines:

- immutable remote references and retrieval behavior;
- authentication and credential handling;
- URI/version/ETag semantics and digest verification;
- retry, cache, and failure behavior;
- whether remote references can participate in custody acceptance and lineage.

The local `internal/remote` seam now validates canonical credential-free
references and verifies caller-owned fetch results against URI/version/ETag
pins, expected digest, and expected size. It performs no provider I/O, stores
no credentials, and does not publish remote bytes or custody records. Custody
v2 continues to record remote source metadata only.

### P1. Define cleanup and enforcement boundaries

The operational boundary contract is now in
[`CLEANUP-ENFORCEMENT-SPEC.md`](CLEANUP-ENFORCEMENT-SPEC.md). It specifies:

- retention and legal-hold enforcement;
- deletion authorization and audit behavior;
- race-safe orphan cleanup, including distributed coordination;
- protected-artifact behavior during cleanup and recovery races.

The read-only `cleanup-plan` projection now records explicit `as_of` and grace
inputs, distinguishes reported orphans from age-qualified candidates, and
keeps every action `not-authorized` with policy, legal-hold, revalidation, and
authorization blockers. Destructive enforcement remains deferred and must
follow the policy contract rather than silently turning the read-only guard
into permission or deletion.

The next handoff is documented in
[`CLEANUP-AUTHORIZATION.md`](CLEANUP-AUTHORIZATION.md). It proposes an exact
local artifact target bound to the canonical cleanup plan, a versioned external
policy/legal-hold snapshot, an action-bound authorization result, and explicit
outcome receipts. The typed target kind and read-only validation seam are now
implemented, but the workflow remains design-only until the external
authority, coordinator, and tombstone policy are approved; no deletion worker
or external cleanup authority is implemented. The read-only typed target,
request/result validator, and `cleanup-authorization-target/v1` schema now
cover the proposed binding. Canonical request/result transport and their
versioned schemas, plus target-bound policy and legal-hold references, are also
covered without granting permission or mutating storage. A read-only readiness
projection now verifies the plan digest and all evidence bindings, then stops
at `ready-for-revalidation` while retaining the fresh-check and coordination
blocker. Target-bound revalidation and lease/fencing evidence plus a
`ready-for-worker` preflight projection are now validated as read-only inputs;
live coordination and the destructive worker remain deferred.
The typed lifecycle receipt contract now validates every outcome state,
including `eligibility-lost`, without persisting receipts or asserting a
post-delete success.

### P2. Strengthen cross-module evidence workflows

The cross-module handoff draft is now in
[`CROSS-MODULE-EVIDENCE-SPEC.md`](CROSS-MODULE-EVIDENCE-SPEC.md). It maps
Sorna evidence, CI results, Hammond governance artifacts, and Nublar
run/delivery receipts through exact local digests while preserving producer
ownership.

The deterministic end-to-end fixture path is now implemented in
[`cross_module_evidence_test.go`](cross_module_evidence_test.go). It verifies
exact byte preservation, producer-owned failed statuses, deterministic Sorna
archive identity, explicit digest-bearing lineage, separate Nublar delivery
receipts, expected-digest rejection, malformed-input rejection, and
unresolved-parent verification failure without live services.

A first-class catalog relation remains deferred until a concrete consumer
requires more than digest-bearing lineage. Dedicated Hammond/Nublar intake
adapters also remain deferred; the generic custody boundary and the offline
fixture are sufficient until a concrete consumer requires producer-specific
normalization.

### P2. Improve query and operator surfaces

- The current deterministic catalog remains the query boundary; no persisted
  search index is required for this slice.
- The read-only `verify-report` command now batch-verifies catalog matches,
  preserves per-record failures, and returns a versioned report without
  changing custody status.
- Query validation now rejects malformed digests, statuses, and lineage
  relations before a catalog scan; combined filters and deterministic empty
  results are covered by library and CLI tests.
- Decide whether a persisted search index is needed only after measured query
  volume or a concrete operator workflow requires it.
- Consider an API or web surface only after the local CLI and governance
  boundaries are stable.

## Suggested order

1. Keep the module status language and roadmap current as boundaries evolve.
2. Draft the identity/authorization contract and its decision points.
3. Review that contract against handling events and redaction provenance.
4. Define remote storage and cleanup policies separately from authorization.
5. Implement only the next approved boundary, with schema fixtures and normal
   plus race-enabled tests.

## Decision gates

Before adding a future capability, answer:

- What exact bytes or state does Lockwood preserve?
- Is the operation read-only, append-only, or destructive?
- Which party supplies identity, authorization, and trust?
- What fails closed, and what evidence is emitted?
- Does the capability preserve the original artifact and custody history?
- Can the contract be tested without depending on a particular provider or
  user interface?
