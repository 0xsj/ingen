# Lockwood

Lockwood is InGen's local evidence-custody and artifact-registry vertical.

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

Implemented local custody vertical. The current foundation preserves
content-addressed artifacts, custody records, append-only handling history,
lineage, redaction traceability, and detached verification evidence.

Future work remains intentionally separate: hosted or remote object storage,
cleanup and retention enforcement, provider-specific identity verification, and
authorization enforcement are not silently included in the local boundary.

The current planning proposal is recorded in
[`PROPOSED-TREE.md`](PROPOSED-TREE.md).

The implementation history and candidate next steps are recorded in
[`roadmap.md`](roadmap.md).

The first boundary draft is recorded in
[`CUSTODY-SPEC.md`](CUSTODY-SPEC.md).

The draft identity and authorization handoff boundary is recorded in
[`AUTHORIZATION-HANDOFF.md`](AUTHORIZATION-HANDOFF.md).

The handoff currently provides typed, read-only target construction, request
validation, caller-owned verification, and transient receipt binding. It does
not implement an identity provider, external policy engine, or live revocation
lookup.

The draft machine-readable contracts are in
[`spec/`](spec/).

A representative accepted custody record is in
[`examples/custody-record.json`](examples/custody-record.json).

Custody v2 can preserve credential-free remote source URI/version metadata;
this records provenance only and does not fetch or attest remote objects.

The provider-neutral remote/object-storage boundary is defined in
[`REMOTE-OBJECT-SPEC.md`](REMOTE-OBJECT-SPEC.md). It keeps the local artifact
digest authoritative and leaves credentials, provider authentication, retries,
and remote retrieval to a caller-owned adapter. The local seam validates
references and verifies fetched bytes, but does not publish remote custody.
Its reference schema is [`spec/lockwood.remote-object-v1.schema.json`](spec/lockwood.remote-object-v1.schema.json).

The cleanup and enforcement boundary is drafted in
[`CLEANUP-ENFORCEMENT-SPEC.md`](CLEANUP-ENFORCEMENT-SPEC.md). Current
reconciliation and legal-hold checks remain read-only; deletion and retention
enforcement require a separate policy, authorization, and coordination
contract.

The proposed cleanup policy and authorization handoff is recorded in
[`CLEANUP-AUTHORIZATION.md`](CLEANUP-AUTHORIZATION.md). It binds a future
decision to one local artifact digest, a canonical cleanup plan, policy and
legal-hold snapshots, and an explicit outcome. It is design-only: cleanup
entries remain `not-authorized` and no deletion worker exists. The typed
read-only target/request/result seam and versioned target, policy-reference,
hold-reference, request, and result schemas validate this binding; they do not
provide an external authority or perform deletion. The readiness projection
stops at `ready-for-revalidation` with `action_status: not-authorized` until a
future worker supplies fresh local checks and coordination.
Typed revalidation, lease/fencing, and worker-preflight evidence are validated
as caller/coordinator inputs; Lockwood still does not acquire coordination or
perform deletion. The lifecycle receipt shape is validated in memory and is
not persisted by Lockwood.

The cross-module evidence handoff is defined in
[`CROSS-MODULE-EVIDENCE-SPEC.md`](CROSS-MODULE-EVIDENCE-SPEC.md). It maps
Sorna, Hammond, and Nublar artifacts through exact local digests without
reinterpreting their producer-owned decisions. An offline deterministic
fixture path covers the existing adapters and generic custody handoff.

The initial local CLI exposes `put`, `import-sorna`, `import-ci-result`,
`import-attestation`, `import-redaction-provenance`, `get`, `inspect`, `lineage-status`, `append-event`,
`list-events`, `handling-status`, `redaction-status`, `check-handling-guard`, `register-redaction`, `promote-redaction`, `inspect-attestation`, `inspect-attestation-link`,
`inspect-redaction-provenance`, `inspect-redaction-provenance-link`,
`find-attestation`, `find-redaction-provenance`, `record-digest`, `sign-attestation`,
`sign-handling-event`, `sign-redaction-provenance`,
`verify-attestation`, `verify-attestation-trusted`, `find-trusted-attestation`,
`verify-handling-event`, `verify-handling-event-trusted`,
`verify-handling-event-authorized`,
`verify-redaction-provenance`, `verify-redaction-provenance-trusted`,
`find-trusted-redaction-provenance`,
`verify`, `verify-report`, `find`, `recover`, read-only `reconcile`, and
read-only `cleanup-plan` reporting for
orphaned or damaged storage. `recover` accepts a saved pending custody record,
re-verifies its existing blob, and retries record publication. Intake commands
accept `--max-bytes`; zero means unlimited and a positive value rejects
oversized input before custody publication. Use `--pending-record <path>` to
save a recoverable record when publication fails.

