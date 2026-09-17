# Lockwood identity and authorization handoff

Status: draft contract; typed read-only handoff, adapter-result, and cleanup
target/request/result validation are implemented.
External identity and authorization verification remain caller-owned and
unimplemented in Lockwood.

This document defines the proposed boundary for the next Lockwood slice. It
does not make Lockwood an identity provider, authorization service, Sentinel
lifecycle owner, or redaction executor.

## Purpose

Lockwood already verifies detached signatures under an explicit key registry
and can record descriptive handling actors. The missing boundary is how an
external identity or authorization decision is carried alongside that local
verification without confusing four different claims:

1. the exact Lockwood bytes or relationship being endorsed;
2. control of the signing key;
3. the identity relationship associated with the operation; and
4. authorization to perform or endorse the operation.

The next slice should make those claims independently visible in a transient
verification receipt.

## Ownership

| Concern | Owner | Lockwood responsibility |
| --- | --- | --- |
| Application provenance and attribution | Amber or the application owner | Preserve an explicit external reference; do not interpret it as authorization. |
| Human/service identity | External governance or identity owner | Consume a reference or assertion supplied by that owner; do not create an identity directory. |
| Key control | Lockwood trust-registry caller | Verify the detached signature and key status for the exact target bytes. |
| Action authorization | External governance owner, optionally with a Lockwood policy snapshot | Verify only the supplied assertion/policy boundary and report its digest and time. |
| Artifact custody and handling history | Lockwood | Preserve records, events, lineage, and detached evidence without silently changing them. |
| Workflow lifecycle and registration | Sentinel | Remain outside Lockwood unless an explicit handoff contract is approved. |

Amber's `attribution` fields describe who initiated, executed, or acted on
behalf of work. Amber explicitly does not perform authorization. An Amber
reference therefore remains descriptive context unless a separate governance
authority supplies an authorization assertion.

## Proposed handoff inputs

The caller supplies the following independent inputs when requesting an
authorization-aware verification:

- a Lockwood target: a custody record, handling event, redaction-promotion
  relationship, or the separately versioned cleanup `artifact-blob` target;
- the detached envelope and its content digest;
- the explicit trust-registry snapshot used to verify the signing key;
- an optional external identity/provenance reference, such as an Amber
  relationship reference;
- an optional externally owned authorization assertion or policy snapshot;
- an evaluation time for validity and revocation decisions.

The external identity and authorization inputs are references or separately
verifiable assertions. They are not copied into closed custody records by this
slice.

## Illustrative receipt shape

This is a design sketch, not yet a versioned machine-readable contract:

```json
{
  "target": {
    "kind": "handling-event",
    "digest": "sha256:<canonical-target>"
  },
  "envelope": {
    "digest": "sha256:<detached-envelope>",
    "key_id": "handling-key-2026-01",
    "signature_verified": true
  },
  "identity_reference": {
    "system": "amber",
    "type": "provenance",
    "id": "<external-reference>"
  },
  "authorization": {
    "authority": "<governance-owner>",
    "assertion_digest": "sha256:<external-assertion>",
    "policy_digest": "sha256:<policy-snapshot>",
    "evaluated_at": "2026-09-17T12:00:00Z",
    "decision": "authorized"
  },
  "key_verified": true,
  "identity_reference_recorded": true,
  "authorization_verified": true
}
```

The receipt must not imply that `identity_reference_recorded` authenticates a
person. `authorization_verified` is true only when the governing authority's
assertion or explicitly supported policy snapshot is actually verified. A
missing, malformed, expired, revoked, mismatched, or untrusted assertion must
fail closed or produce an explicitly non-authorized result; it must never be
silently treated as authorization.

## Target binding

The handoff must bind authorization to the exact operation being evaluated:

- a handling-event target binds custody ID, event ID, event digest, and event
  type;
- a redaction-promotion target binds source record, event, original/result
  digests, and promoted record;
- a custody-record target binds the canonical custody-record digest.

Lockwood now defines that relationship identity with
`CanonicalRedactionProvenanceTargetDigest`. It is the SHA-256 digest of the
canonical JSON document containing the provenance schema and target, equivalent
to:

```json
{"schema":"lockwood.redaction-provenance-attestation/v1","target":{...}}
```

The digest excludes the signing key, signature, and detached-envelope metadata,
so it remains stable when the same relationship is endorsed by another signer
or envelope. It is a target-binding primitive for a future authorization
receipt, not an authorization decision and not an identity assertion.

The typed target constructors now cover the current target kinds:

- custody-record targets use the canonical custody-record digest;
- handling-event targets use the SHA-256 digest of canonical JSON under
  `lockwood.authorization-target/v1`, binding target kind, custody ID, event
  ID, event type, and canonical event digest;
- redaction-promotion targets use the existing canonical relationship digest,
  which binds source record, event, original/result artifacts, and promoted
  record.
- cleanup `artifact-blob` targets bind the exact artifact digest, canonical
  cleanup-plan digest, plan `as_of`, and the `cleanup-delete` action under
  `lockwood.cleanup-authorization-target/v1`.

