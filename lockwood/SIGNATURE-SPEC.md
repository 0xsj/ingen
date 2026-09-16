# Lockwood detached attestation draft

Status: draft contract with a local Ed25519 helper. It does not add signer
trust, authentication, or authorization to Lockwood.

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
decode to the 64-byte Ed25519 signature size. `key_id` is an opaque, single-line
lookup identifier, not a public key and not proof that the signer is trusted.

## Canonical envelope encoding

The local envelope representation is compact JSON in the declared field order,
with no trailing newline. Decoding rejects unknown fields, multiple JSON
values, and otherwise valid JSON whose bytes are not exactly the canonical
encoding. The SHA-256 digest of these bytes identifies the detached envelope
artifact; it is distinct from `target.digest`, which identifies the custody
record.

## Verification boundary

The local helper verifies with a caller-supplied public key, or resolves the
envelope's exact `key_id` from a caller-supplied public-key set. The published
artifact helper also loads by content digest and requires canonical bytes
before performing that verification. A complete workflow should additionally:

1. Load and validate the target custody record.
2. Recompute its canonical digest.
3. Require the recomputed digest to equal `target.digest`.
4. Resolve `key_id` from a caller-supplied trusted key set.
5. Verify the Ed25519 signature over the signing payload above.

Lockwood must not fetch keys, decide whether a key is authorized, or infer
that a valid signature makes a producer verdict true. Key distribution,
rotation, revocation, and authorization belong to the surrounding governance
system.

The read-only CLI verification path accepts one explicit standard-base64
Ed25519 public-key file. It is an operator-supplied verification input, not a
persisted Lockwood trust registry.

The attestation itself can be stored as a separate immutable artifact. Do not
automatically add a `verifies` lineage edge yet: current custody lineage
digests identify stored payload artifacts, while `target.digest` identifies a
custody-record representation. A future linkage model must define that type
boundary explicitly before custody records are changed.

## Open decisions

- Define the trusted-key registry and revocation behavior.
- Decide whether signer identity belongs beside `key_id` or remains registry
  metadata.
- Define expiry and key-rotation handling without mutating old attestations.
- Define how a detached envelope artifact links to a custody-record digest
  without conflating record identity with payload-artifact identity.
- Decide whether a future attestation record needs its own custody status.
