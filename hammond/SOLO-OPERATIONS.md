# Hammond solo operations

Status: active single-operator workflow

This is the recommended operating boundary while Hammond is used by one
operator on one trusted machine. It uses local policy and authority artifacts;
it does not require GitHub, an organization, a hosted Hammond service, or a
membership provider.

## Local authority

The solo example uses the actor ID `solo` and grants it the
`product-reviewer` role. Treat that as a stable local identifier, not as proof
of an external account. If a different identifier is chosen, update the
authority artifact and approval events together, then recalculate every
dependent policy and record digest.

The unsigned authority fixture is appropriate only while the authority file,
policy file, and record store remain inside one trusted filesystem boundary.
Do not copy an unsigned authority artifact between machines or operators and
treat it as independently authenticated.

## Workspace handling

Keep the record store and local authority artifacts in a private workspace.
Use a directory readable only by the operator when the records or authority
mapping are private. Back up the store, policy, and authority artifacts
together; restoring only the record store can leave its digest-bound policy
references unavailable.

The repository-local default store for this workspace is the gitignored
`.hammond/records/` directory. Keep that directory local to the trusted
machine; do not commit or copy it as an authority boundary.

Hammond stores contract and governance references, not a hosted backup. The
backup and recovery channel remain operator responsibilities.

## Safe mutation sequence

For every mutation:

1. Read the current record.
2. Obtain its current `revision` value.
3. Append the next event with `--if-revision`.
4. Re-read the record and obtain a fresh revision before the next event.

A revision conflict means the record changed since it was read. Re-read and
decide from the new state; do not reuse the stale revision or manually merge
event histories.

The runnable sequence is in [examples/solo/README.md](examples/solo/README.md).

## When to add signing

Add an Ed25519 authority signature and caller-owned trust/root bootstrap when
any of these become true:

- the authority artifact crosses a machine boundary;
- more than one operator can change authority;
- automated jobs consume the authority;
- the record store is restored through an untrusted channel; or
- audit requirements need issuer attribution independent of filesystem access.

Until then, filesystem permissions, backups, and explicit revision checks are
the active solo controls.

## Deferred

- GitHub organization/team or repository-collaborator membership;
- hosted Hammond persistence or artifact retention;
- automatic approvals from Sorna; and
- multi-operator authority rotation.
