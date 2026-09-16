# Hammond status

## Current slice

The local v1 governance slice is implemented:

- strict governance record, event, and policy decoding with policy-byte
  verification;
- contract-artifact verification on record decoding and registration, without
  interpreting Sorna semantics;
- explicit local-only artifact loading that rejects URL schemes before
  filesystem access;
- a local content-addressed artifact store with immutable digest-keyed blobs;
- contract identity and artifact SHA-256 binding;
- review-cycle-bound approval and rejection events;
- policy-aware lifecycle validation, with a one-distinct-actor approval as the
  default policy, optional required-role coverage, per-role distinct-actor
  thresholds, and a digest-bound local authority snapshot for actor-role
  grants;
- an `AuthorityVerifier` runtime seam for future verified organization
  membership providers, including the decision timestamp;
- a time-scoped membership adapter with explicit effective and expiry windows;
- digest-bound, optionally signed membership snapshots for normalized provider
  responses, with explicit freshness checks;
- a bounded HTTP membership transport adapter with a caller-owned
  authentication hook, static or resolved endpoint, endpoint policy, and
  redirect-target checks;
- a provider-neutral exact host/port endpoint allowlist usable as that policy;
- a generic bearer-token authenticator backed by a caller-owned token source;
- a caller-owned normalization hook that can reject incomplete provider views
  before Hammond verifies the normalized envelope;
- optional membership references on decision events for authority provenance;
- provenance-carrying membership verifiers that bind decision references to the
  snapshot used for authorization;
- optional Ed25519 authority-artifact signatures verified against a caller-owned
  trusted key set;
- versioned trust snapshots with active/revoked key rotation and monotonic,
  identity-preserving successor checks;
- optional root signatures on trust snapshots, enabling a verified local
  root-to-authority key chain;
- versioned root-key snapshots with bootstrap/predecessor signature checks and
  fail-closed monotonic root rotation, with bootstrap pins for root identity,
  initial version, and public keys;
- append-only file storage with atomic writes and process-shared locking;
- conditional event appends with deterministic revision conflict detection;
- amendment and supersession lineage checks; and
- a local CLI for registration, review events, amendments, supersession, and
  lineage inspection, with revision-aware conditional mutations.

## Next boundary

The next design decision is organization integration: provider-specific
credential implementation and deployment-level TLS/network configuration. The
current local authority, membership, trust, and root snapshots are explicit,
digest-bound inputs; signatures are only meaningful when checked against a
caller-owned, root-approved trust set. Provider selection is required before
that adapter can be implemented without guessing at credential scope or role
mapping.

## Deferred

Hammond still does not provide secure root bootstrap delivery, a hosted API,
identity provider, provider-specific credential implementation, deployment TLS
configuration, hosted artifact retention/garbage collection, semantic
concurrent merge handling, or automatic approval from Sorna results.
