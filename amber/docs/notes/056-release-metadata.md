# Published artifacts should carry explicit release metadata

Release consumers should be able to identify the license, source repository,
and documentation from the artifact itself.

## Origin

Amber declared an MIT license in the TypeScript package metadata, but the
repository had no license files and the package README linked to a path that
only existed inside the source checkout. The package smoke test verified the
runtime entry point but not the legal and metadata files consumers receive.

## What

The repository now includes a root MIT `LICENSE`, and the TypeScript package
includes its own license file so npm consumers receive it. The package metadata
also declares its source repository, subdirectory, homepage, and issue tracker.
The README uses a repository URL for its specification link rather than a
workspace-relative path.

The package smoke test now verifies that an installed tarball contains:

- MIT license metadata;
- `LICENSE`; and
- `README.md`.

## Why

This makes the package artifact self-describing and avoids publishing a package
that works at runtime but loses essential release context. It does not change
the public Amber API or the core wire contract.

## Used in

- [`LICENSE`](../../LICENSE)
- [`typescript/LICENSE`](../../typescript/LICENSE)
- [`typescript/package.json`](../../typescript/package.json)
- [`typescript/README.md`](../../typescript/README.md)
- [`typescript/test/package-smoke.mjs`](../../typescript/test/package-smoke.mjs)

## Related

- [The package artifact should pass a clean consumer smoke test](026-package-install-smoke-test.md)
- [The public API should change only through an explicit compatibility check](032-public-api-compatibility.md)
- [Release readiness should have one repeatable project gate](033-release-candidate-gate.md)
