# Storage concurrency should be verified with the race detector

Storage is shared mutable infrastructure, so its append-only guarantees need
verification under concurrent access rather than only sequential examples.

## Origin

The memory store used `sync.RWMutex` and the file store serialized operations
with a mutex, but the test suite did not exercise overlapping writers and
queries. The storage contract also promises idempotent re-insertion, which is
especially important when concurrent delivery causes the same execution to be
observed more than once.

## What

Storage concurrency tests write distinct child executions from multiple
goroutines, repeat writes for the memory store to exercise idempotence, and run
correlation queries while writes are in progress. The file-store test verifies
that concurrent writes through one store instance leave a readable complete
snapshot after reopening. `make race` runs the full Go suite with the race
detector.

## Why

The race detector can catch unsynchronized access that ordinary tests may never
schedule together. The tests also verify the externally visible result: no
write errors, no lost records, and a valid persisted snapshot. This remains a
process-local guarantee for `FileStore`; independent processes still require a
database or an explicit cross-process locking strategy.

## Example

```sh
make race
```

## Gotchas

- `MemoryStore` is concurrency-safe but not durable.
- `FileStore` is safe for concurrent use through one instance; it does not
  coordinate independent `FileStore` instances in separate processes.
- Race-detector runs are slower and are intentionally separate from the normal
  `make check` gate.

## Used in

- [`go/adapters/storage/concurrency_test.go`](../../go/adapters/storage/concurrency_test.go)
- [`go/adapters/storage/storage.go`](../../go/adapters/storage/storage.go)
- [`Makefile`](../../Makefile)
- [`README.md`](../../README.md)

## Related

- [Storage should distinguish idempotent re-insertion from overwriting history](010-append-only-storage-contract.md)
- [Durable storage should preserve append-only semantics behind an explicit backend seam](014-durable-storage-backend-seam.md)
- [Decoder fuzzing should protect the untrusted input boundary](023-decoder-fuzz-coverage.md)
