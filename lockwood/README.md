# Lockwood

Lockwood is InGen's future evidence-custody and artifact-registry surface.

It should preserve verification artifacts and their history so they can be
located, re-verified, shared, and inspected later. Its scope may include:

- immutable evidence bundles and content-addressed artifacts;
- contract, oracle, implementation, policy, and result references;
- provenance and artifact lineage;
- retention, redaction, and access controls;
- evidence sharing and independent re-verification.

Lockwood preserves what happened. Hammond governs what should be true, while
Nublar coordinates CI and delivery workflows. Lockwood should remain a storage
and custody boundary rather than reimplementing verification semantics.

## Status

Future vertical. It should grow when durable, cross-project evidence storage
and artifact sharing become a real workflow need.

The current planning proposal is recorded in
[`PROPOSED-TREE.md`](PROPOSED-TREE.md).

The first boundary draft is recorded in
[`CUSTODY-SPEC.md`](CUSTODY-SPEC.md).

The draft machine-readable contracts are in
[`spec/`](spec/).

Custody v2 can preserve credential-free remote source URI/version metadata;
this records provenance only and does not fetch or attest remote objects.

The initial local CLI exposes `put`, `import-sorna`, `get`, `inspect`, `verify`,
`find`, and read-only `reconcile` reporting for orphaned or damaged storage.
