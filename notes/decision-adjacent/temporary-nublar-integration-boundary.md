# The current Nublar slice is an integration harness, not the final product

## Decision

The Nublar code currently present in this repository exists only to prove that
Sorna can work end to end through a CI-facing boundary:

```text
Sorna verification
    -> ingen.ci-result/v1
        -> temporary Nublar aggregation
```

This slice is useful for exercising Sorna's evidence and result protocols, but
it is not the architectural foundation of the final Nublar product.

## Intended ownership

The eventual Nublar will be rewritten as an isolated, standalone product with
its own architecture, lifecycle, storage, CI integrations, and ownership. A
separate agent or workstream should own that implementation.

Sorna must remain independently useful without Nublar. Its durable integration
responsibility is to produce well-defined shared artifacts, especially
`ingen.ci-result/v1`.

## Consequence

Future Nublar design should not be constrained by the current aggregation
prototype. The prototype may be retained as an example, replaced, or removed
when the standalone Nublar implementation begins. Shared protocols and artifact
semantics are the intended compatibility boundary; the current Nublar package
layout is not.
