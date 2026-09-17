# Sorna has one broader release checkpoint

The original `make sorna-alpha-check` remains the focused Sorna readiness
gate: package tests, vet, published schema syntax, and the fresh replay matrix.
As provider and downstream boundaries were added, those checks were otherwise
easy to run separately and forget.

`make sorna-release-check` now composes the current proof surface:

```text
Sorna package/vet/schema/replay gate
    + provider conformance corpus
    + TypeScript manifest and exact-binding review
    + Nublar aggregate, durable collection, reload, and decision projection
```

The TypeScript/Nublar portion runs with a fresh temporary artifact root so
output-protection from earlier local runs cannot make the checkpoint
non-repeatable. It still leaves the existing Go behavioral provider and
campaign semantics unchanged.

This is a release checkpoint, not a production-grade isolation attestation.
The alpha limitations around host observation and proof that an oracle author
never saw implementation details remain unchanged.
