# Malcolm substitutes earlier setup captures in later request bodies

Capture interpolation is useful when a contract must carry a value from one
request into the data of a later request without turning the specification
into an expression language.

## Origin

Malcolm already supported setup captures in URL paths. Request bodies still
had no defined way to reuse those values, even though Sorna's runner had a
general placeholder resolver. The next narrow slice makes body substitution
explicit at both the Rust semantic boundary and the Sorna execution boundary.

## What

The supported form is a string placeholder:

~~~text
setup create_seed {
  given body {
    name = "seed copy.txt"
  }
  when POST "/documents"
  must response.status == 202
  capture document_name = response.body.name
}
given body {
  name = "{document_name}"
}
~~~

A setup request may reference captures from earlier setups. The target
request may reference captures from any setup in the same scenario. Malcolm
validates the reference name and ordering, then carries the ordinary string
through the IR. Sorna resolves it after setup capture and before JSON encoding.

## Why

The source remains typed: a placeholder is still a string body value, not an
embedded evaluator. Validation catches missing, malformed, and forward
references before execution. At runtime, the captured value is inserted as
raw body text; URL paths continue to use path escaping because they have
different transport semantics.

## Example

The document interpolation example creates a document named seed copy.txt,
captures that response name, and uses it as the name in a second POST body.
The subject only accepts .md and .txt names, so a literal unresolved
placeholder or a path-escaped seed%20copy.txt would fail the contract.

Run the end-to-end proof with:

~~~sh
make malcolm-sorna-interpolation-run
~~~

## Gotchas

- Only string captures can be substituted. Non-string captures fail during
  Sorna request construction.
- A setup cannot use a capture produced by itself or by a later setup.
- The target can use captures from setup declarations, but the target still
  fails at runtime if a setup did not establish its capture.
- Body substitution does not URL-escape captured text. URL-path substitution
  remains path-escaped.
- Placeholder substitution does not evaluate expressions, perform formatting,
  or coerce integers and booleans into strings.

## Used in

- [malcolm/src/semantic.rs](../../malcolm/src/semantic.rs)
- [sorna/internal/malcolm/adapter.go](../../sorna/internal/malcolm/adapter.go)
- [sorna/internal/runner/runner.go](../../sorna/internal/runner/runner.go)
- [malcolm/examples/document_interpolation.malcolm](../../malcolm/examples/document_interpolation.malcolm)
- Makefile target malcolm-sorna-interpolation-run

## Related

- [Malcolm carries typed request bodies and stateful setup across the boundary](malcolm-request-body-lowering.md)
- [Malcolm uses dotted object paths for nested response selectors](malcolm-response-selectors.md)
- [Malcolm reaches Sorna through a rejecting IR adapter](malcolm-sorna-bridge.md)
- [The contract can cross language boundaries](../concepts/the-contract-can-cross-language-boundaries.md)
- [Notes protocol](../../NOTES.md)
