# Membership snapshot rotation must reject replay

Membership freshness and membership ordering answer different questions. A
snapshot can be fresh enough for evaluation while still being older than the
latest snapshot the provider issued.

## What

`MembershipSnapshot.Rotate` verifies a signed successor with a caller-supplied
provider trust set, requires the same membership ID, and requires a strictly
increasing version. The caller can still apply `FreshAt` or `VerifierAt` to the
returned snapshot before policy evaluation.

`FileMembershipVersionStore` can persist the latest accepted reference across
processes. It stores only the reference, uses an exclusive file lock for the
read-compare-write step, accepts equal references idempotently, and rejects
lower or same-version different-digest references. It also binds the stored
membership ID to the deterministic ledger path.

## Why

This keeps versioned provider state monotonic at the local handoff boundary.
The snapshot reference and digest identify the exact normalized bytes, while
the version transition prevents a valid older snapshot from being replayed as
the current one.

## Gotchas

- Freshness does not establish ordering; callers must retain the accepted
  snapshot or an equivalent durable version precondition.
- The provider trust set, key rotation, and persistence of the latest version
  remain caller-owned; the optional local ledger provides only the persistence
  primitive and does not verify signatures.
- A valid signature authenticates the snapshot bytes and issuer key, not the
  provider's completeness or group-to-role mapping.
- The local ledger validates its own reference structure but is not
  cryptographically authenticated; protect its directory from unauthorized
  filesystem writers.

## Used in

- `hammond/internal/governance/membership.go`
- `hammond/internal/governance/governance_test.go`
- `hammond/internal/store/membership.go`
- `hammond/internal/store/membership_test.go`
- `hammond/GOVERNANCE-SPEC.md`
