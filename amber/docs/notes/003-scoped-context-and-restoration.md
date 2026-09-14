# Context propagation should be scoped by construction

An API that derives a child context from a parent makes exact restoration
possible without mutable process-global provenance state.

## Origin

The context slice implemented the v1 requirement that incoming context be
inspected, explicitly installed, and restored without silently changing an
unrelated context.

## What

Go uses the standard immutable `context.Context` chain: `WithProvenance` validates
the value and returns a derived context, while retaining the parent is the
restoration mechanism. TypeScript uses the platform-neutral
`ProvenanceContext`, whose `withProvenance` method returns another immutable
context value and leaves the parent unchanged.

Neither core implementation maintains an ambient global current value. A
transport or framework adapter can choose how to carry a context across its
boundary, but the core API requires the caller to make propagation explicit.

## Why

A mutable global or request-local singleton is easy to leak across concurrent
requests, asynchronous callbacks, or nested operations. Cleanup can also lose
the exact prior state when there was no value before installation. A derived
context preserves the parent by construction, so restoring it means continuing
to use that parent value.

## Example

```text
base -> with(root) -> with(child)
  ^         ^             ^
  |         |             |
empty    root value    child value
```

The `base` and root contexts remain available after the child context is
created; no cleanup mutation is required.

## Gotchas

- Decoding an incoming JSON value does not install it automatically; inspection
  and installation are separate operations.
- `WithProvenance` validates before installation, so invalid data cannot enter
  the context chain through this API.
- Explicit context values do not automatically cross a process or transport
  boundary; adapters must carry and restore them deliberately.

## Used in

- [`go/context.go`](../../go/context.go)
- [`go/context_test.go`](../../go/context_test.go)
- [`typescript/src/context.ts`](../../typescript/src/context.ts)
- [`typescript/src/provenance.test.ts`](../../typescript/src/provenance.test.ts)
- [`spec/v1.md`](../../spec/v1.md)

## Related

- [A provenance value is a history node, not a mutable request bag](002-immutable-transitions.md)
- [Conformance starts with shared invalid values](004-shared-conformance-fixtures.md)
