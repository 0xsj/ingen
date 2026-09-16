# Hammond Governance Specification

Status: working v1 design

Hammond records cross-project governance decisions about contract artifacts. It
does not interpret contract rules, generate oracles, run subjects, or produce
verification evidence.

## 1. Scope

The first Hammond slice governs one immutable contract artifact at a time. It
can:

- register a contract reference;
- move the reference through review states;
- record decisions against the exact artifact digest;
- create a successor amendment without changing the predecessor; and
- show the resulting contract lineage.

The first implementation is local and file-backed. The record shape must not
depend on that storage choice.

## 2. Ownership boundaries

| Concern | Owner |
| --- | --- |
| Contract syntax and behavioral rules | Sorna |
| Contract canonicalization and sealing | Sorna |
| Oracle generation, verification, mutation, and evidence | Sorna |
| Local workspaces, roles, and operator workflow | Sentinel |
| Review decisions, approvals, amendments, and lineage | Hammond |

Hammond stores a reference to a contract artifact. It does not copy the
contract body into a second authoritative format.

## 3. Core identity

A governed contract version is identified by the combination of:

```text
project_id + contract_id + version + artifact_sha256
```

The artifact digest is the SHA-256 of the canonical contract bytes consumed by
the governed workflow. A path or URI is useful for retrieval, but it is not an
identity and must never replace the digest. The local contract, policy, and
authority loaders accept filesystem paths only; they do not turn URL schemes
into network requests. Membership HTTP transport is a separate, explicit
boundary.

At a local ingress boundary, Hammond may load the referenced artifact and
verify that its bytes produce the recorded digest. This check establishes byte
integrity only; Sorna remains responsible for contract interpretation,
canonicalization, sealing, and verification.

The reference should contain:

```yaml
contract:
  project_id: document-pipeline
  id: document-pipeline
  version: 1
  schema: ingen.contract/v1
  artifact:
    uri: hammond/examples/document-pipeline/contract-v2.canonical.json
    sha256: <64 lowercase hexadecimal characters>
```

The `schema`, `id`, and `version` fields describe the Sorna contract. Hammond's
own record schema is separate and will be versioned as
`ingen.hammond-governance/v1`.

## 4. Governance lifecycle

Hammond's state is separate from the Sorna contract's `draft`, `sealed`, and
`superseded` status:

```text
registered -> in_review -> approved
                       \-> rejected

approved -> superseded
rejected  -> in_review
```

Rules:

- `registered` requires a contract reference and source identity.
- `in_review` means a review cycle has been opened for that exact identity.
- `approved` means the recorded approvals satisfy the active review policy.
- `rejected` preserves the reason and may begin a later review cycle.
- `superseded` is terminal for that governed version and points to a successor.
- An approved version cannot be edited or moved back to review.
- A new version is created through an amendment or successor record, and an
  amendment is created only from an approved predecessor.
- A predecessor is superseded only after its successor has been independently
  approved and the amendment link already exists.

Hammond does not infer that an approved artifact is behaviorally correct. It
records that the required governance decision was made for identified bytes.

## 5. Review decisions

Every decision is bound to the full contract identity and must include:

- a stable decision ID;
- the review cycle ID opened by the current review;
- reviewer identity;
- reviewer role or authority;
- decision: `approve` or `reject`;
- the exact artifact SHA-256;
- a UTC timestamp; and
- an optional rationale or review reference.

An approval or rejection may also carry a `membership` reference identifying
the normalized authority snapshot used by the caller. Hammond validates that
reference's shape but does not load it. When the caller uses Hammond's
provenance-carrying membership verifier, Hammond also requires the event
reference to match; opaque custom verifiers cannot be introspected.

A decision for a different version, contract ID, project, or artifact digest
cannot satisfy the review for the governed record.

A decision from an earlier review cycle cannot satisfy a later review. Review
cycle IDs must be unique within a governance record.

The v1 domain model records decisions and evaluates them against a supplied
review policy artifact. The default policy requires one approval from one
distinct actor in the active cycle. A policy may also require named approval
roles, an overall approval threshold, and per-role distinct-actor thresholds
through `role_approval_thresholds`. A policy may reference a separately
versioned, digest-bound local authority artifact containing actor-to-role
grants. The model does not yet verify
organization-wide identity or role authority; the local snapshot is only an
explicit input to policy evaluation. Runtime callers may provide an authority
verifier, but Hammond v1 does not ship an organization-backed verifier.
The verifier receives the decision event's UTC timestamp so a future provider
can evaluate membership as of the historical decision rather than as of load
time. A normalized membership adapter may use inclusive `valid_from` and
exclusive `valid_until` windows; revocation and retroactivity remain provider
policy.

An organization-backed adapter may first normalize its response into a
versioned membership snapshot. The snapshot carries `issued_at`, effective
grant windows, an optional `expires_at`, and an optional Ed25519 issuer
signature. Hammond can verify that response and expose its grants to the
timestamp-aware authority seam. A caller may enforce maximum age, future clock
skew, and expiry before doing so. The optional `HTTPMembershipProvider` only
fetches bounded response bytes and invokes caller-owned authentication,
endpoint-resolution, and endpoint-policy hooks; provider credentials, TLS and
discovery policy, normalization, completeness, and response semantics remain
outside Hammond. Its `FetchNormalized` path verifies the digest-bound envelope
returned by that normalizer. Hammond provides an exact host/port
`MembershipEndpointAllowlist` helper for callers that want a reusable endpoint
policy; it does not resolve DNS, validate certificates, or enforce network
segmentation. Configured HTTPS and endpoint policies are reapplied to redirect
targets before the HTTP client follows them.

The transport package may provide a bearer-token request adapter, but token
acquisition, storage, refresh, rotation, and scope remain caller-owned.

