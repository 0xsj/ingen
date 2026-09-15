# The generic storage seam should be exercised under concurrent callers

Atomic backend operations only matter if the store layer is tested under the
same concurrent access patterns that production callers will use.

## Origin

Amber's Go race coverage exercised the memory and file stores, while the
generic `KeyValueStore` relied on the backend's documented atomicity without an
end-to-end concurrent test. The TypeScript SDK had no concurrent storage test.

## What

The Go and TypeScript storage tests now run concurrent duplicate writes and
concurrent correlation-history reads through both the memory and generic
key-value stores. Each value is written twice concurrently; the final history
must contain exactly one record per execution and no write or read may fail.

The Go test runs under the existing race-detector target. The TypeScript test
uses concurrent promises to exercise the asynchronous backend contract.

## Why

The generic store owns conflict handling, while the backend owns atomic
insert-if-absent behavior. End-to-end concurrency coverage verifies that these
responsibilities compose without duplicate records, lost writes, conflicts on
identical retries, or unsafe map access.

## Example

Run Go's race-enabled storage coverage and the TypeScript contract coverage with:

```sh
make race
cd typescript && npm test
```

Both suites exercise duplicate writes and concurrent history reads through the
generic key-value seam.

## Gotchas

- The built-in map backends are reference implementations; this does not prove
  that an external database or service backend implements atomic insertion.
- A user-provided backend must still make `PutIfAbsent` atomic for a key under
  its own concurrency model.
- The test does not promise transaction isolation, cross-process file locking,
  or distributed ordering.

## Used in

- [`go/adapters/storage/concurrency_test.go`](../../go/adapters/storage/concurrency_test.go)
- [`typescript/src/storage.contract.test.ts`](../../typescript/src/storage.contract.test.ts)
- [`go/adapters/storage/keyvalue.go`](../../go/adapters/storage/keyvalue.go)
- [`typescript/src/storage.ts`](../../typescript/src/storage.ts)
- [`Makefile`](../../Makefile)

## Related

- [Storage concurrency should be verified with the race detector](024-storage-race-coverage.md)
- [Every Go storage adapter should prove the same semantic contract](038-shared-go-storage-contract-suite.md)
- [TypeScript storage backends should prove the same semantic contract](039-shared-typescript-storage-contract-suite.md)
