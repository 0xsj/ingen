# The package artifact should pass a clean consumer smoke test

An artifact that looks correct in its source repository still needs to work
from the perspective of a fresh consumer.

## Origin

Amber's package check verified the TypeScript build and listed the files that
`npm pack` would publish, but it did not install the real tarball or resolve the
package through its public name.

## What

`make package-check` now builds Amber, runs the existing dry-run artifact check,
packs the actual tarball into a temporary directory, installs it into an
isolated temporary npm project, and runs a public-entry-point smoke test. The
smoke test imports the public ESM entry, creates provenance, performs a
transport encode/decode round trip, and checks the exported validation error
type. The temporary project and tarball are removed when the check exits.

## Why

This catches broken `exports` maps, missing compiled files, incorrect package
boundaries, and runtime import failures that a repository-local TypeScript
build can miss. It does not publish anything or require Amber to exist in a
registry.

## Example

```sh
make package-check
```

## Gotchas

- The smoke test is intentionally runtime-focused; TypeScript declaration
  checking remains part of `make check`.
- The package is ESM-only, so the temporary consumer imports it from an `.mjs`
  file.
- The final npm name and publishing ownership remain open decisions.

## Used in

- [`Makefile`](../../Makefile)
- [`typescript/test/package-smoke.mjs`](../../typescript/test/package-smoke.mjs)
- [`typescript/package.json`](../../typescript/package.json)
- [`docs/notes/021-package-artifact-contract.md`](021-package-artifact-contract.md)

## Related

- [The TypeScript package should publish one explicit, testable entry artifact](021-package-artifact-contract.md)
- [The root check gate should combine static analysis with cross-language tests](017-static-check-gate.md)
- [CI should enforce race safety continuously](025-ci-race-gate.md)
