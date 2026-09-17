# Hammond status

See the [Hammond roadmap](roadmap.md) for the completed milestones, decision
gates, and recommended next sequence.

## Current slice

The local v1 governance slice is implemented:

- strict governance record, event, and policy decoding with policy-byte
  verification;
- contract-artifact verification on record decoding and registration, without
  interpreting Sorna semantics;
- explicit local-only artifact loading that rejects URL schemes before
  filesystem access;
- a local content-addressed artifact store with immutable digest-keyed blobs;
- a local membership-version ledger that stores only the latest accepted
  reference and rejects stale or same-version conflicting snapshots;
- contract identity and artifact SHA-256 binding;
- review-cycle-bound approval and rejection events;
- policy-aware lifecycle validation, with a one-distinct-actor approval as the
  default policy, optional required-role coverage, per-role distinct-actor
  thresholds, and a digest-bound local authority snapshot for actor-role
  grants;
- store mutations that reload and apply each record's referenced policy,
  including custom solo authority mappings;
- registry and lineage reads that resolve each stored record's referenced
  policy before validation, including custom solo policies;
- a runnable solo local-authority approval fixture with revision-aware CLI
  appends and regression coverage;
- a local `validate` CLI preflight that checks the record, contract, policy,
  and authority artifacts before registration;
- documented single-machine solo operations covering permissions, backups,
  revision checks, and the threshold for adding issuer signing;
- an `AuthorityVerifier` runtime seam for future verified organization
  membership providers, including the decision timestamp;
- a time-scoped membership adapter with explicit effective and expiry windows;
- digest-bound, optionally signed membership snapshots for normalized provider
  responses, with explicit freshness checks and monotonic successor checks;
- a bounded HTTP membership transport adapter with a caller-owned
  authentication hook, static or resolved endpoint, endpoint policy, and
  redirect-target checks, passing the final response endpoint to
  normalization;
- a provider-neutral exact host/port endpoint allowlist usable as that policy;
- a generic bearer-token authenticator backed by a caller-owned token source;
- a caller-owned normalization hook that can reject incomplete provider views
  before Hammond verifies the normalized envelope;
- normalized-fetch helpers that apply freshness and expose timestamp-aware
  verifiers with optional membership provenance;
- a selected GitHub team-membership adapter that paginates the official REST
  endpoint, excludes inherited child-team entries, maps stable numeric user IDs
  to one configured Hammond role, and verifies a caller-signed snapshot;
- provider-neutral end-to-end coverage from normalized membership bytes through
  policy authorization and decision provenance;
- optional membership references on decision events for authority provenance;
- provenance-carrying membership verifiers that bind decision references to the
  snapshot used for authorization;
- optional Ed25519 authority-artifact signatures verified against a caller-owned
  trusted key set;
- trust-store-bound policy and authority loaders for an explicit local
  root-to-trust-to-authority chain;
- versioned trust snapshots with active/revoked key rotation and monotonic,
  identity-preserving successor checks;
- optional root signatures on trust snapshots, enabling a verified local
  root-to-authority key chain;
- versioned root-key snapshots with bootstrap/predecessor signature checks and
  fail-closed monotonic root rotation, with bootstrap pins for root identity,
  initial version, and public keys;
- append-only file storage with file-and-directory-synced atomic writes and
  process-shared locking;
- conditional event appends with deterministic revision conflict detection;
- amendment and supersession lineage checks; and
- a local CLI for registration, review events, amendments, supersession, and
  lineage inspection, revision-aware conditional mutations, and read-only
  membership-version inspection.

## Active boundary

The current workflow is solo and local, and its active boundary is complete:
local authority, read-only preflight, revision-aware mutations, private
filesystem handling, and backup guidance are documented. The local authority
remains unsigned while it stays inside one private filesystem boundary.

The first concrete governed artifact was the document-pipeline v2 contract at
[`examples/document-pipeline/contract-v2.canonical.json`](examples/document-pipeline/contract-v2.canonical.json).
It is now superseded by the additive v3 contract at
[`examples/document-pipeline/contract-v3.canonical.json`](examples/document-pipeline/contract-v3.canonical.json),
whose exact SHA-256 is
`c8f7f9f675f6355f7eda3cc3ee2a3f6a84ee8fdf209ee7bd35defae16e528745`.
The gitignored local store contains the approved v3 record and the validated
v2 -> v3 lineage.
The document-pipeline subject and defect fixtures pass against the updated
contract behavior.

No GitHub organization, browser session, token source, or external membership
provider is required.

The GitHub team adapter remains an isolated, tested future option for shared
reviewers across projects. The local authority, membership, trust, and root
snapshots remain explicit, digest-bound inputs. Signing and root bootstrap are
conditional future work, activated only when authority crosses a trust
boundary.

## Deferred

Hammond still does not provide secure root bootstrap delivery, a hosted API,
identity provider, provider-specific credential implementation, deployment TLS
configuration, hosted artifact retention/garbage collection, semantic
concurrent merge handling, or automatic approval from Sorna results.
