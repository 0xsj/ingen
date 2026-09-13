# The first Sorna runner accepts a subject URL instead of a subject package

The runner observes a subject through HTTP so the first executable Sorna result is transport-level rather than an in-process test of the subject.

## Origin

The first end-to-end verification slice after the document-pipeline subject was
implemented. The subject's own tests establish local development behavior, but
they cannot establish that Sorna is independent of the subject implementation.
The runner now supports both an external URL and a Sorna-managed subject
process; this note focuses on the public HTTP evaluation itself.

## What

The runner loads and seals a contract, sends each rule's public HTTP request to
a base URL, normalizes the JSON response, evaluates the declared status/body
predicates, and writes a JSON run record. Stateful rules execute their
declared public setup sequence first and carry named captures into the target
path. The lifecycle manager may establish that base URL before the runner
starts and records its own evidence beside the rule results.

## Why

Importing the subject package would make the verifier depend on implementation
types and could accidentally expose private state. Treating a stateful rule as
a standalone request would test a different scenario, so setup is explicit and
its observations are retained in the rule result.

## Example

```text
make sorna-run         # Sorna manages the subject lifecycle
make sorna-external-run # use an already-running subject
```

The resulting `run.json` contains the sealed contract hash, request and
normalized observation for each executed case, assertion-level results, and an
assurance level of 0 (`self-reported`).

## Gotchas

- A URL boundary alone does not prove capability isolation; the run says so
  explicitly in its limitations.
- Setup failures are reported as `inconclusive` for the target rule rather than
  being mistaken for a target assertion failure.
- A passing baseline only shows agreement with this contract and subject; it
  does not prove correctness or mutation sensitivity.
- The first evaluator supports only the small status/object/error predicates
  used by the document lab, not an arbitrary assertion language.

## Used in

- [`sorna/internal/runner`](../../sorna/internal/runner/)
- [`sorna run`](../../sorna/cmd/sorna/)
- [`document-pipeline lab`](../../examples/document-pipeline-lab/)

## Related

- [`Sorna evidence specification`](../../sorna/EVIDENCE-SPEC.md)
- [`The contract can cross language boundaries`](../concepts/the-contract-can-cross-language-boundaries.md)
- [`A state label is not an executable transition`](../concepts/a-state-label-is-not-a-transition.md)
- [`A subject URL is not an isolation boundary`](../concepts/a-url-is-not-an-isolation-boundary.md)
- [`The document subject keeps workflow transitions visible at the boundary`](document-pipeline-subject.md)
