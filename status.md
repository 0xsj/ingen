# InGen status

As of 2026-09-14

## Current checkpoint

InGen is at a Sorna alpha vertical slice: a contract-driven verification lab can run a subject, execute behavioral checks, execute mutation campaigns, collect evidence, and expose the results to a CI-style consumer.

Nublar currently acts as a thin coordinator and proof surface. It is intentionally not the final Nublar product; a later rewrite can make it a standalone product owned and designed separately.

## What is proven so far

- Sorna loads, validates, canonicalizes, and seals contract and oracle inputs.
- Baseline preconditions, subject policies, process isolation, access/executable telemetry, and evidence bundles are implemented.
- Mutation plans and provider manifests have explicit schemas and are bound to concrete prepared subjects.
- The document-pipeline example has two meaningful mutations:
  - changing the accepted create response from `202` to `200`;
  - removing the required `name` field from the create response.
- Both mutations have been killed by contract-driven checks. This demonstrates that the contract can detect defects; it is not, by itself, proof that the contract is complete.
- A reusable Go source provider can copy a subject, apply AST mutations, build isolated variants, record source/binary hashes, and publish a provider manifest only after successful preparation.
- Provider capabilities are declared explicitly and reviewed before execution. The review is no-execution and can be emitted as the shared `ingen.ci-result/v1` envelope.
- Campaign provenance, source/binary integrity checks, evidence verification, and campaign-result verification are in place.
- Nublar can aggregate the provider preflight result with the behavioral verification result.
- Notes, module explanations, Make targets, and example documentation have been kept alongside the implementation.

## Useful entry points

```sh
make mutation-provider-inspect
make mutation-provider-ci-result
```

The Go source-provider campaign can be run with `make mutation-go-campaign-run`; use fresh output directories when overriding its paths. The resulting campaign can be checked with the matching `make mutation-go-campaign-verify` target.

## Important findings and limitations

- Green implementation tests are weak evidence unless the contract is independently meaningful. The killed mutations are the first concrete signal that the document-pipeline contract is sensitive to important behavioral defects.
- Mutation killing measures contract sensitivity, not contract correctness or completeness. A surviving mutation is useful evidence, but it still needs diagnosis.
- Source and binary hashes establish pre-launch integrity and detect drift. They do not constitute a cryptographic proof that an agent never saw implementation details; that requires stronger execution provenance and environment design.
- The fixture provider is currently usable for local demonstrations without a plan hash binding. The generated Go provider supports a matched plan hash. A release policy still needs to decide when an unbound provider is acceptable.
- Provider outputs, evidence outputs, and review outputs intentionally refuse unsafe reuse of existing paths. Campaigns should therefore use fresh artifact directories.
- The scoped validation suite passes:

  ```sh
  go test ./core/... ./sorna/... ./examples/... ./nublar/...
  go test -race ./core/ciresult ./sorna/internal/campaign ./sorna/internal/evidence ./sorna/cmd/sorna ./nublar/internal/aggregate ./nublar/internal/workflow
  ```

  A full `go test ./...` remains blocked by unrelated pre-existing Paddock compile errors (`loadGraphDocument` and `loadExternalGraphRequest`).
- On some local runs, macOS Seatbelt process inspection requires host permission; the same scoped runs succeed when executed with the required permission.

## Potential remaining avenues

These are candidate directions, not an artificial checklist to complete all at once.

### Reproducibility checkpoint

- Re-run the complete Sorna slice from a clean checkout and record the exact artifact set.
- Decide whether production-ready providers must always be plan-hash matched, with unbound providers restricted to explicitly local/demo use.
- Freeze the Sorna alpha boundary before adding more mutation operators.

### CI and Nublar

- Decide whether mutation campaign results should also be emitted as a shared CI result, so Nublar can aggregate behavioral verification, provider preflight, and mutation outcomes uniformly.
- Define the CI failure taxonomy and exit-code contract for blocked preflight, failed behavioral rules, killed mutations, surviving mutations, and infrastructure errors.
- Add a clean workflow execution path that assembles the full proof without relying on manually chosen commands.

### Sorna hardening

- Move the provider capability vocabulary into a versioned contract/schema rather than keeping it only in code and examples.
- Improve provider build diagnostics and preserve a reviewable source-diff or mutation-summary artifact.
- Continue tightening TOCTOU handling, immutable inputs, subprocess boundaries, and telemetry guarantees.
- Add negative cases for malformed providers, ambiguous targets, unsupported operators, drift, and partial preparation.

### Mutation depth and examples

- Add operators only when they represent meaningful contract risks, beginning with more precise target resolution and ambiguity reporting.
- Expand the document-pipeline lab and later add the webhook and document-processing scenarios discussed earlier.
- Use surviving mutations to guide contract improvements instead of optimizing for a mutation score.

### SDKs and product boundaries

- Keep the provider boundary language-neutral; add TypeScript or other SDKs only after the Go provider contract is stable.
- Define the future Sentinel contract workspace separately from Sorna's execution internals.
- Treat the current Nublar integration as a proof-of-concept envelope and plan its later standalone rewrite.

## Recommended next step

Use the next checkpoint to establish clean-checkout reproducibility and decide the policy for unbound versus plan-hash-matched providers. After that, freeze the alpha interfaces and choose whether the next concrete investment is the unified mutation CI result or deeper contract/mutation semantics.

This file is a project checkpoint, not a requirement to implement every avenue listed above immediately.
