# Hammond status

## Current slice

The local v1 governance slice is implemented:

- strict governance record, event, and policy decoding with policy-byte
  verification;
- contract identity and artifact SHA-256 binding;
- review-cycle-bound approval and rejection events;
- policy-aware lifecycle validation, with a one-distinct-actor approval as the
  default policy, optional required-role coverage, and local actor-role grants;
- append-only file storage with atomic writes;
- amendment and supersession lineage checks; and
- a local CLI for registration, review events, amendments, supersession, and
  lineage inspection.

## Next boundary

The next design decision is policy authority: organization identity, verified
role membership, and advanced quorum rules. Those should extend the versioned
policy artifact before they are accepted by a hosted registry or CI gate.

## Deferred

Hammond still does not provide a hosted API, identity provider, artifact blob
storage, concurrent merge handling, or automatic approval from Sorna results.
