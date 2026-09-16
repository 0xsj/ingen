# A background worker should make child and retry semantics concrete

Amber's next reference vertical is a deployment-agnostic background worker
that exercises logical work, incoming child execution, retry semantics, and
storage without requiring a broker or hosted deployment.

## Origin

The HTTP and messaging reference verticals already demonstrate network and
consumer boundaries. Deployment remains backlogged, so the next example should
show how Amber is used inside a worker function itself without choosing a
specific queue, scheduler, or runtime service.

## What

The repository now includes matching worker flows:

- [`go/examples/worker`](../../go/examples/worker/); and
- [`typescript/src/worker-example.ts`](../../typescript/src/worker-example.ts),
  backed by [`typescript/src/worker.ts`](../../typescript/src/worker.ts).

Each flow accepts a parent provenance value, derives an incoming child, stores
the child, records a retry of the same logical work, and reads the ordered work
history. Focused tests verify child/retry origin, work identity, causation, and
context/error behavior where the SDK supports it.

## Why

This vertical demonstrates that Amber's core value is not tied to HTTP or a
message broker. A worker can preserve logical work and retry history while the
application remains responsible for job delivery, acknowledgement, scheduling,
and retry policy.

## Example

The application-owned worker operation has the same shape in both SDKs:

```go
child, err := parent.Child(amber.ChildOptions{Origin: amber.OriginIncoming})
if err != nil {
	return err
}
if err := store.Put(ctx, child); err != nil {
	return err
}
retry, err := child.Retry()
```

The worker stores both executions and can query them with `ListByWorkID` or
`listByWorkId`.

## Gotchas

- A worker example does not define queue acknowledgement, leasing, scheduling,
  or delivery semantics.
- A retry is another execution of the same logical work; it is not a new work
  ID.
- The worker's incoming trust policy and storage durability remain
  application-owned.
- This vertical does not add a new wire field, transition, or required runtime
  dependency.

## Used in

- [`go/examples/worker`](../../go/examples/worker/)
- [`go/examples/worker/main_test.go`](../../go/examples/worker/main_test.go)
- [`typescript/src/worker.ts`](../../typescript/src/worker.ts)
- [`typescript/src/worker-example.ts`](../../typescript/src/worker-example.ts)
- [`typescript/src/worker.test.ts`](../../typescript/src/worker.test.ts)
- [`Makefile`](../../Makefile)
- [`examples/README.md`](../../examples/README.md)
- [`docs/getting-started.md`](../getting-started.md)

## Related

- [The first deployment vertical should prove a real Go HTTP service path](047-go-http-reference-vertical.md)
- [The messaging reference vertical should preserve consumer-owned child metadata](052-messaging-reference-vertical.md)
- [The reference verticals should share one observable conformance fixture](053-reference-vertical-conformance.md)
- [User-defined adapters should reuse Amber's invariant-owning seams](065-user-defined-adapter-authoring.md)
- [Custom backend examples should make the generic seam copyable](066-custom-backend-examples.md)
