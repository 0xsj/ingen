# Rust enums make a small literal grammar explicit

An enum is a useful Rust boundary when a value may have a small, closed set of
forms.

## Origin

Malcolm request bodies initially need only strings, integers, and booleans.
Representing all of them as source text would force every later consumer to
parse the same values again.

## What

The AST uses:

~~~rust
enum Literal {
    String(String),
    Integer(i64),
    Boolean(bool),
}
~~~

The parser chooses one variant. The IR serializer then matches on every
variant to emit the corresponding JSON type.

## Why

The type records the fact that a body value is already understood. A string
cannot accidentally be emitted as an integer, and Rust's exhaustive `match`
requires a future literal variant to be considered by the serializer. The
enum also avoids a loosely typed `String` field with an undocumented
interpretation.

## Gotchas

- `String` in `Literal::String(String)` is the owned heap string type; the enum owns
  the request data after parsing.
- `i64` is deliberately narrower than arbitrary JSON numbers. Expanding the
  grammar should be a schema decision, not an incidental parser change.
- Matching on an enum is exhaustive, so adding arrays or nested objects will
  require updates to every consumer, including validation and serialization.

## Used in

- [`malcolm/src/lib.rs`](../../malcolm/src/lib.rs)
- [`malcolm/src/parser.rs`](../../malcolm/src/parser.rs)
- [`malcolm/src/ir.rs`](../../malcolm/src/ir.rs)
- [`malcolm/examples/document_flow.malcolm`](../../malcolm/examples/document_flow.malcolm)

## Related

- [Malcolm carries typed request bodies and stateful setup across the boundary](../modules/malcolm-request-body-lowering.md)
- [Rust ownership makes parser state explicit](rust-ownership-in-first-parser.md)
- [Notes protocol](../../NOTES.md)
