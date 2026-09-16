# Malcolm carries typed request bodies and stateful setup across the boundary

Executable request data needs a typed source shape before a cross-language
adapter can safely hand it to Sorna.

## Origin

Malcolm's original `given` clause stored free-form text. That preserved source
shape, but it could not become an HTTP request without guessing. Sorna already
supports JSON request bodies, setup requests, response assertions, and
captures, so the next slice makes those pieces explicit in Malcolm.

## What

Malcolm now supports a small executable subset:

~~~text
scenario read_document {
  state document_accepted
  setup accept_document {
    given body {
      name = "welcome.md"
    }
    when POST "/documents"
    must response.status == 202
    must response.body.id exists
    capture document_id = response.body.id
  }
  when GET "/documents/{document_id}"
  must response.status == 200
}
~~~

The Rust AST represents body fields as ordered names plus a `Literal` enum.
The IR preserves the body and setup structure. The Go adapter lowers the
scenario request to `given.body`, the setup sequence to `given.setup`, the
state label to `given.state`, and capture selectors to Sorna's
`body.FIELD` form. Setup requirements are merged into one executable setup
expectation.

## Why

The source contract must say which values are executable. A typed body block
can be serialized and executed without interpreting prose. A setup block also
keeps state creation, setup verification, capture, and target execution in
the Sorna runner, where those lifecycle and evidence semantics already live.

The adapter still rejects free-form `given` text and unsupported assertions.
This keeps a Malcolm source contract from silently becoming a weaker Sorna
contract.

## Example

~~~sh
make malcolm-sorna-flow-contract
~~~

The target compiles `malcolm/examples/document_flow.malcolm`, translates it,
and asks Sorna's validator to accept the resulting `ingen.contract/v1`
document.

## Gotchas

- The current body grammar supports only non-empty, top-level fields whose
  values are strings, signed 64-bit integers, or booleans.
- Nested objects, arrays, generated values, and capture interpolation in body
  values are not source features yet.
- A `state` label requires at least one setup; otherwise it would be metadata
  with no executable way to establish the state.
- Each setup needs a request, at least one positive lowerable requirement, and
  unique capture names. Target-rule must_not is now supported for the same
  lowerable expressions; negative setup requirements remain rejected.
- Sorna runs setup once per generated rule case. Repeating a setup for separate
  requirements is deliberate because each rule remains an independent oracle
  case.
- The generated contract is validated by the flow target; behavioral execution
  belongs to the separate flow-run target and its dedicated policies.

## Used in

- [`malcolm/src/lib.rs`](../../../malcolm/src/lib.rs)
- [`malcolm/src/parser.rs`](../../../malcolm/src/parser.rs)
- [`malcolm/src/semantic.rs`](../../../malcolm/src/semantic.rs)
- [`malcolm/src/ir.rs`](../../../malcolm/src/ir.rs)
- [`sorna/internal/malcolm/adapter.go`](../../../sorna/internal/malcolm/adapter.go)
- [`malcolm/examples/document_flow.malcolm`](../../../malcolm/examples/document_flow.malcolm)
- `Makefile` target `malcolm-sorna-flow-contract`

## Related

- [Malcolm reaches Sorna through a rejecting IR adapter](malcolm-sorna-bridge.md)
- [Malcolm's JSON IR is a transport boundary, not an evaluator](malcolm-json-ir.md)
- [Rust enums make a small literal grammar explicit](../language/rust-enums-for-typed-literals.md)
- [Notes protocol](../../../NOTES.md)
