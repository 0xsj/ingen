# DECIDED — Solo Hammond authority stays local and unsigned for now

The current Hammond deployment has one operator and one trusted filesystem
boundary. Its local authority artifact remains unsigned for this phase.

## Decision

Use the local authority and policy files together with private filesystem
permissions, backups, read-only preflight validation, and revision-aware
mutations. Do not add GitHub membership, hosted identity, or signing-key
bootstrap solely for the solo workflow.

The example actor remains `solo` until an operational local identifier is
chosen. That value is a local governance identity, not an external account
claim.

## Why

An issuer signature is valuable when the authority bytes cross a trust
boundary. On one private machine, adding a key-generation, root-delivery,
rotation, and recovery workflow would add operational complexity without a
second authority holder or verifier to benefit from it.

This does not make the unsigned artifact portable or independently authentic.
The boundary is intentionally local.

## Revisit triggers

Move to signed authority artifacts and root bootstrap when:

- the authority file is consumed on another machine;
- another operator can change or approve authority;
- automation consumes authority outside the local workspace;
- backups or restores cross an untrusted channel; or
- audit requirements need issuer attribution separate from filesystem access.

## Used in

- `hammond/SOLO-OPERATIONS.md`
- `hammond/roadmap.md`
- `hammond/status.md`
