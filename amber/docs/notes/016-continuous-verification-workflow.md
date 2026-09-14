# CI should run the same cross-language checks and examples as local development

Continuous verification is useful only when it exercises the commands a
contributor can reproduce locally.

## Origin

Amber had a root `make test` command and runnable composition examples, but a
future pull request could still omit one SDK or let the examples drift without
an automated repository check.

## What

The GitHub Actions workflow checks out the repository, installs the Go version
declared by `go/go.mod`, installs Node.js and the locked TypeScript
dependencies, then runs `make check`, `make race`, and `make examples`. It
triggers on pushes to `main` and on pull requests, with read-only repository
permissions.

## Why

The root commands are the project's verification contract. Reusing them in CI
keeps local and hosted verification aligned and ensures shared conformance
fixtures plus end-to-end examples remain executable as the adapters evolve.

## Example

```yaml
- name: Run complete test suite
  run: make test
- name: Run composition examples
  run: make examples
```

## Gotchas

- CI installs TypeScript dependencies with `npm ci`, so changes to
  `package.json` must be accompanied by a lockfile update.
- The workflow follows the Go version in `go/go.mod`; changing that directive
  changes the CI toolchain.
- CI validates the framework-neutral examples and Go race safety but does not provision a broker,
  database service, or production tracing backend.
- GitHub Actions availability and hosted-runner behavior remain external to
  Amber's code-level guarantees.

## Used in

- [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml)
- [`README.md`](../../README.md)
- [`Makefile`](../../Makefile)
- [`examples/README.md`](../../examples/README.md)

## Related

- [A project needs one verification command that crosses its language boundaries](011-root-verification-command.md)
- [A runnable composition example should cross the adapters without external services](015-end-to-end-composition-example.md)
