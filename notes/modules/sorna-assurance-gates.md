# CI should gate behavior first and observation quality explicitly

An observation gap and a contract failure are different facts. A CI surface
needs to preserve both instead of silently turning every warning into a build
failure or every successful behavior result into an isolation claim.

## Decision

Sorna now has a separate `gate` command. `evidence verify` remains an integrity
and structural check; `gate` applies acceptance policy after verification.

The default gate blocks:

- a non-passing ordinary contract verdict;
- a declared mutation that was not killed (a killed mutation is a passing
  sensitivity result even though its contract verdict is expected to fail);
- an incomplete oracle execution.

Observation coverage is report-only by default. A caller can opt into a strict
minimum with `--minimum-observation-coverage periodic-best-effort`. The weaker
`periodic-best-effort-with-gaps` threshold allows known gaps while still
blocking unavailable or unobserved telemetry.

## Why

The default is intentionally conservative about interpretation but practical
for adoption. A periodic sampler can miss a transition between ticks even when
no sampler error occurred. That fact should be visible to CI and review, but a
team must explicitly choose when it becomes a blocking policy. The policy is
therefore separate from the evidence producer and from the behavioral verdict.

## Result

`sorna gate` emits `ingen.gate/v1` with the evidence schema, gate status, exit
code, observation policy, observation coverage, behavioral status, and reasons.
Text is convenient for local use; `--format json` is the CI integration form.

```sh
sorna evidence verify .artifacts/document-pipeline-run
sorna gate .artifacts/document-pipeline-run
sorna gate --minimum-observation-coverage periodic-best-effort \
  .artifacts/document-pipeline-run
sorna gate --format json .artifacts/document-pipeline-run
```

## Limits

This is an initial bundle-level gate, not yet the full Nublar workflow. It does
not approve contracts, interpret waivers, or claim that periodic observation
is continuous. A future CI coordinator can compose this result with Paddock,
mutation campaign policy, review state, and artifact provenance.

## Related

- [`Sorna evidence specification`](../../sorna/EVIDENCE-SPEC.md)
- [`Executable identity is a timeline, not a startup fact`](sorna-executable-identity-history.md)
- [`InGen CI result artifact`](../../core/CI-RESULT-SPEC.md)
