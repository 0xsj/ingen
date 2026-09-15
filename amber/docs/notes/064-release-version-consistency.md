# Release metadata should have one checked version source

When Go and TypeScript are released together from a monorepo, package metadata
can drift even if the release decision is clear. A small repository-level
version source and an automated check make that drift visible before publishing.

## Origin

Amber's intended first release was confirmed as `0.1.0` with the monorepo tag
`amber/v0.1.0`, but only `typescript/package.json` and its lockfile carried the
version. The Go module has no source-level version declaration because its
release identity comes from the repository tag.

## What

The repository now includes:

- [`VERSION`](../../VERSION), containing `0.1.0`;
- `typescript/test/version.mjs`, which compares the repository version with
  `typescript/package.json` and the lockfile root package; and
- the version test in the TypeScript `npm test` command, which is included in
  `make check` and `make release-check`.

[`VERSIONING.md`](../../VERSIONING.md) records the synchronized release
identity and intended `amber/v0.1.0` tag. An unauthenticated `npm view
@0xsj/amber version` lookup returned `E404`, so public availability and scope
ownership remain unverified. A network-enabled `npm whoami` lookup also
returned `401 Unauthorized`, so this environment is not npm authenticated. No
tag or package publication was performed.

## Why

This creates one low-cost consistency gate for the published TypeScript
artifact while keeping Go's versioning in the repository release process. It
also keeps the package version, lockfile, and release documentation from
silently diverging.

## Example

```sh
make check
```

The TypeScript suite prints `Release version metadata passed: 0.1.0` when the
repository version source and package metadata agree.

## Gotchas

- The `VERSION` file does not create a Git tag or publish an npm package.
- The intended `amber/v0.1.0` tag still requires repository-level approval and
  release credentials.
- An npm registry `E404` without authentication is not conclusive evidence that
  a package name is available.
- A successful network request returning `401 Unauthorized` confirms that the
  current npm environment still needs authentication; it does not establish
  package or scope ownership.
- Provenance wire and storage schema versions remain separate contract values.
- A future release must update `VERSION`, `package.json`, and the lockfile
  together before changing the release notes.

## Used in

- [`VERSION`](../../VERSION)
- [`typescript/test/version.mjs`](../../typescript/test/version.mjs)
- [`typescript/package.json`](../../typescript/package.json)
- [`typescript/package-lock.json`](../../typescript/package-lock.json)
- [`VERSIONING.md`](../../VERSIONING.md)
- [`RELEASE.md`](../../RELEASE.md)

## Related

- [Release identity should be decided separately from wire and storage versions](063-versioning-decision.md)
- [Contributor and release runbooks should make the verified workflow repeatable](060-contributor-release-runbooks.md)
- [Published artifacts should carry explicit release metadata](056-release-metadata.md)
