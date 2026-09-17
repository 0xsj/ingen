# Sentinel's local receipt is sufficient for alpha, not host durability

## Decision

Keep the file-backed Sentinel receipt for the current local alpha. Do not add a
durable store abstraction before the Herdr host contract identifies the owner
of event identity, receipt updates, restart recovery, and retry acknowledgement.

The current boundary is sufficient for the proven local workflow because it
provides:

- a sibling advisory lock around each read-modify-publish update;
- temporary-file sync followed by atomic rename for complete receipt files;
- strict reload validation and exact snapshot bytes for audit provenance;
- idempotent retries that do not rewrite an unchanged receipt; and
- root-aware artifact verification before an accepted update is published.

## What this does not claim

The local receipt does not provide remote retention, distributed locking,
authenticated callback origin, durable directory metadata, or recovery policy
for a host that owns retries and shutdown. Those are host-contract decisions,
not reasons to widen Sentinel's alpha semantics.

The local file is restart-readable when it remains available, but that is not
the same as proving Herdr restart recovery. Native binding acceptance must still
exercise the real host's durable owner, retry behavior, cancellation, and
shutdown path.

The local reload regression
`TestUpdateFileReloadsPersistedReceiptAcrossWriterBoundaries` proves the
alpha-level case: one writer publishes, a later writer reloads that file, and
the later update preserves the earlier event. It intentionally does not claim
host restart or distributed recovery.

## Revisit conditions

Introduce a store abstraction only when a concrete consumer requires one of:

1. callbacks arriving from processes or machines that do not share the receipt
   filesystem;
2. host-owned retry and recovery that cannot be represented by the local file;
3. retention, replay, or authenticated event delivery beyond the project root;
4. a documented need for stronger directory or multi-object transaction
   guarantees.

Until then, a store abstraction would add an ownership and consistency surface
without resolving the missing Herdr contract.

## Evidence

- [`sentinel-receipt-update-lock.md`](sentinel-receipt-update-lock.md)
- [`sentinel-evidence-publication.md`](sentinel-evidence-publication.md)
- [`sentinel-herdr-host-binding-contract.md`](sentinel-herdr-host-binding-contract.md)
- [`herdr-sentinel/internal/run/update.go`](../../herdr-sentinel/internal/run/update.go)
- [`herdr-sentinel/internal/run/update_test.go`](../../herdr-sentinel/internal/run/update_test.go)
- [`herdr-sentinel/internal/run/run.go`](../../herdr-sentinel/internal/run/run.go)

The focused checkpoint remains green with:

```sh
GOCACHE=/private/tmp/ingen-sentinel-go-cache go test -race ./herdr-sentinel/... ./nublar/... ./core/...
```
