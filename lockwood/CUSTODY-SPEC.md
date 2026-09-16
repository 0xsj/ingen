# Lockwood custody specification

Status: draft design specification.

This document defines the first Lockwood boundary: how verification artifacts
are identified, accepted, stored, related, and verified. It is intentionally
about custody and artifact history, not about deciding whether a verification
result is correct.

## 1. Responsibility

Lockwood preserves what happened to an artifact after a producer creates it.
It provides a durable handoff between producers such as Sorna and consumers
such as Nublar, Sentinel, or a future review system.

Lockwood may verify artifact integrity and preserve producer-owned reports,
but it must not reimplement:

- contract or policy semantics;
- oracle generation or mutation semantics;
- CI gate decisions;
- governance or approval policy;
- claims that a hash proves correctness, isolation, or independent attestation.

## 2. Terminology

### Artifact

An artifact is a finite sequence of bytes. Its identity is the digest of those
bytes, not its path, filename, run ID, or upload attempt.

The first implementation uses SHA-256. The serialized identity should retain
the algorithm explicitly:

```text
sha256:<64 lowercase hexadecimal characters>
```

### Artifact reference

An artifact reference points to stored bytes and carries enough metadata for a
consumer to validate what it retrieved:

```json
{
  "schema": "lockwood.artifact/v1",
  "digest": "sha256:...",
  "size_bytes": 1234,
  "media_type": "application/json",
  "logical_name": "sorna-ci-result.json"
}
```

`logical_name` is descriptive. It is never an identity or storage key.

### Custody record

A custody record describes one Lockwood intake of an artifact. It records the
source, producer, integrity checks, lineage, and handling metadata separately
from the artifact bytes.

The same bytes may have more than one custody record if they are accepted from
different workflows or at different times. The artifact digest remains the
same while custody history grows append-only.

The filesystem store may persist one canonical reference manifest per
descriptive metadata variant. These manifests support read-only inventory and
do not change the content identity of the referenced blob; callers must still
verify the blob before treating a reference as trustworthy.

### Lineage

Lineage describes relationships between artifacts, such as a CI result
referencing an evidence bundle or a campaign result being derived from a
campaign plan. Lineage points to artifact digests and does not copy or
reinterpret producer-owned semantics.

## 3. Custody record

The initial record shape is:

```json
{
  "schema": "lockwood.custody/v1",
  "custody_id": "lockwood-...",
  "status": "accepted",
  "artifact": {
    "schema": "lockwood.artifact/v1",
    "digest": "sha256:...",
    "size_bytes": 1234,
    "media_type": "application/json",
    "logical_name": "sorna-ci-result.json"
  },
  "received_at": "2026-09-15T12:00:00Z",
  "producer": {
    "tool": "sorna",
    "kind": "behavioral-verification",
    "version": "..."
  },
  "custodian": {
    "tool": "lockwood",
    "version": "..."
  },
  "source": {
    "run_id": "...",
    "path": "document-pipeline-ci-result.json"
  },
  "integrity": {
    "status": "verified",
    "method": "sha256",
    "verified_at": "2026-09-15T12:00:01Z"
  },
  "parents": [
    {
      "relation": "references",
      "digest": "sha256:..."
    }
  ],
  "handling": {
    "redaction": "none",
    "retention_class": "default"
  }
}
```

The following fields are normative for the first version:

- `schema` identifies the record format;
- `custody_id` identifies this custody event and is distinct from the artifact
  digest. It is a storage-safe identifier containing only letters, numbers,
  `.`, `_`, and `-`;
- `status` identifies whether this intake is accepted, quarantined, or
  rejected;
- `artifact.digest` identifies the exact stored bytes;
- `artifact.size_bytes` must match the stored bytes;
- `received_at` records when Lockwood accepted the artifact;
- `producer` identifies the producing tool without importing its package;
- `custodian` identifies the Lockwood service or process that recorded the
  intake;
- `integrity.status` records whether Lockwood verified the bytes and any
  producer-supplied integrity manifest;
- `integrity.method` is `sha256` in v1;
- `parents` records known artifact relationships by digest;
- `handling` records redaction and retention metadata without silently
  changing the payload.

Unknown producer-specific fields belong in a preserved producer report or
separate artifact. Lockwood should not add producer semantics to this record.

