# Lockwood detached attestation and handling-event signature draft

Status: draft contract with a local Ed25519 helper and explicit local trust
registry. It authenticates control of a signing key when explicitly verified,
but does not establish human identity or access authorization.

## Purpose

An attestation can authenticate that a signer approved the exact canonical
representation of a custody record. It must not be added directly to the
closed v1 or v2 custody records, and it must not be interpreted as proof that
the producer's verification result is correct.

The attestation envelope is detached from the custody record:

```json
{
  "schema": "lockwood.attestation/v1",
  "target": {
    "kind": "custody-record",
    "digest": "sha256:..."
  },
  "algorithm": "ed25519",
  "key_id": "review-key-2026-01",
  "signature": "<standard-base64-ed25519-signature>"
}
```

`target.digest` is the digest returned by `custody.CanonicalDigest`. The
artifact digest inside the custody record remains the identity of the stored
payload; the target digest identifies the custody-record representation.

Handling-event signatures use a separate envelope and target kind so a record
attestation cannot be replayed as an action-history signature:

```json
{
  "schema": "lockwood.handling-event-attestation/v1",
  "target": {
    "kind": "handling-event",
    "custody_id": "lockwood-...",
    "event_id": "event-...",
    "digest": "sha256:..."
  },
  "algorithm": "ed25519",
  "key_id": "handling-key-2026-01",
  "signature": "<standard-base64-ed25519-signature>"
}
```

## Signing payload

The signer signs the exact UTF-8 bytes of this domain-separated message:

```text
lockwood.attestation/v1
custody-record
review-key-2026-01
sha256:<canonical-custody-record-digest>
```

Implementations construct it as four lines joined by `\n` and one final `\n`.
The fixed domain, target kind, and key ID prevent a signature from being
replayed as an attestation for another Lockwood object type or relabeled under
another key identifier.

The draft algorithm is Ed25519. The signature is standard base64 and must
decode to the 64-byte Ed25519 signature size; its text must be canonical
standard-base64 with no embedded whitespace. `key_id` is an opaque,
single-line lookup identifier, not a public key and not proof that the signer
is trusted.

## Canonical envelope encoding

The local envelope representation is compact JSON in the declared field order,
with no trailing newline. Decoding rejects unknown fields, multiple JSON
values, and otherwise valid JSON whose bytes are not exactly the canonical
encoding. The SHA-256 digest of these bytes identifies the detached envelope
artifact; it is distinct from `target.digest`, which identifies the custody
record.

## Verification boundary

The local helper verifies with a caller-supplied public key, or resolves the
envelope's exact `key_id` from a caller-supplied public-key set or trust
registry. The published artifact helper also loads by content digest and
requires canonical bytes before performing that verification. A complete
workflow should additionally:

1. Load and validate the target custody record.
2. Recompute its canonical digest.
3. Require the recomputed digest to equal `target.digest`.
4. Resolve `key_id` from a caller-supplied trusted key set or registry.
5. Verify the Ed25519 signature over the signing payload above.

Lockwood must not fetch keys, identify a human signer, enforce access
permissions, or infer that a valid signature makes a producer verdict true.
Key distribution and access authorization belong to the surrounding
governance system.

## Explicit local trust registry

The optional `lockwood.attestation-trust/v1` registry is a caller-supplied,
canonical JSON snapshot. Each entry contains an exact `key_id`, `ed25519`
algorithm, standard-base64 public key, `active` or `revoked` status, and
optional canonical UTC `not_before`/`not_after` timestamps. Key IDs must be
unique within a registry, and public-key text must be canonical
standard-base64 without embedded whitespace. Validity windows are inclusive at `not_before` and
exclusive at `not_after`.

Registry resolution is exact by `key_id`. A revoked key is rejected
immediately, including when evaluating an earlier timestamp. An expired or
not-yet-active key is rejected at the evaluation time. Rotation adds a new key
ID and retains the old ID as revoked; IDs must not be reused by governance.
Old envelopes are never mutated and remain available for direct
cryptographic verification even after a key is revoked or expires.

The registry makes a key-status decision for detached record-attestation,
handling-event-signature, and redaction-provenance verification. It is not a
signer directory, an access-control list, or proof of a producer claim. The
trusted verification CLIs require this registry explicitly and report its
canonical snapshot digest and evaluation time in transient receipts.

The read-only `verify-attestation` CLI path accepts one explicit
standard-base64 Ed25519 public-key file. It is an operator-supplied
verification input. The separate `verify-attestation-trusted` path accepts a
canonical trust registry file and does not persist or mutate that registry.

The local signing CLI accepts one standard-base64 Ed25519 private-key file
containing the 64-byte private key, with an optional final newline. The file
must be a regular file with mode `0600` or stricter. The command verifies the
target custody record before signing and publishes only the detached envelope;
it never stores or emits the private key and does not mutate the custody record.
Its publication receipt may include the custody ID as contextual metadata, but
the signed identity remains the target digest.

The publication helper checks the envelope target against the canonical digest
of the specific record supplied by the caller before storing bytes. This is a
binding check, not signature verification; callers must still verify with the
appropriate public key.

The `import-attestation` CLI accepts a canonical envelope file, checks its
content digest and target against an explicitly named custody record, and
publishes it without verifying the signature. This separates structural
intake from caller-controlled signer trust.

The `import-redaction-provenance` CLI accepts a canonical provenance envelope
file, checks its content digest and target against explicitly named source,
event, and promoted custody records, and publishes it without verifying the
signature. It also rechecks the promoted record's declared relationship to the
event. This separates structural provenance intake from caller-controlled
signer trust; use `verify-redaction-provenance` or its trusted variant for
cryptographic verification.

