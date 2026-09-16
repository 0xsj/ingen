# Rust's Result keeps CLI failures separate from successful output

A small Rust CLI becomes easier to reason about when argument parsing, file I/O, parsing, validation, and output each return errors instead of printing or panicking inside the library pipeline.

## Origin

Malcolm's first user-facing command was added with Rust 1.86.0 and had to
bridge command-line arguments, filesystem/stdin input, the existing library
API, and stdout. The boundary needed to remain understandable without a CLI
framework dependency.

## What

The run function is generic over its argument iterator and output writer. An
IntoIterator<Item = String> can be supplied by env::args() in production or
by a small array in tests. A generic Write target lets tests capture stdout in
a Vec<u8> instead of launching a process.

Each fallible step returns Result. The binary's main function is the place that
converts the final error into a diagnostic on stderr and a nonzero exit code;
the library and testable command function do not terminate the process.

## Why

Printing errors deep inside the pipeline would make the code hard to reuse and
hard to test. Panicking on a missing file or malformed contract would also turn
normal user input into a crash. A CLI-specific error enum lets the command keep
usage, I/O, parse, validation, and output failures distinct while still
presenting one readable interface to a person.

The ? operator passes each error back to main's boundary. This makes the happy
path read as the actual pipeline—read, parse, compile, write—while the type
system requires every failure path to be handled.

## Example

~~~rust
let source = read_source(&options.input)?;
let specification = parse(&source)?;
let ir = compile(&specification)?;
stdout.write_all(ir.to_json().as_bytes())?;
~~~

Malcolm wraps the different error types into CliError so this compact shape can
still provide phase-specific diagnostics.

## Gotchas

- env::args() includes the executable name; the CLI passes skip(1) to its
  argument parser.
- stdout is a stream, not a file path. A test writer such as Vec<u8> is useful
  because it implements the same Write interface.
- Read::read_to_string requires valid UTF-8. Malcolm source is currently
  defined as UTF-8 text, so invalid bytes are reported as input failure.
- ? does not catch panics; it propagates values returned as Err.
- A command that writes a file has a filesystem side effect. The library
  compiler remains free of that side effect, which keeps it reusable.

## Used in

- [malcolm/src/main.rs](../../malcolm/src/main.rs)
- cargo run --manifest-path malcolm/Cargo.toml -- malcolm/examples/document_api.malcolm

## Related

- [Malcolm's CLI keeps the compile pipeline visible](../modules/malcolm-cli.md)
- [Rust ownership makes parser state explicit](rust-ownership-in-first-parser.md)
- [The Rust Book: Error handling](https://doc.rust-lang.org/book/ch09-00-error-handling.html)
- [Notes protocol](../../NOTES.md)
