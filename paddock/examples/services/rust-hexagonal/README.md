# Rust hexagonal adapter fixture

This is a tiny Rust-shaped source fixture for the external adapter example. It
does not require Cargo or a Rust toolchain: Paddock invokes the dependency-free
`rust-use-adapter.py`, which parses ordinary `use` declarations and returns a
`paddock.graph/v1` document.

The `good` subject keeps domain code pointed at the standard library. The
`violating` subject adds a domain-to-adapter import, which Paddock should report
through `domain-is-pure` (and the resulting cycle through `no-cycles`).
