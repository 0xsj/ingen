# Malcolm uses dotted object paths for nested response selectors

Nested response data needs a selector grammar that is small enough to validate
at the language boundary and explicit enough to preserve missing-value
semantics at runtime.

## Origin

Malcolm initially accepted only response.body.FIELD assertions and captures.
Sorna's runner already traversed dotted object paths, but the parser and
adapter rejected those paths and the response-shape evaluator could not yet
evaluate nested expectations.

## What

The supported selector form is a dotted object path:

~~~text
setup inspect_document {
  when GET "/documents"
  must response.body.metadata.owner.id exists
  capture owner_id = response.body.metadata.owner.id
}
~~~

Every segment after response.body must be an identifier. The Rust parser
normalizes capture selectors to body.FIELD[.FIELD...]. The Go adapter lowers
assertions into nested Sorna properties and the runner walks the same path
through JSON objects.

## Why

The dotted form adds useful structure without introducing a general query
language. An exists assertion checks key presence at the final object level,
so an explicit JSON null is present and passes. A missing key fails, as does a
non-object intermediate value. This distinction is retained for captures:
selecting null succeeds as a selection, but the resulting non-string value
cannot be used by Malcolm's string body interpolation.

## Example

The adapter lowers response.body.metadata.owner.id exists into a nested
expectation whose intermediate values must be objects and whose final id key
must be present. Multiple nested setup requirements merge recursively rather
than overwriting sibling expectations.

The runner tests exercise the same shape against a fake HTTP subject, covering
nested equality, nested capture selection, explicit null, and missing fields.

## Gotchas

- Array indexes are not supported. Selectors cannot contain brackets or
  numeric path segments.
- Dotted paths traverse JSON objects only; a scalar or array in the middle of
  a path is a failure.
- exists means key presence, not non-nullness.
- Equality remains limited to Malcolm's existing scalar literal forms.
- A nested capture keeps its JSON type. Only string captures are eligible for
  body interpolation.

## Used in

- [malcolm/src/parser.rs](../../malcolm/src/parser.rs)
- [malcolm/src/semantic.rs](../../malcolm/src/semantic.rs)
- [sorna/internal/malcolm/adapter.go](../../sorna/internal/malcolm/adapter.go)
- [sorna/internal/runner/runner.go](../../sorna/internal/runner/runner.go)
- [sorna/internal/runner/runner_test.go](../../sorna/internal/runner/runner_test.go)

## Related

- [Malcolm carries typed request bodies and stateful setup across the boundary](malcolm-request-body-lowering.md)
- [Malcolm substitutes earlier setup captures in later request bodies](malcolm-capture-interpolation.md)
- [Malcolm reaches Sorna through a rejecting IR adapter](malcolm-sorna-bridge.md)
- [Notes protocol](../../NOTES.md)
