# Hammond roadmap

Last reviewed: 2026-09-17

Hammond is currently a local v1 governance module, not a hosted service. Its
job is to govern identified contract artifacts, review decisions, amendments,
and lineage while leaving contract behavior and evidence to Sorna.

## Current baseline — complete

### Governance model

- Strict decoding and validation for governance records, events, policies,
  authority artifacts, trust roots, root snapshots, and membership snapshots.
- Contract identity and exact artifact SHA-256 binding.
- Review-cycle-bound approvals and rejections.
- Lifecycle transitions, amendments, supersession, and semantic lineage
  checks.
- Default one-distinct-actor approval policy, with role requirements and
  distinct-actor thresholds.
- Decision-time authority evaluation and optional membership provenance.

### Local persistence and CLI

- Append-only file-backed records with atomic publication, file and directory
  synchronization, and process-shared locking.
- Conditional mutations with deterministic revisions and conflict detection.
- Content-addressed local artifact blobs with immutable digest-derived paths.
- Monotonic membership-version ledger storing references only.
- CLI support for registration, events, amendments, supersession, lineage,
  and revision-aware mutations.
- Registry and lineage reads resolve each record's referenced review policy
  before validation, so custom local policies remain valid after reload.

### Trust and membership boundaries

- Optional Ed25519 authority-artifact signatures.
- Active/revoked authority trust snapshots with identity-preserving,
  monotonic rotation.
- Caller-owned root bootstrap pins, signed root rotation, and root-bound trust
  loading.
- Trust-store-bound policy and authority loaders for an explicit
  root → trust → authority chain.
- Effective-dated membership grants, freshness checks, signed normalized
  snapshots, monotonic snapshot rotation, and provenance binding.
- Provider-neutral HTTP transport with caller-owned authentication, endpoint
  resolution, exact host/port policy, redirect checks, and normalization.

### Verification and documentation

- Hammond tests pass with `go test ./hammond/...`.
- Formatting and whitespace checks pass.
- The implementation status, governance specification, and related learning
  notes are maintained alongside the code.

## Recommended next sequence

### 1. Prove the solo local approval workflow — complete

For the current solo workflow, use a local authority artifact and one local
actor. The runnable fixture is in
[`examples/solo/`](examples/solo/README.md) and covers:

- one actor mapped to the `product-reviewer` role;
- one review cycle and one approval event; and
- local file-backed registration and append operations.

No organization, browser session, GitHub token, or external membership source
is required for this phase.

The first concrete governed artifact was the repository's document-pipeline v2
contract at
[`examples/document-pipeline/contract-v2.canonical.json`](examples/document-pipeline/contract-v2.canonical.json).
Its exact SHA-256 is
`6b40dfb15fa67f96c9f3bc79bc46206d45f6d44124197b344757499299e43445`.
It was preflight-validated, registered, opened for review, and approved before
an additive v3 contract was created at
[`examples/document-pipeline/contract-v3.canonical.json`](examples/document-pipeline/contract-v3.canonical.json).
The v3 digest is
`c8f7f9f675f6355f7eda3cc3ee2a3f6a84ee8fdf209ee7bd35defae16e528745`.
The gitignored local store now records v2 as superseded and v3 as approved.
The document-pipeline lab suite passes against the v3 behavior.

### 2. Keep the GitHub adapter optional — prepared

The GitHub team adapter is implemented and tested in isolation, but remains
deferred until a GitHub organization and shared-reviewer use case exist. Its
future boundary is captured in the [provider decision note](../notes/decision-adjacent/hammond-provider-integration-boundary.md)
and [provider checklist](PROVIDER-INTEGRATION-CHECKLIST.md).

### 3. Close the solo operational boundary — complete

The solo example now uses revision-aware appends, so a concurrent writer cannot
silently replace the record between registration, review opening, and approval.
The single-machine operating guidance is captured in
[SOLO-OPERATIONS.md](SOLO-OPERATIONS.md). The solo boundary is now explicit:

- use the local `validate` preflight before registration;
- use revision-aware mutations for every event append; and
- keep the authority unsigned while it remains inside one private filesystem
  boundary.

Keep the local governance model explicit; do not add hosted or organization
infrastructure until the solo workflow exposes a real need.

### 4. Define operational trust bootstrap — conditional

Specify how deployment configuration delivers the initial root pins, how root
rotation is approved, and how recovery works after compromise. This belongs to
deployment operations; Hammond can validate the supplied pins and signatures
but cannot make an out-of-band delivery channel secure by itself.

This is deferred until one of the solo trust-boundary revisit triggers occurs.

Use the [root-bootstrap operations checklist](ROOT-BOOTSTRAP-OPERATIONS.md) as
the preparation document.

### 5. Add a hosted boundary only after the local model is exercised

If multiple projects or workspaces need shared governance, define a hosted
API around the local model. Decide authentication, tenancy, authorization,
artifact references, idempotency, audit export, and optimistic concurrency
before choosing a database or API framework.

### 6. Add hosted artifact retention and lifecycle

Define retention, pinning, garbage collection, replication, and recovery for
hosted artifacts. The local content-addressed store is an integrity and
locator primitive; it is not a hosted retention system.

### 7. Decide semantic concurrent merges

The current store detects stale revisions and returns conflicts. If Hammond
must merge concurrent amendments or decisions, define domain-specific merge
rules and audit semantics first; do not silently merge event histories.

### 8. Define Sorna automation explicitly

Only after an evidence contract is agreed should Sorna results automatically
create approvals, amendments, or other Hammond events. Evidence validity,
producer identity, policy scope, and replay behavior must be explicit.

## Deferred until a concrete decision

- Hosted Hammond API and database persistence.
- Organization identity provider and provider-specific credentials.
- Deployment TLS, network segmentation, and secure root bootstrap delivery.
- Hosted artifact retention, replication, and garbage collection.
- Semantic concurrent merge handling.
- Automatic approval or amendment from Sorna results.
- UI and broader operator workflows.

## Exit criteria for the next phase

The next phase is ready to close when one selected provider can produce a
complete, verified, fresh, monotonic membership snapshot; a policy can use it
to authorize a decision at the event timestamp; the event records matching
membership provenance; and the end-to-end path is covered by repeatable tests
and a provider-specific note.