The read-only `inspect-redaction-provenance` command loads a known envelope by
artifact digest and reports canonical metadata without verifying the signature.
The `inspect-redaction-provenance-link` command additionally checks the target
against explicitly named source, event, and promoted records, verifies their
direct payload references, and verifies promoted lineage when the record store
is available. Neither command establishes signer trust; use the direct or
trusted verification commands for that decision.

The read-only `inspect-attestation` CLI loads a known envelope by artifact
digest and reports its canonical metadata. It does not verify the signature;
use `verify-attestation` when cryptographic verification is required.

The read-only `inspect-attestation-link` CLI verifies the envelope's target
against an explicitly named custody record and verifies that record's direct
payload artifact. Its typed output keeps the attestation artifact digest,
custody-record representation digest, and payload artifact digest separate.
It does not verify the signature or resolve the record's reachable lineage.

The read-only `find-attestation` CLI filters persisted references by target
custody-record digest or key ID, verifies the matching canonical artifacts, and
does not verify signatures. It is discovery, not a trust decision.

The separate read-only `find-trusted-attestation` CLI applies an explicit trust
registry to that inventory. It resolves each target digest to a custody record,
verifies the record's payload and reachable lineage, verifies the detached
signature, and returns only successful results. A missing target, damaged
record, unresolved lineage, invalid signature, revoked key, or invalid validity
window fails closed; no result is silently omitted.

Successful verification may return a transient receipt containing the target
custody ID, attestation artifact digest, target record digest, key ID,
algorithm, and `verified: true`. The receipt is operational evidence only; it
does not replace the detached envelope, create a custody record, or assert
semantic correctness.

The attestation itself can be stored as a separate immutable artifact. The
typed `attests-custody-record` inspection relation makes the boundary explicit
without adding a `verifies` lineage edge: current custody lineage digests
identify stored payload artifacts, while `target.digest` identifies a
custody-record representation. Custody records remain unchanged.

## Handling-event signatures

The handling-event signing payload is the exact UTF-8 bytes of this
domain-separated message, constructed with one final newline:

```text
lockwood.handling-event-attestation/v1
handling-event
handling-key-2026-01
lockwood-...
event-...
sha256:<canonical-handling-event-digest>
```

The envelope binds the event's custody ID, event ID, and canonical digest.
`sign-handling-event` loads an existing immutable event and publishes only
the detached envelope; it does not append, edit, or delete the event. The
read-only verification commands load the event by custody ID and event ID,
verify the detached artifact, and either use an explicit public key or resolve
the key through the trust registry.

Successful verification receipts are transient operational evidence. They
authenticate the signature under the selected key snapshot but do not prove
that the actor named in the event is a human, that the event was authorized,
or that any redaction, retention, or legal-hold effect was enforced.

## Redaction-promotion provenance

The redaction-promotion envelope is a separate detached signature. It binds a
registered redaction event to the accepted custody record that anchors its
resulting artifact without changing either custody record, the event stream, or
the payload bytes:

```json
{
  "schema": "lockwood.redaction-provenance-attestation/v1",
  "target": {
    "kind": "redaction-promotion",
    "source_custody_id": "lockwood-source",
    "source_record_digest": "sha256:<canonical-source-record>",
    "event_id": "redaction-2026-01",
    "event_digest": "sha256:<canonical-handling-event>",
    "original_digest": "sha256:<original-artifact>",
    "resulting_digest": "sha256:<resulting-artifact>",
    "promoted_custody_id": "lockwood-result",
    "promoted_record_digest": "sha256:<canonical-promoted-record>"
  },
  "algorithm": "ed25519",
  "key_id": "provenance-key-2026-01",
  "signature": "<standard-base64-ed25519-signature>"
}
```

The signer signs the exact UTF-8 bytes of this domain-separated message, with
one final newline:

```text
lockwood.redaction-provenance-attestation/v1
redaction-promotion
provenance-key-2026-01
lockwood-source
sha256:<canonical-source-record>
redaction-2026-01
sha256:<canonical-handling-event>
sha256:<original-artifact>
sha256:<resulting-artifact>
lockwood-result
sha256:<canonical-promoted-record>
```

The implementation requires accepted source and promoted records, a redaction
event belonging to the source record, matching original/result references, and
an explicit `derived-from` parent on the promoted record for the event's
original digest. Publication stores only the canonical detached envelope. The
direct and trusted verification commands also re-verify the referenced payload
artifacts and promoted record lineage. A valid signature authenticates control
of the selected key over this relationship; it does not authenticate the human
actor, grant action authorization, or prove the transformation's semantic
correctness.

For authorization handoffs, `CanonicalRedactionProvenanceTargetDigest` derives
a separate stable relationship digest from the canonical JSON encoding of the
schema and target only. It intentionally excludes the detached envelope's
`key_id` and `signature`, allowing an external assertion to bind to the exact
relationship without becoming coupled to one signer. This digest does not
authenticate an actor or grant authorization.

The read-only `find-redaction-provenance` command inventories persisted
provenance envelopes by source custody ID, event ID, promoted custody ID, or
key ID. It verifies the recognized artifact reference and canonical envelope
but does not verify the signature or resolve signer trust. The separate
`find-trusted-redaction-provenance` command resolves the source and promoted
custody records and the handling event named by each envelope, verifies the
payload references and promoted lineage, verifies the signature through the
explicit trust registry, and returns only successful results. A missing record
or event, damaged artifact, incomplete lineage, invalid signature, revoked key,
or invalid validity window fails closed.

## Open decisions

- Decide whether signer identity belongs beside `key_id` or remains registry
  metadata.
- Define access authorization around the local key-status decision.
- Decide whether a future attestation record needs its own custody status.
