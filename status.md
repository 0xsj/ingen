# InGen status

As of 2026-09-16

## Current checkpoint

InGen is at a Sorna alpha vertical slice: a contract-driven verification lab can run a subject, execute behavioral checks, execute mutation campaigns, collect evidence, and expose the results to a CI-style consumer.

Nublar currently acts as a thin coordinator and proof surface. It is intentionally not the final Nublar product; a later rewrite can make it a standalone product owned and designed separately.

## What is proven so far

- Sorna loads, validates, canonicalizes, and seals contract and oracle inputs.
- Baseline preconditions, subject policies, process isolation, access/executable telemetry, and evidence bundles are implemented.
- Mutation plans and provider manifests have explicit schemas and are bound to concrete prepared subjects.
- The document-pipeline example has six meaningful mutations:
  - changing the accepted create response from `202` to `200`;
  - removing the required `name` field from the create response.
  - changing the unsupported-document response from `400` to `500`.
  - leaving the process response `queued` instead of transitioning to `completed`.
  - persisting under a key different from the returned document ID.
  - accepting an unsupported PNG document type.
- All six mutations have been killed by contract-driven checks. This demonstrates that the contract can detect defects; it is not, by itself, proof that the contract is complete.
- A reusable Go source provider can copy a subject, apply AST mutations, build isolated variants, record source/binary hashes, and publish a provider manifest only after successful preparation.
- Provider capabilities are declared explicitly and reviewed before execution. The review is no-execution and can be emitted as the shared `ingen.ci-result/v1` envelope.
- Plan binding is now an explicit caller policy: unbound fixture providers remain usable for local demonstrations, while `--require-plan-binding` blocks them. The generated Go provider passes the strict matched-binding review.
- `make mutation-go-provider-ci-result` provides a strict positive-path example; the fixture provider's strict review returns `blocked` as expected.
- The default Nublar document workflow now uses the strict Go provider and strict Go mutation campaign; the unbound fixture remains local-only.
- Verified mutation campaign results can now be emitted as the shared `ingen.ci-result/v1` envelope, preserving the complete campaign report and exposing survivors or integrity failures to Nublar.
- Mutation campaign CI explanations now classify failed entries as contract-insensitive, insufficient-observation, invalid, equivalent, timeout, execution-error, or unclassified campaign failures, with aggregate category counts.
- Campaign results now preserve expected-rule statuses and a compact per-entry diagnosis, so survivors and inconclusive outcomes remain actionable at the CI boundary.
- The former extra-field survivor is now killed by contract v2's explicit closed response shapes (`additional_properties: false`), demonstrating a diagnosed contract gap being closed without changing mutation classification.
- Source-provider provenance now records target-resolution counts, and ambiguous or missing Go AST targets return a typed `ingen.mutation-target-resolution-error/v1` preparation error without writing the source copy.
- Go provider negative-path tests now cover ambiguous state targets, missing persistence targets, and malformed validation changes; ambiguity and missing-target cases verify that the source remains unchanged.
- The versioned provider manifest schema is now published under `sorna/spec/`; it gives future language providers and CI consumers a language-neutral structural contract while Go retains semantic runtime validation.
- Shared CI exit-code mapping is now centralized in `core/ciresult` and consumed by Sorna and Nublar; producer-specific outcomes remain in their nested reports.
- Go provider preparation now emits an `ingen.mutation-preparation/v1` summary with changed source files, hashes, and provenance, and rejects no-op mutations before build/publication.
- Provider preparation now has a `mutation-preparation` CI envelope that binds the summary to the provider manifest and plan, and the default Nublar workflow retains it as a required check.
- The current Sorna/Nublar alpha boundary is now named in [ALPHA-INTERFACES.md](ALPHA-INTERFACES.md), including stable cross-tool invariants and deliberately unfrozen areas.
- Campaign provenance, source/binary integrity checks, evidence verification, and campaign-result verification are in place.
- Nublar can aggregate provider preflight, behavioral verification, mutation preparation, and mutation campaign results. The fresh document-pipeline aggregate passed all four required checks.
- All generated outputs now derive from `ARTIFACT_ROOT`, and `make nublar-aggregate-fresh` snapshots the current source tree into a new temporary workspace before running the full document workflow.
- The fresh-workspace reproducibility checkpoint is recorded in [sorna-reproducibility-checkpoint.md](notes/modules/sorna-reproducibility-checkpoint.md): stable contract/oracle/policy and binary identities matched, while the exact mutation-plan hash changed only with the regenerated baseline run ID.
- The committed-checkout reproducibility checkpoint now passes at `aacaf52` in two clean worktrees: oracle, clean baseline, and all six mutation binary hashes matched; exact plan hashes differed only by generated baseline run ID, while the semantic identity remained stable.
- Campaign plans now expose a stable semantic identity alongside the exact run-bound plan hash; strict provider execution continues to bind to exact plan bytes.
- Campaign verification now recomputes both the exact plan-byte hash and the optional semantic identity before accepting a campaign result.
- Alpha-boundary tests now reject exact plan tampering, semantic plan drift, and mismatched provider semantic identities before accepting the handoff.
- The persistence mutation demonstrates that a green create response can still be invalid across requests; the campaign preserves the resulting failure blast radius in its diagnosis.
- `sorna-ci-result` now includes the clean baseline run, so the fresh workflow no longer depends on a pre-existing run bundle.
- Notes, module explanations, Make targets, and example documentation have been kept alongside the implementation.
- The first post-alpha webhook-validation lab now has a contract, isolated policies, a stateful black-box Go subject, a baseline workflow, and a fixture-based duplicate-idempotency mutation campaign; a webhook-specific source provider remains intentionally deferred, while its opaque CI envelopes now have a Nublar workflow path.
- The webhook fixture campaign froze one semantic mutation, killed it with the duplicate rule, and preserved the campaign result as a passing CI envelope; the fixture path remains local-only until a source-level webhook provider is justified.
- The reusable Go provider now supports the webhook mutation set with strict exact-plan binding, preparation evidence, and a source-level campaign target; the source provider remains vertical-aware at the target resolver, not in the generic preparation machinery.
- The strict webhook source-provider campaign passed: one copied Go source variant was prepared, its provider and preparation envelopes passed, and the duplicate-idempotency mutation was killed in isolation.
- The webhook vertical now has a Nublar workflow declaration and fresh aggregate/run-collection targets; Nublar consumes four opaque Sorna envelopes without adding webhook-specific semantics.
- Sentinel now has a versioned webhook contract-workspace manifest and structural validator; it assembles role capabilities and Sorna/Nublar references without duplicating behavioral verification.
- Sentinel now has a versioned lifecycle receipt that hashes the workspace manifest and records ordered, artifact-linked orchestration events without reinterpreting Sorna or Nublar results.
- Sentinel now compiles a versioned declaration-only capability plan, rejects allow/deny overlap, and checks that the oracle writer denies every implementation root.
- Sentinel now binds the capability plan to exact oracle and subject policy bytes and delegates the oracle-writer execution handoff to Sorna's existing CLI/Seatbelt boundary.
- Sentinel now has a separate verifier handoff that binds a frozen oracle plus distinct oracle and subject-policy snapshots and composes Sorna's managed `run` command.
- The Sentinel oracle adapter's read-only contract probe reached macOS Seatbelt successfully with host permission; without that permission, `sandbox-exec` returns an explicit `Operation not permitted` failure rather than a false pass.
- The adapter now uses a temporary read-only policy snapshot and can persist `policy-applied`, `sorna-started`, and `sorna-completed` events into the Sentinel receipt; this reduces ordinary drift risk but is not external attestation.
- The verifier handoff can attach Sorna's opaque `run.json` to the Sentinel receipt; Sentinel records provenance and lifecycle only, while Sorna retains behavioral verdict semantics.
- Sentinel verifier receipts can now be adapted to the shared `ingen.ci-result/v1` envelope and collected by the Nublar `sentinel-webhook-verifier` workflow; Nublar sees only the envelope status and opaque receipt report.
- Sentinel now has a provider-neutral `ingen.herdr-event/v1` ingress that binds Herdr callbacks to the active run/workspace, preserves source and session identity, applies optional receipt status atomically, and makes identical retries idempotent.
- Sentinel now exposes an explicit hashed artifact handoff and root-aware event ingestion rejects missing or drifted artifact bytes before appending a Herdr event.
- Sentinel now has an audit report that verifies receipt structure, workspace/artifact hashes, and terminal lifecycle completeness without reinterpreting Sorna results or claiming independent Herdr attestation.
- The Sentinel CI-result command now runs that integrity audit before emitting a terminal shared envelope, so receipt or artifact drift cannot become a passing Nublar check.
- Sentinel now accepts newline-delimited Herdr event batches and publishes them all-or-nothing against a receipt, preserving idempotent retries without claiming queue or host persistence guarantees.
- In-place Herdr receipt updates now serialize the read-modify-publish cycle with a local advisory lock, preventing concurrent callback writers from silently dropping accepted events.
- In-place Sentinel artifact registration now uses the same advisory lock, so artifact handoff cannot silently overwrite a concurrent Herdr receipt update.
- Sentinel artifact registration now treats an exact ID/role/kind/path/hash retry as a no-op while rejecting conflicting reuse of an artifact ID.
- Sentinel oracle and verifier lifecycle writes now use the same lock at startup and completion, closing the remaining local receipt-writer race around delegated Sorna runs.
- Sentinel receipt status updates now reject terminal-to-nonterminal regressions across Herdr and direct Sorna lifecycle writes while leaving the full host lifecycle graph unfrozen.
- Sentinel now renders a concise operator report that keeps lifecycle completion, receipt integrity, producer-owned Sorna meaning, artifacts, and limitations separate.
- Sentinel CI explanations now carry the compact audit status and check identities used by the integrity gate, preserving that decision at the shared Nublar boundary.
- Sentinel audit and operator-report outputs now publish through temporary-file plus rename boundaries, preventing partial report artifacts at their final paths.
- Sentinel audit, operator-report, and CI-result construction now share one validated receipt snapshot, preventing a mutable receipt path from mixing audit and emitted-envelope versions.
- Sentinel’s CI explanation, including its compact audit trace, now has a versioned closed schema under `herdr-sentinel/spec`.
- The host-enabled `nublar-sentinel-run-collect` proof now passes end to end: Sorna freezes the webhook oracle, Sentinel emits the audit-gated CI envelope, and Nublar stores a passed `ingen.nublar-run/v1` result preserving the receipt and audit trace.

