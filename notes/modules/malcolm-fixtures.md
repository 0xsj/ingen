# Malcolm carries fixture identity without owning fixture bytes

An oracle may depend on known input data, but a language specification should
not silently read arbitrary files or turn subject-owned implementation fixtures
into verifier inputs.

## Origin

Sorna's contract boundary already supports fixture metadata and digest
validation. Malcolm previously had no typed way to preserve that metadata, so a
fixture reference would have to be added outside the Rust-to-Sorna handoff.
The next boundary question was how to verify provider-supplied bytes without
making the portable contract depend on a local artifact directory.

## What

Malcolm supports one narrow fixture declaration:

~~~text
fixture "welcome-document" {
  owner oracle
  purpose "canonical document input bytes"
  sha256 "637cecb53db658da5231f933fad366fce94f982501e59d7ddc7688cfaf81b825"
}
~~~

The Rust AST and `malcolm.ir/v1` retain the fixture ID, `oracle` owner, human
purpose, and lowercase 64-character SHA-256 digest. The Sorna adapter lowers
these values to `contract.fixtures`. Sorna's existing contract seal keeps the
digest in the sealed artifact.

Sorna now accepts a separate `ingen.fixture-provider/v1` manifest whose entries
map those IDs to relative file paths and repeat the expected digest. The
`sorna fixture bind` command requires the contract to already be sealed,
resolves each path below the provider root, hashes the actual regular file,
and emits an `ingen.fixture-handoff/v1` artifact only when every byte digest
matches. The handoff records the relative path and byte count, but never embeds
the fixture bytes or the provider root.

## Ownership

The declaration identifies data owned by the oracle boundary. Malcolm checks
the declaration's shape; Sorna validates and seals the contract metadata; the
fixture provider or orchestrator supplies a path and bytes, while Sorna
verifies that the supplied file matches the declared digest. Subject-owned
binaries, mutation fixtures, and reset hooks remain outside this declaration.

## Why

Digest-pinned identity makes fixture dependencies reviewable without giving the
Rust compiler permission to guess filesystem locations. The separate provider
handoff binds a concrete file to that identity while keeping environment-local
paths out of the sealed contract. This preserves the separation between oracle
inputs and implementation fixtures, which is necessary for an independent
verification boundary.

## Gotchas

- Only `owner oracle` is supported by Malcolm. Sorna's broader contract model
  may retain `subject` metadata for other producers, but Malcolm rejects it.
- The current declaration requires `id`, `purpose`, and `sha256`; it has no
  `path` field and does not load, copy, or materialize fixture bytes.
- The declaration itself is identity metadata. The file-based provider handoff
  verifies matching bytes before a consumer uses it, but does not copy or
  embed those bytes.
- Provider paths must be relative, remain below the provider root, resolve to
  regular files, and use lowercase SHA-256 digests. Symlinks that resolve
  outside the root are rejected.
- The current provider handoff is path-based. An in-memory byte handoff and
  automatic fixture materialization into request bodies remain unsupported.
- Request bodies remain explicit typed literals. Declaring a fixture does not
  implicitly populate a scenario's `given body`.
- Subject-owned reset endpoints are lifecycle controls, not oracle input
  fixtures. See the isolation note for that separate boundary.

## Used in

- [`malcolm/src/lib.rs`](../../../malcolm/src/lib.rs)
- [`malcolm/src/parser.rs`](../../../malcolm/src/parser.rs)
- [`malcolm/src/semantic.rs`](../../../malcolm/src/semantic.rs)
- [`malcolm/src/ir.rs`](../../../malcolm/src/ir.rs)
- [`sorna/internal/malcolm/adapter.go`](../../../sorna/internal/malcolm/adapter.go)
- [`sorna/internal/contract/contract.go`](../../../sorna/internal/contract/contract.go)
- [`sorna/internal/fixture/fixture.go`](../../../sorna/internal/fixture/fixture.go)
- [`sorna/spec/ingen.fixture-provider-v1.schema.json`](../../../sorna/spec/ingen.fixture-provider-v1.schema.json)
- [`sorna/spec/ingen.fixture-handoff-v1.schema.json`](../../../sorna/spec/ingen.fixture-handoff-v1.schema.json)
- [`malcolm/examples/document_fixture.malcolm`](../../../malcolm/examples/document_fixture.malcolm)
- [`malcolm/examples/document_fixture_provider/provider.yaml`](../../../malcolm/examples/document_fixture_provider/provider.yaml)
- `Makefile` targets `malcolm-sorna-fixture-contract` and `malcolm-sorna-fixture-handoff`

## Related

- [Malcolm makes scenario reset semantics explicit](malcolm-isolation.md)
- [Malcolm's JSON IR is a transport boundary, not an evaluator](malcolm-json-ir.md)
- [A managed subject needs its own isolation policy](sorna-subject-isolation.md)
- [The contract can cross language boundaries](../concepts/the-contract-can-cross-language-boundaries.md)
- [Notes protocol](../../../NOTES.md)
