# Sorna standalone CLI build

Date: 2026-09-17

## Decision

Sorna now has a small standalone binary build surface. `make sorna-build`
produces `.artifacts/sorna/sorna` with `-trimpath` and embeds version, commit,
and build-date metadata through Go linker flags. The binary exposes
`sorna version --format text|json` for human inspection and CI probes.

## Boundary

This is packaging, not a new verification mode. The binary contains the same
CLI currently exercised through `go run`; it does not add release archives,
install behavior, or cross-platform claims yet. CI builds it after the Sorna
release checkpoint and checks the JSON metadata before uploading it.

## Verification

The build target is exercised by the repository Sorna workflow. A local build
can be checked with:

```sh
make sorna-build
./.artifacts/sorna/sorna version --format json
```
