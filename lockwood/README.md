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

A representative accepted custody record is in
[`examples/custody-record.json`](examples/custody-record.json).

Custody v2 can preserve credential-free remote source URI/version metadata;
this records provenance only and does not fetch or attest remote objects.

The initial local CLI exposes `put`, `import-sorna`, `import-ci-result`, `get`,
`inspect`, `record-digest`, `verify-attestation`, `verify`, `find`, `recover`, and read-only `reconcile` reporting for
orphaned or damaged storage. `recover` accepts a saved pending custody record,
re-verifies its existing blob, and retries record publication. Intake commands
accept `--max-bytes`; zero means unlimited and a positive value rejects
oversized input before custody publication. Use `--pending-record <path>` to
save a recoverable record when publication fails.

The library also includes process-local in-memory artifact and custody-record
stores for tests and short-lived workflows; they provide no durability
guarantees.

Shared streaming integrity helpers keep digest and size-limit behavior
consistent across storage and adapter boundaries.

Canonical custody records can also be assigned a representation digest; this
is not an external signature or attestation.

The detached attestation boundary is drafted in
[`SIGNATURE-SPEC.md`](SIGNATURE-SPEC.md). Its draft schema is in
[`spec/lockwood.attestation-v1.schema.json`](spec/lockwood.attestation-v1.schema.json);
the local helper can sign and verify Ed25519 attestations against a
caller-supplied key or exact `key_id` key set, but Lockwood does not provide
signer trust or authorization.

Attestation envelopes have a strict canonical JSON encoding for reproducible
detached-artifact storage; their envelope digest is distinct from the custody
record digest they target.

The attestation helper can publish canonical envelope bytes through the shared
artifact store. It does not create custody records or automatically add
lineage, because the current lineage model addresses payload artifacts while
attestations target custody-record representations.

Published envelopes can be loaded by content digest and verified end to end
against a custody record using a caller-supplied key set.

The read-only `verify-attestation` command accepts an explicit public-key file
containing standard base64 Ed25519 public-key bytes, with an optional final
newline. It does not treat that file as a Lockwood trust registry.
