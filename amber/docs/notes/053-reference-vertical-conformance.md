# The reference verticals should share one observable conformance fixture

Individually passing SDK tests do not prove that the same application flow
means the same thing in both languages.

## Origin

Amber had shared core, transport, storage, logging, tracing, and OpenTelemetry
fixtures, while the new HTTP and messaging reference verticals were verified by
language-specific tests. Their observable child behavior could therefore drift
without a shared fixture making the difference visible.

## What

`conformance/reference-v1.json` defines the shared expectations for the
reference flows:

- successful HTTP and messaging handling;
- incoming child origin and depth increment;
- execution causation and correlation preservation;
- explicit child response propagation; and
- malformed HTTP rejection.

The Go service and consumer tests read the fixture directly. The TypeScript
conformance runner executes both reference handlers against the same expected
values.

## Why

This adds a small, stable semantic checkpoint without requiring deterministic
random IDs, a broker, or a network service. It tests the contract that a
deployment can observe while keeping runtime and infrastructure choices
application-owned.

## Used in

- [`conformance/reference-v1.json`](../../conformance/reference-v1.json)
- [`conformance/README.md`](../../conformance/README.md)
- [`go/examples/service/main_test.go`](../../go/examples/service/main_test.go)
- [`go/examples/messaging/main_test.go`](../../go/examples/messaging/main_test.go)
- [`typescript/test/conformance.test.mjs`](../../typescript/test/conformance.test.mjs)

## Related

- [The Go HTTP reference vertical should have focused boundary tests](049-go-http-reference-vertical-tests.md)
- [The TypeScript HTTP reference vertical should match the Go boundary](050-typescript-http-reference-vertical.md)
- [The messaging reference vertical should preserve consumer-owned child metadata](052-messaging-reference-vertical.md)
