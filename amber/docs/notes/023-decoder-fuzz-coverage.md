# Decoder fuzzing should protect the untrusted input boundary

Untrusted provenance enters Amber through decoders, so malformed input should
be exercised beyond a fixed list of hand-written examples.

## Origin

The core and transport adapters already bounded and validated incoming values,
but their tests primarily used known malformed cases. Decoder code is a small
and high-leverage target for fuzzing because it parses attacker-controlled JSON,
base64, enums, IDs, and bounded collections.

## What

Go fuzz targets cover `FromJSON`, `InspectIncomingJSON`, and transport
`DecodeValue`. Any accepted value must still validate, marshal successfully,
and preserve its execution identity through a JSON round trip. Any rejected or
ignored value must not be returned as present. A `make fuzz` target runs short
five-second passes locally for the core and transport packages.

## Why

Fuzzing explores combinations of malformed bytes and valid-looking fields that
are easy to miss with example-based tests. Keeping fuzzing opt-in avoids making
the normal cross-language check gate timing-dependent while leaving a single,
documented command for local hardening.

## Example

```sh
make fuzz
```

The targets can also be run for longer during release hardening:

```sh
cd go && go test . -run '^$' -fuzz FuzzFromJSON -fuzztime 2m
```

## Gotchas

- Fuzz targets verify decoder safety and model invariants; they do not prove
  authenticity or authorization.
- The normal test suite executes the seed corpus, but broad mutation requires
  an explicit fuzz run.
- The incoming JSON byte bound remains part of `InspectIncomingJSON`; direct
  `FromJSON` callers are responsible for choosing an appropriate input limit.

## Used in

- [`go/fuzz_test.go`](../../go/fuzz_test.go)
- [`go/adapters/transport/fuzz_test.go`](../../go/adapters/transport/fuzz_test.go)
- [`Makefile`](../../Makefile)
- [`README.md`](../../README.md)

## Related

- [Incoming handling must make the failure policy explicit](005-explicit-incoming-policy.md)
- [The HTTP adapter should preserve the core JSON inside a transport-safe envelope](006-http-header-adapter.md)
- [Amber v1 should stay unsigned by default while exposing an explicit trust seam](022-unsigned-v1-trust-seam.md)
