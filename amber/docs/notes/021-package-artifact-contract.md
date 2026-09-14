# The TypeScript package should publish one explicit, testable entry artifact

Package metadata is part of the public API: consumers need a stable import
target and maintainers need to know what will be published.

## Origin

The TypeScript SDK had build and test scripts, but its package metadata only
declared legacy `main` and `types` fields. There was no repository check showing
that the package artifact contained the intended compiled entry point.

## What

`typescript/package.json` now declares an ESM `exports` entry with explicit
runtime and type targets, marks the package as side-effect free, and publishes
only an explicit whitelist of library files from `dist`, excluding compiled
tests and examples. `typescript/README.md` provides the package quickstart. The root
`package-check` target builds the SDK, checks `npm pack --dry-run`, then installs
the actual tarball in a clean temporary project and exercises its public ESM
entry point. `make check` includes that target. `CHANGELOG.md` records the
current unreleased surface.

## Why

An explicit exports map prevents consumers from depending on internal source
paths, while the dry-run package check catches missing build output or an
unexpected package boundary before publishing. Keeping this contract in the
repository makes release preparation repeatable without requiring an npm
publish or external registry access.

## Example

```sh
make package-check
```

The package should expose the compiled `dist/index.js` runtime and
`dist/index.d.ts` type entry, with source tests excluded from the artifact. A
fresh consumer should be able to install the tarball and import the package by
name.

## Gotchas

- `package-check` requires TypeScript dependencies under `typescript/`.
- The package is ESM-only through its explicit `exports` entry.
- The protocol version in the Amber specification is separate from the npm
  package version; changing one does not automatically change the other.
- The package is not published by this workflow; publishing ownership and the
  final npm name remain open project decisions.
- The smoke test uses a temporary local project and does not contact the npm
  registry for Amber itself.

## Used in

- [`typescript/package.json`](../../typescript/package.json)
- [`typescript/README.md`](../../typescript/README.md)
- [`Makefile`](../../Makefile)
- [`CHANGELOG.md`](../../CHANGELOG.md)
- [`README.md`](../../README.md)

## Related

- [The root check gate should combine static analysis with cross-language tests](017-static-check-gate.md)
- [CI should run the same cross-language checks and examples as local development](016-continuous-verification-workflow.md)
- [The package artifact should pass a clean consumer smoke test](026-package-install-smoke-test.md)
- [The shared contract must precede both SDKs](001-spec-first-foundation.md)