`reconcile --orphan-grace <duration>` can classify sufficiently old, still
valid orphan blobs as cleanup candidates. This report is read-only; Lockwood
does not delete candidates automatically. Use `--as-of <RFC3339>` for a
reproducible age cutoff. The CLI recognizes and protects valid published
detached attestation and redaction-provenance artifacts from orphan
classification; ordinary unreferenced blobs remain reportable.

`cleanup-plan --orphan-grace <duration>` projects that reconciliation into a
versioned report with explicit `reported-orphan` or `cleanup-candidate` state,
blockers, and `action_status: not-authorized`. It is a planning report only;
it does not evaluate external retention policy, authorize deletion, or mutate
storage. Use `--as-of <RFC3339>` for a reproducible plan.

`lineage-status --root <root> <custody-id>` provides a read-only projection of
reachable lineage. It reports unresolved parents, cycles, and damaged
reachable artifacts without changing the custody record's status; `verify`
continues to fail closed when lineage is incomplete.

`verify-report --root <root> [query flags]` verifies every matched custody
record, returns deterministic per-record outcomes, continues after individual
failures, and exits non-zero when any match fails. It is diagnostic only: a
failed verification does not change the custody record status. Its versioned
output contract is [`spec/lockwood.verification-report-v1.schema.json`](spec/lockwood.verification-report-v1.schema.json).

The append-only `handling-event/v1` contract records redaction observations,
retention classification, and legal-hold placement or release as separate
immutable events under a custody ID. `append-event` requires an existing
custody record and an explicit event time; `list-events` returns events in
recorded-time order. Events do not edit custody records, delete blobs, perform
redaction, enforce retention, or authenticate the descriptive `actor` field.
`handling-status` provides a read-only projection of the recorded retention
class, redaction history, and currently placed legal-hold IDs; it does not
claim that payload redaction or policy enforcement occurred.
`check-handling-guard --action redact|delete` reports whether a visible legal
hold blocks a payload-changing operation. A `not-blocked` result is not a
permission grant and does not evaluate retention or other authorization.
`register-redaction` records a caller-produced resulting artifact only after
both original and resulting digests verify and the legal-hold guard is clear;
it preserves the original artifact and custody record. The built-in memory and
filesystem stores serialize handling-event mutations per custody ID; on
flock-capable platforms, filesystem coordination is local advisory locking for
Lockwood processes that share a root, not distributed coordination.
`promote-redaction` creates a new accepted custody record for a registered
resulting artifact, with a `derived-from` parent for the source artifact. It
verifies the source and result again, preserves the source record and blob, and
is idempotent for identical record metadata.
`redaction-status` gives a read-only trace from the source custody record and
event through verified original/result artifacts and any explicitly promoted
result records. `complete` means the result is custody-anchored and its
lineage verifies; `result-unanchored` means the result is verified but has no
promoted custody record.

An optional detached `redaction-promotion` provenance envelope binds the source
custody record, redaction event, original/result digests, and promoted custody
record by their canonical representation digests. `sign-redaction-provenance`
publishes the envelope; the direct and trusted verification commands verify the
relationship with an explicit public key or trust-registry snapshot. This
authenticates the signed bytes and key status, but does not prove who operated
the redaction or that the payload transformation was correct.

`find-redaction-provenance` inventories persisted provenance envelopes by
source custody ID, event ID, promoted custody ID, or key ID without asserting
signature trust. `find-trusted-redaction-provenance` additionally resolves and
verifies the source record, event, promoted record, payload references, signed
relationship, and explicit trust-registry snapshot; any missing or damaged
relationship fails the whole read-only query.

Handling events can optionally be accompanied by a detached
`handling-event-attestation/v1` Ed25519 envelope. `sign-handling-event`
publishes one envelope for the exact canonical event; the two verification
commands support an explicit public key or the existing trust-registry
snapshot. A trusted key authenticates control of that key, not a human
identity or authorization to perform the described action.
`verify-handling-event-authorized` adds the explicit key/event-type allowlist
from a canonical policy snapshot and reports both snapshot digests.

The library also includes process-local in-memory artifact and custody-record
stores for tests and short-lived workflows; they provide no durability
guarantees.

The filesystem artifact store persists canonical reference manifests so
descriptive media-type and logical-name metadata can be inventoried after a
restart. Multiple metadata variants may refer to the same blob digest; they do
not change blob identity.

Shared streaming integrity helpers keep digest and size-limit behavior
consistent across storage and adapter boundaries.

