# Authority trust rotation must reject replay

Hammond's authority trust snapshot is a signed keyset, not a mutable record.
Loading a valid snapshot is not enough to make a later update safe: a caller
must also reject an older snapshot that is still correctly signed.

## What

`AuthorityTrustStore.Rotate` verifies a successor against the caller-supplied
root verifier, requires the same trust ID, and requires a strictly increasing
version. `RotateWithRootStore` is the safer chain-specific entry point: it
validates the root store and derives the verifier from its active keys. The
returned store exposes only the successor's active keys; revoked keys remain
represented in the signed artifact but are not usable by its signature
verifier. The decoder and direct store validator also reject an empty keyset or
malformed active-key material; a valid all-revoked artifact is still
representable because the decoded active-key map may be empty only after the
artifact itself has supplied at least one valid revoked entry.

## Why

The root layer already had this transition rule. Applying the same rule to the
root-signed authority trust layer makes replay protection explicit at both
versioned trust boundaries. The previous store is not itself the signer of a
successor: the configured root trust set authorizes the replacement, while the
current store supplies the version precondition.

## Gotchas

- A valid signature does not authorize a different trust identity.
- Equal or lower versions are rejected even when their signatures are valid.
- The root verifier and its bootstrap/rotation policy remain caller-owned.
- Prefer `RotateWithRootStore` when the authority root is available so a
  malformed or unvalidated root cannot authorize a trust transition.
- This helper does not persist the latest version; callers must retain the
  returned store or enforce an equivalent durable state rule.

## Used in

- `hammond/internal/governance/authority_trust.go`
- `hammond/internal/governance/governance_test.go`
- `hammond/GOVERNANCE-SPEC.md`
