# Rust ownership makes parser state explicit

Rust's ownership and borrowing rules make the parser's temporary state visible, while owned AST values keep the parsed model independent of the source buffer's lifetime.

## Origin

Malcolm is the first Rust implementation in this repository. The initial
parser was built with Rust 1.86.0 and exposed several concepts that are easy to
miss when coming from Go: borrowed input, owned strings, `Option`, `Result`,
enums, and derived traits.

## What

The parser receives `&str`, a borrowed view of the caller's source text. Each
AST field stores an owned `String`, so the returned specification does not need
the original source text to stay alive. `Vec<T>` owns a growable list of child
values, and `Option<T>` represents state that may not exist yet, such as the
current specification or the current scenario.

`Result<T, E>` makes success and failure part of the function's type. The `?`
operator returns a parse error immediately when a nested operation fails. The
`RequirementKind` enum makes the two requirement forms explicit instead of
representing them with a loosely agreed string.

## Why

Borrowing the input avoids copying the whole source while parsing, but storing
references in the AST would introduce lifetime relationships between the AST
and its input. Copying each small field into an owned `String` is the simpler
boundary for a compiler model that should be passed to later stages.

The parser uses `as_mut()` to temporarily borrow an `Option`'s contained value
for editing, and `take()` to move a completed scenario out of the option and
replace it with `None`. This makes the parser's state transitions explicit:
there is either an unfinished scenario being built or a completed one ready to
attach to the specification.

## Example

```rust
let result: Result<Specification, ParseError> = malcolm::parse(source);
let specification = result?;
```

The first line says the operation may succeed with a `Specification` or fail
with a `ParseError`. The second line unwraps only the success path; an error is
returned to the caller instead of being silently ignored.

The structs derive `Debug`, `Clone`, and equality traits so tests can inspect
values directly. `pub` controls what callers can use, while the parser module
itself stays private and is exposed through the small `parse` re-export.

## Gotchas

- `&str` is a borrowed string slice; `String` owns its text. Use `.to_owned()`
  or `.to_string()` when a returned value must outlive the input borrow.
- `&mut T` is an exclusive temporary borrow. While it is active, the same
  value cannot also be changed through another reference.
- `Option::take()` moves the value out and leaves `None`; it is not a copy.
- `unwrap()` and `expect()` can panic. The parser's `expect` calls protect
  internal states established immediately before them; user-facing malformed
  input is handled with `Result` instead.
- `#[derive(...)]` generates useful implementations, but it does not define
  domain semantics. Equality here is structural equality for tests.
- `cargo fmt` formats Rust source; `cargo test` compiles the crate and runs
  unit tests. Both are part of the feedback loop for this slice.

## Used in

- [`malcolm/src/lib.rs`](../../malcolm/src/lib.rs)
- [`malcolm/src/parser.rs`](../../malcolm/src/parser.rs)
- `cargo test --manifest-path malcolm/Cargo.toml`

## Related

- [`Malcolm's first parser preserves shape and defers meaning`](../modules/malcolm-first-parser.md)
- [The Rust Book](https://doc.rust-lang.org/book/)
- [`Notes protocol`](../../NOTES.md)