## Useful entry points

```sh
make mutation-provider-inspect
make mutation-provider-ci-result
make mutation-campaign-ci-result
make mutation-go-provider-ci-result
make mutation-go-campaign-ci-result
make webhook-alpha
make webhook-mutation-alpha
make webhook-go-mutation-alpha
make nublar-webhook-aggregate-fresh
make nublar-sentinel-run-collect
make sentinel-workspace-validate
make sentinel-run-bootstrap
make sentinel-capability-plan
make sentinel-adapter-oracle-probe
make sentinel-adapter-verifier-probe
make nublar-aggregate
```

The Go source-provider campaign can be run with `make mutation-go-campaign-run`; use fresh output directories when overriding its paths. The resulting campaign can be checked with the matching `make mutation-go-campaign-verify` target.

## Important findings and limitations

- Green implementation tests are weak evidence unless the contract is independently meaningful. The killed mutations are the first concrete signal that the document-pipeline contract is sensitive to important behavioral defects.
- Mutation killing measures contract sensitivity, not contract correctness or completeness. A surviving mutation is useful evidence, but it still needs diagnosis.
- Source and binary hashes establish pre-launch integrity and detect drift. They do not constitute a cryptographic proof that an agent never saw implementation details; that requires stronger execution provenance and environment design.
- The fixture provider is currently usable for local demonstrations without a plan hash binding. The generated Go provider is the strict production-style path; the default Nublar workflow requires its matched plan hash.
- The local fixture workflow remains in optional-binding mode; the default Nublar workflow now selects strict binding through the generated Go provider and executor.
- Provider outputs, evidence outputs, and review outputs intentionally refuse unsafe reuse of existing paths. Campaigns should therefore use fresh artifact directories.
- Workflow result paths are declared in the Nublar YAML. Overriding Make output variables does not automatically rewrite those declarations; custom artifact paths require a matching workflow declaration or a fresh workflow file.
- The workflow declaration now names files relative to the artifact root. A fresh workspace keeps those paths aligned with the workspace-relative Sorna policies.
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

