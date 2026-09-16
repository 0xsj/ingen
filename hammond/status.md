# Hammond status

## Current slice

The local v1 governance slice is implemented:

- strict governance record, event, and policy decoding with policy-byte
  verification;
- contract-artifact verification on record decoding and registration, without
  interpreting Sorna semantics;
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
  authentication hook, static or resolved endpoint, and endpoint policy;
- a caller-owned normalization hook that can reject incomplete provider views
  before Hammond verifies the normalized envelope;
- optional Ed25519 authority-artifact signatures verified against a caller-owned
  trusted key set;
- versioned trust snapshots with active/revoked key rotation;
- optional root signatures on trust snapshots, enabling a verified local
  root-to-authority key chain;
- versioned root-key snapshots with bootstrap/predecessor signature checks and
  fail-closed root rotation;
- append-only file storage with atomic writes;
- amendment and supersession lineage checks; and
- a local CLI for registration, review events, amendments, supersession, and
  lineage inspection.

## Next boundary

The next design decision is organization integration: provider credential
implementation and deployment network policy. The current local authority,
membership, trust, and root snapshots are explicit,
digest-bound inputs; signatures are only meaningful when checked against a
caller-owned, root-approved trust set.

## Deferred

Hammond still does not provide secure root bootstrap delivery, a hosted API,
identity provider, provider-specific credential implementation, artifact blob
storage,
concurrent merge handling, or automatic approval from Sorna results.
