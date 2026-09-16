# Rust's explicit string building makes a small JSON serializer teachable

For a tiny temporary IR, Rust's `String` and `char` APIs can make escaping and output order visible, but hand-written serialization should not become a schema strategy as the format grows.

## Origin

Malcolm's first JSON IR has no external dependencies and only needs strings,
arrays, objects, and `null`. The serializer was kept in the standard library
so the first cross-language artifact could be understood without introducing a
dependency or a second configuration layer.

## What

The serializer appends JSON punctuation and values to a mutable `String`.
`&mut String` is an exclusive borrowed reference that lets helper functions
append to the same output buffer without taking ownership of it. String values
are written character by character so quotes, backslashes, newlines, tabs, and
other control characters receive JSON escapes.

The output functions use the same field sequence every time. Rust's `for`
loops and `enumerate()` make it straightforward to preserve vector order while
adding commas only between items.

## Why

Making the escaping code visible is useful at this learning stage: JSON is a
text format, not a safe interpolation context. A direct `push_str` of an
expression containing a quote would create invalid JSON. Explicit helper
functions also make the current artifact policy—field order, null handling,
and array order—easy to test and review.

The limitation is equally important. Manual serializers become fragile when
types, nesting, optional fields, or schema versions multiply. The current code
is a teaching and prototype boundary, not a recommendation to avoid `serde`
forever.

## Example

```rust
fn write_json_string(output: &mut String, value: &str) {
    output.push('"');
    output.push_str(&value.replace('"', "\\\""));
    output.push('"');
}
```

This abbreviated example shows the ownership shape, but it is incomplete: a
real JSON string must also handle backslashes and control characters. Malcolm's
implementation covers those cases explicitly and tests them.

## Gotchas

- `push` accepts one `char`; `push_str` accepts a borrowed string slice `&str`.
- A backslash must itself be escaped before JSON can carry it safely.
- Unicode text can remain UTF-8, but control characters must use JSON escapes.
- Deterministic output is not automatically canonical output. Whitespace,
  numeric representation, and future object-key rules still need a contract if
  bytes will be hashed.
- A serializer should only serialize data. It must not evaluate Malcolm
  expressions or contact a subject as a side effect.

## Used in

- [`malcolm/src/ir.rs`](../../malcolm/src/ir.rs)
- `cargo test --manifest-path malcolm/Cargo.toml`

## Related

- [`Malcolm's JSON IR is a transport boundary, not an evaluator`](../modules/malcolm-json-ir.md)
- [The Rust Book: Strings](https://doc.rust-lang.org/book/ch08-02-strings.html)
- [The Rust Book: References and borrowing](https://doc.rust-lang.org/book/ch04-02-references-and-borrowing.html)
- [`Notes protocol`](../../NOTES.md)
