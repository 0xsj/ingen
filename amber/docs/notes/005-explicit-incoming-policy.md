# Incoming handling must make the failure policy explicit

An adapter cannot safely infer whether malformed provenance should fail a
boundary or be ignored; the caller must choose that policy deliberately.

## Origin

The v1 specification left invalid incoming data to adapter policy, and the
first context implementation needed a concrete, testable behavior before
transport-specific adapters could be added.

## What

Amber exposes `reject` and `ignore` inspection modes. Empty input is absent;
non-empty malformed, unsupported, or oversized input is invalid. `reject`
returns an error, while `ignore` returns an absent result. Incoming JSON is
bounded at 16,384 UTF-8 bytes before decoding. Installing an accepted value is
separate from inspecting it, and absent or ignored input leaves the current
context unchanged.

## Why

Always rejecting untrusted metadata can turn an optional observability field
into a request outage. Always ignoring it can hide malformed producers and
silently lose useful causal context. Making the choice explicit keeps the core
safe while allowing an HTTP, message, or storage adapter to select behavior
appropriate to its boundary.

## Example

```text
no header              -> absent, existing context preserved
valid header + reject  -> accepted and explicitly installed
bad header + ignore    -> absent, existing context preserved
bad header + reject    -> error, no context installed
```

## Gotchas

- `ignore` means “treat as absent,” not “accept without validation.”
- Whitespace-only input is non-empty malformed input, not absence.
- The limit is measured in UTF-8 bytes, so a character count is not a safe
  substitute for the boundary check.
- Transport-specific header or envelope encodings are still adapter decisions;
  the core currently accepts JSON bytes or strings.

## Used in

- [`spec/v1.md`](../../spec/v1.md)
- [`go/incoming.go`](../../go/incoming.go)
- [`go/incoming_test.go`](../../go/incoming_test.go)
- [`typescript/src/incoming.ts`](../../typescript/src/incoming.ts)
- [`typescript/src/incoming.test.ts`](../../typescript/src/incoming.test.ts)
- [`conformance/v1.json`](../../conformance/v1.json)

## Related

- [Context propagation should be scoped by construction](003-scoped-context-and-restoration.md)
- [Conformance starts with shared invalid values](004-shared-conformance-fixtures.md)
