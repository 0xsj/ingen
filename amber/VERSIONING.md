# Amber versioning

This document records Amber's release identity separately from the provenance
wire version and persistence schema versions.

## Current state

- The repository `VERSION` file and TypeScript package declare version `0.1.0`.
- The Go module path is `github.com/0xsj/ingen/amber` and does not encode a
  release version in source.
- The repository currently has no Git tags.
- Amber is maintained inside the `github.com/0xsj/ingen` monorepo.
- An unauthenticated `npm view @0xsj/amber version` lookup returned `E404`, so
  public package availability and scope ownership remain unverified.
- A network-enabled `npm whoami` lookup returned `401 Unauthorized`, so the
  current environment is not authenticated to npm.

## Approved release identity

The intended first release is:

- one semantic version, `0.1.0`, for the Go module and TypeScript package;
- the monorepo-qualified Go tag `amber/v0.1.0`; and
- the TypeScript artifact name `@0xsj/amber`, subject to registry ownership
  confirmation before publishing.

The version is tracked in [`VERSION`](VERSION) and checked against the npm
package and lockfile by the TypeScript test suite. The tag is the intended
release tag, but it has not been created; the repository owner must still apply
the convention through the repository's release process.

## Semantic versioning boundary

- Patch releases fix behavior without changing the documented public contract.
- Minor pre-1.0 releases may add compatible APIs, adapters, or documentation;
  compatibility review is still required for wire and storage behavior.
- Major releases may change public APIs or required contracts and must document
  the migration and compatibility impact.

Amber's provenance wire version and storage schema versions remain explicit
contract versions. They must not be inferred from the npm version or Git tag.

## Remaining release-owner decisions

Before a release commit or tag is prepared, confirm:

1. repository-level approval of `amber/v0.1.0`;
2. that Go and TypeScript are released together;
3. registry ownership of `@0xsj/amber`; and
4. repository and publishing credentials.

Use [`RELEASE.md`](RELEASE.md) for the evidence and stop-condition checklist.