### Canonical local encoding

The filesystem backend writes custody records as compact JSON using the v1
field order, an explicit empty `parents` array when there are no parents, and
no trailing newline. Reads reject otherwise valid but non-canonical record
bytes. This gives local idempotency and stable storage bytes; it is not yet a
claim of compatibility with an external signing canonicalization standard.

The implementation can derive a `sha256:` digest from those canonical record
bytes. This is a stable representation identity for local use and detached
attestation; by itself it authenticates neither the record author nor the
producer's claims.

The detached attestation helper can sign and verify this canonical digest with
Ed25519 when the caller supplies the private or public key. It does not resolve
human identity or access permissions, and it does not turn a valid signature
into proof that a producer verdict is correct. A separate canonical trust
registry can make an explicit local key-status decision for verification.

The v1 record is a closed contract. Additive fields such as remote source
locators or a pending lineage status require a versioned contract or a
separate artifact; they must not be added as unknown v1 fields.

Custody v2 adds optional `source.uri` and `source.version` fields for remote
provenance. The URI must be absolute and credential-free. The version is an
opaque provider locator such as an object version; it never replaces the
artifact digest. V2 records preserve the remote claim but do not make Lockwood
responsible for fetching, authenticating, or attesting the remote object.

## 4. Artifact and record lifecycle

The initial lifecycle is:

```text
incoming bytes
    ↓
digest and size computed by Lockwood
    ↓
optional producer manifest verified
    ↓
immutable blob stored
    ↓
custody record appended
    ↓
artifact available for retrieval and re-verification
```

Intake preflights custody metadata that does not depend on the computed
digest, including schema, source, producer, lineage syntax, and handling
fields. The complete record is validated again after the artifact digest and
size are known.

Intake callers may set a positive `max-bytes` limit. Zero keeps the current
unlimited behavior for library compatibility; negative values are rejected.
The filesystem store enforces the limit while streaming bytes into its
temporary file and rejects an oversized input before publishing a blob. The
Sorna and CI-result adapters apply the same policy before custody publication;
Sorna also bounds the verified bundle inputs before building its deterministic
archive. A size-limit failure must not create a custody record or published
blob.

An artifact must not have custody status `accepted` before its bytes and
custody record are durably written and its integrity status is `verified`. If
intake fails, the caller receives an error and no partially written artifact is
treated as accepted custody.

If blob publication succeeds but custody-record publication fails, the intake
error retains the complete pending record and artifact reference. A recovery
retry must verify the already-published blob before appending that record; it
must not replace or republish the blob.

The first implementation may retain rejected or quarantined intake metadata,
but it must never expose an artifact with another custody status as accepted
evidence.

Reconciliation is read-only in v1. It reports valid blobs with no custody
record, custody records whose referenced blobs are missing or invalid, and
corrupt published blobs. It does not delete or reclassify anything.

## 5. Required invariants

### 5.1 Content identity

Lockwood computes the digest from the received bytes. A caller-supplied digest
is a claim to check, not an identity to trust.

### 5.2 Immutability

After a digest is accepted, its blob bytes cannot be replaced. A repeated
intake of identical bytes is idempotent. Different bytes necessarily receive a
different digest.

### 5.3 Atomicity

Blob and record writes must use a temporary location followed by an atomic
publish step where the backend supports one. The local filesystem backend also
syncs the published parent directory after the link so a successful write is
durable across the normal crash-recovery boundary. An interrupted write must
not look like a complete artifact.

### 5.4 Integrity before acceptance

For a Sorna evidence bundle, Lockwood must verify the bundle's recorded
checksums before recording the bundle as integrity-verified. For a generic
artifact, Lockwood can verify its own content digest but must not invent a
semantic validity claim.

`custody.status: accepted` requires `integrity.status: verified`. A failed or
not-checked integrity result may be retained only as quarantined or rejected
custody.

Accepted custody means that Lockwood durably stored and verified the artifact
bytes; it does not mean every lineage parent is resolved or that a producer's
semantic verdict is correct. A record may be accepted at intake with an
unresolved parent, but record-level `verify` must fail closed until the
lineage is complete.

### 5.5 Path independence

Paths, filenames, and local artifact roots are provenance fields only. They do
not identify an artifact and must not be used as stable lookup keys.

### 5.6 Append-only custody history

