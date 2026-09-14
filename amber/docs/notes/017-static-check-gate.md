# The root check gate should combine static analysis with cross-language tests

Tests show runtime behavior for exercised paths; static checks catch a
different class of integration and maintenance errors.

## Origin

Amber's root verification command ran both SDK test suites and conformance
fixtures, and CI ran those tests, but neither gate explicitly ran Go vet or
TypeScript's no-emit typecheck.

## What

The root `Makefile` now provides `make lint` for `go vet ./...` and
`npm run typecheck`. `make check` combines `lint` and `test`. GitHub Actions
uses `make check` before running the composition examples.

## Why

Keeping static analysis in the same root gate makes it harder for a contributor
to validate only runtime behavior while missing an invalid API use or type
regression. Separate `lint` and `test` targets still make focused local work
fast and explicit.

## Example

```sh
make lint    # Go vet + TypeScript typecheck
make test    # runtime and conformance suites
make check   # both categories
```

## Gotchas

- TypeScript checks require the locked dependencies installed under
  `typescript/`.
- `go vet` follows the Go module under `go/`; it does not inspect unrelated
  repositories in the parent workspace.
- Static checks do not replace tests, conformance fixtures, or the runnable
  examples.

## Used in

- [`Makefile`](../../Makefile)
- [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml)
- [`README.md`](../../README.md)
- [`docs/notes/011-root-verification-command.md`](011-root-verification-command.md)

## Related

- [A project needs one verification command that crosses its language boundaries](011-root-verification-command.md)
- [CI should run the same cross-language checks and examples as local development](016-continuous-verification-workflow.md)
- [A runnable composition example should cross the adapters without external services](015-end-to-end-composition-example.md)
