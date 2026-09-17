# A local preflight should validate dependencies before registration

Hammond's `validate` CLI command is a read-only preflight for a record file.
It loads the referenced contract, policy, and local authority artifacts and
checks their digests and governance invariants without creating a store entry
or mutating any record.

## Why

Registration is the first mutating operation. A separate preflight gives a
solo operator a safe way to catch missing files, digest drift, malformed
authority mappings, or policy mismatches before publishing a record.

## Gotchas

- `validate` does not check the current store revision; use `revision` before
  each mutation.
- A successful preflight does not authorize future events by itself; the
  referenced policy and authority are applied again during mutation.
- The command remains local-only and does not contact GitHub or a hosted
  Hammond service.

## Used in

- `hammond/cmd/hammond/main.go`
- `hammond/cmd/hammond/main_test.go`
- `hammond/examples/solo/README.md`