Existing custody records are not edited to correct history. A correction,
redaction, or new observation creates a new record linked to the prior record
or artifact.

### 5.7 Producer ownership

Sorna remains authoritative for Sorna evidence semantics. Nublar remains
authoritative for aggregate status semantics. Lockwood stores these reports,
their hashes, and their relationships without recomputing their verdicts.

### 5.8 Honest assurance

Integrity verification proves that retrieved bytes match their recorded digest.
It does not prove that the bytes were originally truthful, that a contract was
correct, or that an execution boundary was independently attested.

### 5.9 Bundle boundary

The first store accepts byte artifacts. A Sorna evidence directory is not a
single byte artifact until an adapter either:

- creates a deterministic archive whose bytes can be hashed; or
- stores its files individually and creates a bundle manifest artifact that
  references their digests.

The filesystem store must not silently hash a directory listing or depend on
filesystem traversal order. The Sorna adapter will own this representation
decision.

The first Sorna adapter uses a deterministic, uncompressed tar archive. It
normalizes archive metadata, sorts paths, verifies every checksum-listed file
before archiving, rejects symlinks and unchecksummed regular files, and stores
the resulting tar as one immutable artifact. The archive media type identifies
whether it contains a subject evidence bundle or an oracle evidence bundle.
This proves custody of the verified input bytes; it does not replace Sorna's
semantic evidence verification.

The CI-result adapter validates `ingen.ci-result/v1` with the shared core
contract, stores the original JSON bytes unchanged, and records the envelope's
`tool` and `kind` as producer metadata. It preserves passed, failed, and error
envelopes as artifacts; Lockwood does not reinterpret their verdicts or nested
producer reports.

The intake coordinator is the accepted-custody boundary. Callers should not
publish an `accepted` record directly without first storing and verifying its
referenced blob. If record publication fails after blob publication, the blob
is an unreferenced orphan and must remain verifiable until a future cleanup
policy handles it. Read-only `reconcile --orphan-grace <duration>` may
classify valid orphans whose blob modification time is at least that old as
`cleanup_candidates`; this is an age signal, not a deletion decision. The
`--as-of` option makes the classification time explicit for reproducible
reports. Published detached artifacts, such as attestation envelopes, are
protected from orphan classification when their media type is explicitly
recognized by the reconciliation caller; their reference metadata and blob
are still verified. Ordinary unreferenced artifacts remain candidates.

## 6. Initial custody operations

The first implementation should support these conceptual operations:

| Operation | Purpose |
| --- | --- |
| `put` | Compute identity, verify optional source integrity, and accept bytes. |
| `get` | Retrieve bytes by digest. |
| `inspect` | Read custody metadata and lineage without loading the payload. |
| `lineage-status` | Report reachable lineage resolution and diagnostic issues without changing custody status. |
| `append-event` | Append a validated immutable redaction, retention, or legal-hold handling event. |
| `list-events` | List handling events for a custody record in deterministic recorded-time order. |
| `inspect-attestation` | Read a known detached envelope by artifact digest without asserting signer trust. |
| `inspect-attestation-link` | Report the typed detached link to a custody-record representation without adding lineage. |
| `find-attestation` | Find persisted detached envelopes by target digest or key ID without asserting signer trust. |
| `find-trusted-attestation` | Find and fully verify detached envelopes through an explicit trust-registry snapshot. |
| `record-digest` | Compute the digest of a record's canonical representation. |
| `sign-attestation` | Verify a custody record, sign its canonical digest with an explicit local key, and publish a detached envelope. |
| `import-attestation` | Validate and publish a canonical detached envelope against an explicit custody record without asserting signer trust. |
| `verify-attestation` | Verify a published detached envelope with an explicit public key. |
| `verify-attestation-trusted` | Verify a published detached envelope through an explicit trust-registry snapshot. |
| `verify` | Recompute a blob digest, or verify a custody record's blob digest and declared size. |
| `find` | Locate artifacts by metadata such as run, producer, media type, or logical name. |
| `recover` | Re-verify a published blob and retry appending its pending custody record. |
| `reconcile` | Report orphan blobs, dangling records, and corrupt blobs without mutating storage; protect recognized detached artifacts and optionally classify aged cleanup candidates. |