An authority artifact may also carry an Ed25519 signature with a `key_id`.
The signature covers the canonical JSON payload formed from `schema`, `id`,
`version`, and `actors`, excluding the signature object itself. Signature
verification is opt-in through a caller-owned trusted key set; a digest proves
which bytes were loaded, while the signature identifies an issuing key.

The caller-owned trust input may itself be a versioned Hammond trust snapshot.
It can retain old keys as `revoked` while introducing a new `active` key, so
rotation rejects signatures made by the revoked key. The trust snapshot is a
configuration input, not a self-authenticating root of trust. Its rotation
helper preserves the trust ID and requires a strictly increasing version,
preventing a previously valid trust snapshot from being replayed.
When the root layer is available, callers can bind that rotation directly to
the active keys of a validated `AuthorityRootStore`.

The caller may keep the root layer as a separate versioned root snapshot.
`AuthorityRootStore` exposes only its active keys and can verify a replacement
root snapshot with a bootstrap or predecessor root set. This makes root-key
rotation explicit and fail-closed for revoked keys; initial root-key delivery,
approval, and any multi-party rotation rule remain caller-owned. The bootstrap
must pin the expected root ID, initial version, and public keys; a snapshot
that differs from those pins is rejected. Its rotation helper also requires a
stable root ID and strictly increasing version.

For a stronger local chain, the trust snapshot may carry its own Ed25519 root
signature. Hammond verifies that signature with a separately configured root
key set before exposing active keys to authority-artifact verification. This
binds key rotation to an approved snapshot but does not define how the root
key set is delivered.

Policy evaluation must use bytes that have been strictly decoded and whose
SHA-256 matches the policy reference carried by the governance record.

## 6. Amendments and lineage

An amendment creates a new contract version. It must contain:

- the complete identity of the predecessor;
- the complete identity of the successor;
- an amendment kind;
- a human-readable reason; and
- the author and UTC creation time.

The amendment kind follows the Sorna contract vocabulary:

- `clarifying` — intended behavior is unchanged;
- `additive` — a new requirement is added;
- `restrictive` — permitted behavior is narrowed;
- `corrective` — an incorrect requirement is changed; or
- `breaking` — prior compatibility is intentionally invalidated.

The predecessor remains readable and its review history remains attached to
its original digest. A successor must be governed independently; approval of a
predecessor never carries forward automatically.

The initial lineage model uses one predecessor link per amendment. It may
represent branches, but it must reject cycles and self-links.

## 7. Record shape

The first exchange artifact is a governance record containing one contract
reference and its append-only event history:

```yaml
schema: ingen.hammond-governance/v1
record_id: document-pipeline-v1
contract:
  project_id: document-pipeline
  id: document-pipeline
  version: 1
  schema: ingen.contract/v1
  artifact:
    uri: examples/document-pipeline-lab/contract/contract.yaml
    sha256: <digest>
policy:
  id: single-approval
  version: 1
  schema: ingen.hammond-review-policy/v1
  artifact:
    uri: hammond/examples/review-policy-v1.json
    sha256: <policy-digest>
state: approved
events:
  - id: event-001
    type: registered
    actor: contract-owner
    at: 2026-09-15T00:00:00Z
  - id: event-002
    type: review-opened
    actor: contract-owner
    at: 2026-09-15T00:01:00Z
    review_cycle_id: review-001
  - id: event-003
    type: approval-recorded
    actor: reviewer@example.test
    role: product-reviewer
    review_cycle_id: review-001
    decision: approve
    artifact_sha256: <digest>
    at: 2026-09-15T00:02:00Z
```

The `state` field is a materialized value derived from valid events. The
events remain the audit history and must not be silently rewritten.

## 8. Required invariants

The v1 validator must reject:

- a missing or malformed contract identity;
- a non-lowercase 64-character SHA-256 digest;
- an event bound to a different contract identity or digest;
- duplicate event IDs;
- invalid lifecycle transitions;
- an approval without reviewer, role, decision, digest, or timestamp;
- an approval or rejection that is not bound to the active review cycle;
- an approval or rejection whose actor/role pair is not granted by the loaded
  authority snapshot;
- an invalid membership reference, or a membership reference on a non-decision
  event;
- a provenance-carrying membership verifier when the decision reference is
  missing or does not match;
- a review that fails an overall or per-role distinct-actor approval threshold;
- an authority verifier error or a decision evaluated without its validated
  UTC timestamp;
- an effective-dated authority grant with an invalid or non-UTC time window;
- a stale, expired, or future-dated membership snapshot when freshness
  validation is requested;
- a root snapshot with invalid key material or a signature that is not trusted
  by the configured bootstrap or predecessor root set;
- an authority trust snapshot with no keys or invalid active-key material;
- a signed authority artifact with a missing, unknown, malformed, or invalid
  signature when signature verification is requested;
- an amendment without a predecessor, successor, kind, reason, or author;
- an amendment with a self-link or lineage cycle; and
- an attempt to mutate a superseded or approved historical record.

Validation must be deterministic and independent of the persistence backend.

## 9. First executable milestone

The first CLI or package-level workflow should demonstrate:

1. Register the existing document-pipeline contract reference.
2. Open a review for its exact artifact digest.
3. Record a valid approval and materialize `approved` state.
4. Create a linked version 2 amendment.
5. Preserve version 1 and print the version 1 -> version 2 lineage.

This milestone does not require a server, UI, database, external identity
provider, or automatic Sorna execution.

## 10. Deferred decisions

The following remain outside v1:

- organization and team identity providers, root-key bootstrap delivery and
  approval;
- organization-aware identity and role authority;
- hosted registry APIs;
- artifact blob storage and retention;
- merge conflict handling for concurrent amendments; and
- automatic approval or amendment based on Sorna results.