Canonical custody records can also be assigned a representation digest; this
is not an external signature or attestation.

The detached attestation boundary is drafted in
[`SIGNATURE-SPEC.md`](SIGNATURE-SPEC.md). Its draft schema is in
[`spec/lockwood.attestation-v1.schema.json`](spec/lockwood.attestation-v1.schema.json);
the local helper can sign and verify Ed25519 attestations against a
caller-supplied key or exact `key_id` key set. A separate canonical trust
registry schema is available at
[`spec/lockwood.attestation-trust-v1.schema.json`](spec/lockwood.attestation-trust-v1.schema.json)
for explicit local trust decisions; it does not provide access control or
human identity.

Attestation envelopes have a strict canonical JSON encoding for reproducible
detached-artifact storage; their envelope digest is distinct from the custody
record digest they target.

The attestation helper can publish canonical envelope bytes through the shared
artifact store. It does not create custody records or automatically add
lineage, because the current lineage model addresses payload artifacts while
attestations target custody-record representations.

Published envelopes can be loaded by content digest and verified end to end
against a custody record using a caller-supplied key set or explicit trust
registry snapshot.

The `sign-attestation` command accepts a standard-base64 Ed25519 private-key
file containing 64-byte private-key bytes, with an optional final newline; the
file must be mode `0600` or stricter. It verifies the target custody record
before publishing only the detached envelope. Its JSON output includes the
custody ID as contextual receipt metadata, plus the envelope artifact
reference and envelope; publication also checks that the envelope targets that
exact record. The read-only
`verify-attestation` command accepts an explicit public-key file containing
standard-base64 Ed25519 public-key bytes, with an optional final newline. It
does not treat that key file as a Lockwood trust registry. The
`verify-attestation-trusted` command instead accepts a canonical registry file,
checks the key's active status and validity window at `--at` (or current UTC
time), and reports the registry snapshot digest in its transient receipt.

Successful `verify-attestation` output is a transient verification receipt
containing the custody ID and both digests, plus the key ID, algorithm, and a
`verified` flag. It is operational evidence, not a new custody record or
producer verdict.

The `import-attestation` command accepts a canonical envelope file, verifies
its target matches the supplied custody record, and publishes the detached
artifact. It performs no signature verification; use `verify-attestation` for
that step.

The `import-redaction-provenance` command accepts a canonical provenance
envelope file, checks its content digest and relationship against explicitly
named source, event, and promoted custody records, and publishes the detached
artifact. It performs no signature verification; use the direct or trusted
provenance verification commands for that step.

`inspect-redaction-provenance` loads a known provenance envelope by artifact
digest and reports its canonical metadata without verifying the signature.
`inspect-redaction-provenance-link` additionally checks the envelope against
explicit source, event, and promoted records, including their payloads and
promoted lineage, but still does not establish signer trust.

The read-only `inspect-attestation` command loads a known envelope by its
artifact digest and reports its canonical envelope metadata. It does not verify
the signature or establish signer trust.

The read-only `find-attestation` command filters persisted attestation
references by target custody-record digest or key ID. It verifies the matching
artifact bytes and canonical envelope but does not verify signatures.

The read-only `inspect-attestation-link` command reports the typed detached
relationship between an attestation artifact and its target custody-record
representation, alongside the payload artifact digest. It verifies the
canonical envelope binding and direct payload artifact, but does not assert
signature trust, lineage resolution, or a custody `verifies` edge.

The read-only `find-trusted-attestation` command applies the explicit trust
registry to that inventory. It resolves each target custody record, verifies
its payload and reachable lineage, verifies the detached signature, and
returns only trusted results with transient verification receipts.

The handling-event draft schema is in
[`spec/lockwood.handling-event-v1.schema.json`](spec/lockwood.handling-event-v1.schema.json),
with a representative fixture in
[`testdata/valid-handling-event-v1.json`](testdata/valid-handling-event-v1.json).
The detached event-signature schema is in
[`spec/lockwood.handling-event-attestation-v1.schema.json`](spec/lockwood.handling-event-attestation-v1.schema.json).
The action-policy schema is in
[`spec/lockwood.handling-event-policy-v1.schema.json`](spec/lockwood.handling-event-policy-v1.schema.json).
The detached redaction-provenance schema is in
[`spec/lockwood.redaction-provenance-attestation-v1.schema.json`](spec/lockwood.redaction-provenance-attestation-v1.schema.json),
with a representative fixture in
[`testdata/valid-redaction-provenance-attestation-v1.json`](testdata/valid-redaction-provenance-attestation-v1.json).