These operations may initially be exposed through a CLI and a filesystem
backend. The artifact and custody-record contracts also have process-local
in-memory implementations for tests and short-lived workflows; they provide
no durability guarantees. A stable public library or remote service API
should wait until the record and lifecycle semantics have been exercised.

Shared integrity helpers perform streaming SHA-256 and optional size-limit
checks. Adapters remain responsible for interpreting producer-specific
manifests, such as Sorna checksum lists.

Record-level verification is separate from `inspect`: `inspect` reads custody
metadata, while `verify` must check the referenced blob exists, its SHA-256
matches `artifact.digest`, and its byte count matches `artifact.size_bytes`.

When verifying a record with parents, `verify` must traverse the complete
reachable lineage. Every reachable parent digest must have at least one
`accepted` custody record, and every accepted custody record for that digest
must reference a blob whose digest and size verify. Cycles are verification
failures. Multiple accepted custody records for the same artifact digest are
allowed, but they do not weaken these checks.

For the first filesystem catalog, `find` scans custody records in deterministic
ID order and returns only records that match exact metadata filters. A corrupt
or unknown record causes the scan to fail rather than being silently omitted.
The catalog does not interpret producer reports or derive new verification
verdicts.

## 7. Initial lineage relations

The first version needs only a small vocabulary:

- `references`: a report or manifest points to another artifact;
- `derived-from`: an artifact was generated from another artifact;
- `contains`: a bundle or archive contains another artifact;
- `verifies`: an integrity or verification artifact records a check over
  another artifact.

Unknown relations may be preserved as opaque strings, but Lockwood should not
assign them semantics until they are specified.

## 8. Redaction and retention

The first implementation preserves the record's initial `handling` metadata
and now provides an append-only handling-event stream. A handling event has
schema `lockwood.handling-event/v1`, an immutable `event_id`, a target
`custody_id`, an explicit `recorded_at` time, descriptive `actor` and
`reason` fields, and type-specific fields:

- `redaction` requires distinct `original_digest` and `resulting_digest`
  values;
- `retention-classified` requires a `retention_class`;
- `legal-hold-placed` and `legal-hold-released` require a `legal_hold_id`.

Events are stored at `events/<custody-id>/<event-id>.json`. Repeating the same
event is idempotent; reusing an event ID with different canonical contents is
rejected. Events do not edit custody records or artifact bytes. The `actor`
field is a descriptive caller claim and is not authenticated by this
contract. `append-event` and `list-events` are the initial CLI surfaces.

The implementation still does not perform deletion, redact payloads, enforce
retention, or enforce legal holds. Those workflows require a separate policy,
authorization, and race-safe storage decision.

When those workflows are added:

- redaction must produce a new artifact or an explicitly versioned derivative;
- the original digest must remain in custody history unless policy requires
  restricted access or legal deletion;
- the redaction rule, actor, time, and resulting digest must be recorded;
- retention and legal-hold decisions must be represented as append-only
  events, not silent metadata edits.

## 9. Explicit non-goals

This specification does not yet define:

- human signer identity, access authorization, or external attestation workflows;
- remote object-storage protocols;
- authentication or authorization policy;
- retention deletion and legal-hold enforcement;
- authenticated handling-event actors and authorization policy;
- a search index implementation;
- a web interface;
- a new verification or CI verdict model.

Those may be added after the local content-addressed custody workflow is
working and its invariants are tested.

## 10. Known follow-up work

- Define human signer identity and access authorization around the local
  trusted-key registry. Registry v1 defines exact key IDs, Ed25519 public keys,
  active/revoked status, validity windows, immediate revocation, and
  non-reused IDs as governance rules; it does not grant access.
- Define remote retrieval, authentication, and attestation semantics if
  Lockwood later becomes responsible for obtaining remote objects.
- Define orphan cleanup policy, including a grace period and race-safe
  coordination with recovery. The current implementation only reports orphans
  and optionally classifies age-based candidates; it never deletes them.
- The current read-only `lineage-status` projection keeps v1/v2 custody status
  independent of lineage resolution. A future workflow may persist a
  versioned lineage status only after defining its update and compatibility
  semantics; `verify` continues to fail closed on unresolved parents.
- The handling-event stream records descriptive decisions but does not
  authenticate actors or execute redaction, deletion, retention, or legal-hold
  enforcement. Those policies remain a future governance boundary.