Each constructor validates the supplied local object before deriving the
target. Callers do not need to hand-assemble a target digest from labels.

`NewAuthorizationHandoff` builds the versioned envelope from one of those
targets and derives the action from the target kind, including the explicit
cleanup mapping from `artifact-blob` to `cleanup-delete`. It validates the
principal and authorization evidence references before returning the value,
preventing an action/target mismatch from entering the verification path.

## Proposed external authorization assertion shape

The next implementation should accept a caller-provided handoff envelope with
the versioned shape `lockwood.authorization-handoff/v1`. This is a transport
contract for passing independently verifiable external evidence into Lockwood;
it is not an assertion issued by Lockwood and does not replace the governing
authority's signed assertion.

The minimum shape is:

```json
{
  "schema": "lockwood.authorization-handoff/v1",
  "target": {
    "kind": "redaction-promotion",
    "digest": "sha256:<canonical-target>"
  },
  "action": "redaction-promotion",
  "principal": {
    "authority": "identity.example",
    "reference": "opaque-principal-reference",
    "assertion_digest": "sha256:<canonical-identity-assertion>"
  },
  "authorization": {
    "authority": "governance.example",
    "decision": "authorized",
    "assertion_digest": "sha256:<canonical-authorization-assertion>",
    "policy_digest": "sha256:<canonical-policy-snapshot>",
    "issued_at": "2026-09-17T12:00:00Z",
    "not_before": "2026-09-17T12:00:00Z",
    "expires_at": "2026-09-17T13:00:00Z",
    "revocation_snapshot_digest": "sha256:<canonical-revocation-snapshot>"
  }
}
```

The contract has these rules:

- `target.digest` must equal the locally derived digest for the exact target;
  the `kind` and `action` must be supported exact values, with no wildcard
  matching.
- `principal.reference` is opaque context. It becomes authenticated only when
  an external identity owner verifies the referenced identity assertion; a
  matching string alone proves nothing about a person or service.
- `authorization.assertion_digest` identifies the canonical external bytes
  that the governing authority verified. A policy-derived decision must also
  carry `policy_digest`, which identifies the canonical policy snapshot used.
- `decision` must be either `authorized` or `denied`. Only an externally
  verified `authorized` assertion can produce an authorized receipt.
- `issued_at`, `not_before`, and `expires_at` use canonical UTC RFC3339. The
  validity interval is inclusive at `not_before` and exclusive at `expires_at`;
  an expired, not-yet-active, or malformed assertion fails closed.
- `revocation_snapshot_digest` identifies the revocation state used by the
  external authority. Revoked or unknown state, a missing snapshot when
  revocation is required, or a stale snapshot produces a non-authorized result.
- `evaluated_at` is supplied by the caller as verification context and belongs
  in the transient receipt, not in the reusable assertion identity.

The external authority owns the assertion signature, identity semantics, and
revocation source. Lockwood may verify an adapter-supplied result and record
the assertion, policy, revocation, and target digests, but it must not infer
authorization from an opaque reference or fetch an authority's live directory.

## Verification adapter boundary

The external verification adapter is caller-owned and must be supplied as an
explicit dependency or as already-materialized evidence. It may validate the
authority's signature, resolve the principal relationship, validate the policy
snapshot, and check revocation at the requested evaluation time. It must not
mutate Lockwood state.

The adapter input is the canonical handoff, the exact target digest, the
detached-envelope verification result, and the caller-supplied `evaluated_at`.
Its output is a structured result, not a free-form boolean:

```text
identity_reference       opaque reference plus external assertion digest
identity_verified        true only after the identity owner verifies it
authorization_decision   authorized | denied | indeterminate
authorization_verified  true only for a verified authorized decision
assertion_digest         digest of the verified external assertion bytes
policy_digest            digest of the verified policy snapshot, if used
revocation_digest        digest of the checked revocation snapshot
evaluated_at             canonical UTC evaluation time
failure_code             stable reason when the result is not authorized
```

Lockwood must independently enforce these adapter-result invariants:

1. The returned target and action match the locally verified target and the
   requested operation exactly.
2. Every reported digest is computed from the canonical bytes actually
   verified, rather than copied from an unverified label.
3. `authorization_verified` can be true only when the decision is
   `authorized`, the validity window contains `evaluated_at`, the policy (when
   required) matches its supplied snapshot, and revocation is known-good.
4. Missing, malformed, mismatched, expired, revoked, stale, or indeterminate
   evidence returns a non-authorized result with a stable failure code.
5. The adapter cannot turn `identity_verified` into authorization; identity,
   key control, and action authority remain separate receipt fields.

No default network client, directory lookup, key discovery, or live policy
fetch belongs in Lockwood. A future integration may provide those capabilities
outside this package and pass the resulting canonical evidence through this
boundary.

