# Hammond status

## Current slice

The local v1 governance slice is implemented:

- strict governance record and event decoding;
- contract identity and artifact SHA-256 binding;
- review-cycle-bound approval and rejection events;
- policy-aware lifecycle validation, with a one-distinct-actor approval as the
  default policy;
- append-only file storage with atomic writes;
- amendment and supersession lineage checks; and
- a local CLI for registration, review events, amendments, supersession, and
  lineage inspection.

## Next boundary

The next design decision is policy configuration: organization identity,
authorized roles, and quorum rules. Those should become a versioned policy
artifact before they are accepted by a hosted registry or CI gate.

## Deferred

Hammond still does not provide a hosted API, identity provider, artifact blob
storage, concurrent merge handling, or automatic approval from Sorna results.