- Re-run the complete Sorna slice from a committed clean checkout and compare both exact and semantic plan identities.
- Review whether the strict Go-provider workflow is the right alpha default before adding other language providers.
- Freeze the Sorna alpha boundary before adding more mutation operators.

### CI and Nublar

- The mutation campaign CI envelope and its initial failure taxonomy are now present, but both remain alpha interfaces.
- The preparation CI envelope is now present; its report and cross-artifact binding rules remain alpha interfaces.
- Keep exercising the shared exit-code contract as new producer kinds are added; blocked preflight, failed behavioral rules, killed mutations, surviving mutations, and infrastructure errors now map through the common envelope.
- The repository now has a clean workflow execution path; decide whether the temporary fresh-workspace runner should eventually become a first-class Nublar workflow command rather than remain a Makefile convenience.

### Sorna hardening

- Move the provider capability vocabulary into a versioned contract/schema rather than keeping it only in code and examples.
- Improve provider build diagnostics further and preserve a reviewable source-diff or mutation-summary artifact.
- Continue tightening TOCTOU handling, immutable inputs, subprocess boundaries, and telemetry guarantees.
- Add negative cases for malformed providers, ambiguous targets, unsupported operators, drift, and partial preparation.

### Mutation depth and examples

- Add operators only when they represent meaningful contract risks, extending the target-resolution and ambiguity rules beyond the document-pipeline proof.
- Expand the document-pipeline lab and later add the webhook and document-processing scenarios discussed earlier.
- Use surviving mutations to guide contract improvements instead of optimizing for a mutation score.

### SDKs and product boundaries

- Keep the provider boundary language-neutral; add TypeScript or other SDKs only after the Go provider contract is stable.
- Define the future Sentinel contract workspace separately from Sorna's execution internals.
- Treat the current Nublar integration as a proof-of-concept envelope and plan its later standalone rewrite.

## Recommended next step

The Nublar first slice now has workflow validation, strict CI-envelope and run
loading, byte-bound collection provenance, immutable filesystem storage,
deterministic run queries with status/workflow/correlation filters, provider-neutral
decision projection, optional provider-neutral external correlation metadata,
generic webhook delivery with independent receipts, and an end-to-end consumer
contract test covering the local collect-to-delivery path.
The next Nublar decision should come from a concrete consumer: either add the
metadata or delivery behavior it requires, or freeze this local contract
before a hosted implementation. Producer execution, hosted storage,
scheduling, and provider-specific delivery remain outside the current Nublar
boundary.

For Sentinel, the next boundary remains concrete Herdr host integration: the
provider-neutral event ingress and operator report are ready to exercise now,
while native plugin binding still waits for Herdr’s actual hook and session API.

This file is a project checkpoint, not a requirement to implement every avenue listed above immediately.
