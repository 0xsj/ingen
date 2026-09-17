# A solo Hammond workflow can stay local until trust crosses a boundary

A single operator on one trusted machine does not need an organization
membership provider to use Hammond. A local actor-to-role authority artifact,
the file-backed record store, and revision-aware mutations are sufficient for
the current workflow.

## What

The solo path uses an unsigned local authority artifact and keeps the policy,
authority, and record store in one private filesystem boundary. Every mutation
uses a freshly read record revision. Backups preserve the record store and its
digest-bound policy and authority artifacts together.

## Why

Adding GitHub or hosted identity before there are multiple operators would add
credential, endpoint, and role-mapping semantics without solving a current
problem. Signing becomes necessary when authority crosses machines, operators,
automation, or an untrusted recovery channel.

## Gotchas

- `solo` is an example stable local actor ID, not an external identity claim.
- An unsigned authority file must not be treated as independently authentic
  after it leaves the trusted filesystem boundary.
- A revision conflict requires rereading the record; it is not permission to
  merge event histories manually.
- Restoring a record without its referenced policy and authority artifacts
  breaks digest-bound loading.

## Used in

- `hammond/SOLO-OPERATIONS.md`
- `hammond/examples/solo/README.md`
- `hammond/internal/store/filesystem.go`
