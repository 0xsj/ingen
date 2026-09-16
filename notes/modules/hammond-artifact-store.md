# A digest-keyed blob store makes the URI a locator, not identity

Local artifact storage should derive its key from the bytes so a caller cannot
replace an approved artifact merely by reusing its path.

## Origin

Hammond already verified artifact digests at local load boundaries, but the
initial examples depended on arbitrary filesystem paths. The deferred local
storage slice needed a small persistence boundary that preserved the same
digest-first identity without becoming hosted retention infrastructure.

## What

`FileArtifactStore` stores bytes beneath a SHA-256-derived path and returns a
`governance.Artifact` containing both that locator and digest. Repeated `Put`
calls for the same bytes are idempotent. `Get` derives its lookup from the
digest, verifies the bytes again, and ignores a caller-supplied URI that could
point elsewhere. Publication uses Hammond's atomic write helper and a shared
artifact-store lock.

## Why

The losing alternative was to trust a caller-provided path as the storage key.
That makes replacement or accidental cross-reference possible even when the
record carries a digest. Separating locator from content identity keeps the
artifact reference auditable and makes tampering observable.

## Gotchas

- The store detects bytes changed outside its API; it does not prevent an
  account with filesystem write access from changing them.
- This is local content addressing, not hosted blob retention, replication,
  garbage collection, authorization, or remote retrieval.
- `Get` is intentionally digest-driven; the reference URI is provenance or a
  locator returned by `Put`, not permission to read an arbitrary path.
- A digest proves byte identity, not contract meaning, completeness, or
  approval.

## Used in

- `hammond/internal/store/artifacts.go`
- `hammond/internal/store/artifacts_test.go`
- `hammond/internal/store/store.go`

## Related

- [A contract reference is only integrity-checked when its bytes are loaded](hammond-contract-artifact-loading.md)
- [Hammond approval governs identified bytes, not behavior](hammond-governance-artifact-identity.md)
- [Atomic replacement does not serialize independent store processes](hammond-file-store-concurrency.md)