The typed `AuthorizationVerificationResult.ValidateAgainst` helper now enforces
the local side of this boundary. It requires the adapter result to match the
locally verified target digest, action, principal reference, external evidence
digests, and canonical evaluation time. It also requires a stable failure code
for every non-authorized result and rejects `identity_verified` when no
identity assertion digest was supplied. Successful validation confirms only
that the result is well-bound; it does not perform the external verification.

`VerifyAuthorizationHandoff` is the read-only orchestration entry point. It
validates the request before invoking the caller-owned verifier, rejects a
result that is not bound to the local target, and returns a transient receipt
with the canonical handoff digest. A valid denied or indeterminate result is a
receipt with an explicit failure code; an unavailable or malformed verifier is
returned as an error and never treated as authorization.

## Authority and integration decision

For v1, Lockwood remains external-authority-neutral. It does not select or
embed an identity provider, governance service, or policy engine. The
`authority` fields are opaque governance identifiers supplied by the caller;
their meaning and trust roots belong to the named external owner.

The integration point is a caller-owned verifier with semantics equivalent to:

```text
VerifyAuthorization(request) -> result | verification-error
```

The request contains the canonical handoff, the locally verified target and
action, the external assertion/policy/revocation bytes identified by the
handoff digests, and the explicit `evaluated_at`. The verifier owns external
signature validation, principal matching, policy evaluation, and revocation
checking. The result uses the structured fields above. A valid but denied,
expired, revoked, stale, or mismatched assertion is a non-authorized result;
an error is reserved for malformed input or an unavailable verifier, and must
never be interpreted as authorization.

Trust is intentionally split:

- Lockwood's local trust registry verifies the detached custody, event, or
  provenance envelope key.
- The external verifier validates the authorization and identity evidence
  under the external authority's trust roots.
- Reusing a key or registry across both domains requires an explicit,
  versioned cross-reference; matching `key_id` text is not sufficient.

This keeps the default integration deterministic and offline once evidence is
materialized, while allowing a future application-owned adapter to perform
network-backed verification outside Lockwood.

## Existing policy relationship

`lockwood.handling-event-policy/v1` currently maps a trusted `key_id` to
handling-event types. It is an action allowlist, not an identity directory.
The next slice must decide whether:

- that policy grows a narrowly defined provenance target scope; or
- provenance endorsement uses a separate policy/assertion contract; or
- an external governance assertion remains authoritative while the local
  policy is only an additional fail-closed guard.

No wildcard rule or implicit key-to-human mapping should be introduced.

## Verification sequence

The intended read-only sequence is:

1. Load and validate the target and detached envelope.
2. Verify the target's referenced artifacts, records, event, and lineage as
   appropriate.
3. Resolve the envelope key through the explicit trust registry and verify the
   signature.
4. Validate the external identity reference shape without claiming identity
   authentication unless its owner supplies a verifiable assertion.
5. Verify the authorization assertion or policy snapshot at the explicit
   evaluation time.
6. Return a transient receipt with separate key, identity-reference,
   authorization, and custody/provenance results.

The sequence is read-only. It does not append an event, mutate custody,
register a Sentinel artifact, redact bytes, delete data, or install external
identity into process state.

## Required fixtures for implementation

The implementation slice should include fixtures for:

- a valid handling-event handoff;
- a valid redaction-promotion handoff;
- an Amber identity reference that is recorded but not treated as proof;
- an unknown or revoked signing key;
- an expired or not-yet-active policy/assertion;
- a target or principal mismatch;
- a stale policy snapshot digest;
- a missing authorization assertion that fails closed.

The adapter contract adds these expected outcomes to the fixture matrix:

| Fixture | Expected result |
| --- | --- |
| valid handling-event authorization | key, identity, target, policy, and revocation checks pass; `authorized` |
| valid redaction-promotion authorization | target uses the canonical relationship digest; `authorized` |
| Amber reference only | reference recorded; `identity_verified: false`; never authorized by reference alone |
| target or action mismatch | `indeterminate` or denied; stable target-mismatch failure |
| principal mismatch | non-authorized; identity relationship is not silently substituted |
| revoked or unknown external state | non-authorized; revocation failure is explicit |
| expired or not-yet-active assertion | non-authorized; validity-window failure |
| stale or mismatched policy snapshot | non-authorized; policy-digest failure |
| missing, malformed, or noncanonical assertion | non-authorized; assertion failure |

## Open decisions

- Which governance owner issues authorization assertions?
- Does that authority use the existing Lockwood trust registry or a separate
  registry with an explicit cross-reference?
- Is an Amber relationship referenced by opaque ID, canonical digest, or both?
- Which future target kinds need canonical target constructors beyond custody
  records, handling events, redaction promotions, and the cleanup artifact
  target (for example, custody tombstones or remote objects)?
- Which concrete external authority, if any, will supply the first production
  adapter?
- Should authorization receipts remain transient, or later become detached
  custody artifacts?
- Which existing policy, if any, is an additional local guard rather than the
  source of identity or authority?

Until these decisions are settled, Lockwood should continue to report trusted
key verification and descriptive actor/provenance references separately, and
should not claim an authorized human action.
