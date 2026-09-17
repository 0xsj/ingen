# Hammond root-bootstrap operations

Status: deployment design required

Hammond can validate a caller-delivered root bootstrap, but it cannot make the
delivery channel secure. This document defines the operational handoff that a
deployment must complete before using root-signed authority trust in a
production workflow.

## Hammond guarantees

Given caller-owned bootstrap pins, Hammond can:

- bind the initial root snapshot to its expected ID, version, and public keys;
- verify the root artifact's exact SHA-256 and predecessor/bootstrap signature;
- expose only active root keys;
- reject revoked root keys and invalid key material;
- require stable root identity and strictly increasing rotation versions; and
- verify a root-signed authority trust snapshot before exposing its active
  authority keys.

These checks establish artifact and transition integrity. They do not prove
that the bootstrap pins were delivered to the right deployment or approved by
the right people.

## Deployment-owned requirements

Before production use, record decisions for:

- bootstrap pin delivery channel and access control;
- root-key generation, custody, quorum, and approval ceremony;
- operator identity and audit evidence for bootstrap changes;
- root rotation lead time, overlap, and rollback policy;
- compromise detection and emergency revocation;
- recovery when the active root key is unavailable or compromised; and
- separation between deployment configuration, root artifacts, and provider
  credentials.

Do not place private keys, bearer tokens, or refresh credentials in Hammond
records, membership snapshots, trust snapshots, or checked-in examples.

## Recommended local workflow

1. Deliver the initial root ID, version, and public-key pins through an
   approved deployment channel.
2. Load the digest-bound root snapshot with
   `LoadAuthorityRootStoreWithBootstrap`.
3. Load a root-signed authority trust snapshot through the validated root
   store.
4. Use the trust store's active keys for signed authority artifacts or the
   explicitly selected provider issuer.
5. For rotation, verify the signed successor with `AuthorityRootStore.Rotate`,
   persist the accepted reference through the deployment's own state system,
   and update the bootstrap/recovery configuration only through the approved
   change process.
6. Retain the old root and trust artifacts for audit without leaving revoked
   keys active in new verifiers.

## Acceptance checklist

- [ ] Bootstrap pins are delivered out of band and are access-controlled.
- [ ] The initial root ID and version are explicitly pinned.
- [ ] An invalid bootstrap key, identity, version, digest, or signature fails
  closed.
- [ ] A replayed or same-version root snapshot is rejected.
- [ ] A successor with a different root ID is rejected.
- [ ] Revoked root keys cannot verify a later rotation.
- [ ] A root-signed trust snapshot exposes only active authority keys.
- [ ] Rotation and recovery produce auditable deployment records.
- [ ] Compromise recovery has been tested without weakening verification.

## Explicit non-goals

This document does not define a hosted key-management service, quorum
algorithm, certificate authority, provider credential protocol, TLS deployment,
or universal recovery procedure. Those require deployment-specific owners and
acceptance behavior.
