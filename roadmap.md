# InGen roadmap

Updated: 2026-09-17

## Current position

Amber is treated as the completed foundation for this phase. The active
product boundary is now the Herdr/Sentinel vertical slice around Sorna and
Nublar.

The current flow is:

```text
Herdr host lifecycle
        ↓  (native binding: not yet available)
Sentinel workspace, receipt, and event boundary
        ↓
Sorna policy, oracle, subject execution, and evidence
        ↓
Shared CI envelope
        ↓
Nublar collection, aggregation, and delivery
```

Ownership is intentionally split:

| Area | Current responsibility | Position |
| --- | --- | --- |
| Amber | Earlier application and substrate foundation | Stable/completed for this phase |
| Sorna | Contract execution, isolation, oracle, evidence, and mutation campaigns | Alpha vertical slice proven |
| Sentinel | Workspace declaration, lifecycle receipt, artifact lineage, adapters, audit, and CI projection | Local alpha boundary proven |
| Nublar | Thin CI consumer, collection, aggregation, and delivery surface | Integration proof; standalone product later |
| Herdr | Host lifecycle, sessions, callbacks, durability, and plugin runtime | Native contract still required |

## What has been completed

### Sorna foundation

- Contract and oracle inputs are validated, canonicalized, hashed, and sealed.
- Subject policies, process isolation, executable/access telemetry, and
  evidence bundles are implemented.
- Mutation plans, provider manifests, provider preparation, campaign
  execution, result verification, and shared CI envelopes are present.
- The document-pipeline and webhook-validation examples have meaningful
  contract-driven baseline, defect, and mutation paths.
- Strict Go source-provider preparation and campaign execution are proven.

### Nublar boundary

- Nublar consumes shared `ingen.ci-result/v1` envelopes rather than parsing
  producer-specific reports.
- Collection, aggregation, filtered run queries, immutable filesystem storage,
  provider-neutral decision projection, webhook delivery, and receipt history
  are covered by local contract tests.
- Fresh-workspace aggregate and Sentinel collection targets prevent stale
  artifacts from satisfying a proof.

### Sentinel boundary

- A versioned workspace manifest validates role declarations and Sorna/Nublar
  references without becoming a verifier.
- A versioned lifecycle receipt records workspace identity, ordered events,
  status, and opaque artifact references.
- A declaration-only capability plan binds exact workspace and policy bytes.
- Separate oracle and verifier adapters delegate enforcement and managed runs to
  Sorna rather than duplicating Sorna policy semantics.
- Receipt updates and delegated lifecycle writes use locked read-modify-publish
  paths. Exact artifact and event retries are idempotent; conflicting reuse is
  rejected.
- Audit, operator reports, CI-result emission, nested explanations, and
  terminal status mapping are implemented with shared receipt snapshots.
- Rooted path handling is consistent across capability loading, bootstrap,
  artifact registration, Herdr ingress, Sorna handoff, audit, and completion
  artifacts. Symlink escapes are rejected and references remain root-relative.
- The provider-neutral `ingen.herdr-event/v1` boundary supports identity,
  monotonic timestamps, artifact checks, batches, retries, and terminal guards.

The explicit Sentinel alpha boundary is frozen in
[ALPHA-INTERFACES.md](ALPHA-INTERFACES.md), currently through invariant 24.

## Current verification baseline

- Focused race suite passes:

  ```sh
  GOCACHE=/private/tmp/ingen-sentinel-go-cache go test -race ./herdr-sentinel/... ./nublar/... ./core/...
  ```

- Fresh positive Sentinel→Sorna→Nublar proof passed with four Sorna rules
  passing and a passed Nublar run. Latest workspace:
  `/private/tmp/ingen-sentinel-workspace.ezgUfA`.
- Fresh expected-failure proof passed with three Sorna rules passing, one
  controlled duplicate-idempotency defect, and the expected Nublar `failed/1`
  decision. Latest workspace:
  `/private/tmp/ingen-sentinel-failure-workspace.n9eDRZ`.
- The external-root proof now completes the positive Sentinel→Sorna→Nublar
  workflow while the caller remains outside the temporary project root;
  bootstrap, oracle/verifier handoff, CI emission, audit, and collection all
  pass.
- The external-root expected-failure proof now carries the controlled
  duplicate-idempotency defect through the same boundary, preserving the
  expected failed Sentinel and Nublar decisions.
- The verified baseline for this checkpoint is commit `b8118cf`; other product
  roadmap and README changes may coexist in the shared worktree and are outside
  this Sentinel roadmap.
- Host-enabled `make alpha-interface-check` passes with exit code 0 across
  Sentinel, Sorna, examples, Nublar, and core, including the Darwin
  process-sampling tests. An unprivileged local sandbox can still
  fail those observations because `/bin/ps` is denied; that is an execution
  permission limitation, not a Sorna timing failure.
- A permission-enabled repository-wide `go test ./...` reaches and passes
  Sentinel, Sorna, Hammond, Lockwood, and Nublar. It remains non-green only at
  the unrelated Paddock acceptance test
  `TestCLICleanJSONReportUsesEmptyFindingsArray`, which times out while building
  its CLI. The restricted environment also cannot run Hammond loopback tests or
  Darwin process inspection reliably.

The detailed Sentinel proof and caveat are maintained in
[notes/modules/sentinel-alpha-interface-checkpoint.md](notes/modules/sentinel-alpha-interface-checkpoint.md).
The invariant-to-test evidence index is maintained in
[ALPHA-INTERFACES.md](ALPHA-INTERFACES.md).

## What is deliberately not implemented

- A production native Herdr binding. Herdr 0.9.0 exposes plugin and socket
  surfaces, but Sentinel-specific event identity, timestamps, replay,
  acknowledgement, persistence, authentication, artifact handoff, and shutdown
  behavior are not yet sufficient for a durable lifecycle translation.
- A complete Herdr lifecycle state machine. Sentinel currently protects the
  important terminal regressions while the host lifecycle graph is unfrozen.
- Sentinel-side enforcement of role `write_roots`. Those are declarations;
  Sorna or a future host capability adapter owns enforcement semantics.
- Independent attestation that a process did not observe protected data.
- Remote event queues, hosted retry retention, callback-origin attestation, or
  durable cross-process receipt storage.
- A standalone Nublar product or additional language providers before the
  current interfaces are stable.

The host dependency is described in
[notes/modules/sentinel-herdr-host-binding-contract.md](notes/modules/sentinel-herdr-host-binding-contract.md).

The local session alignment note confirms this boundary in
[herdr-sentinel/AGENT-ALIGNMENT.md](herdr-sentinel/AGENT-ALIGNMENT.md): the
provider-neutral Sentinel path is ready, and the version-pinned compatibility
probe is receiving real Herdr callbacks. The production binding remains
unavailable until Herdr supplies the durable identity, time, delivery, and
recovery parts of its hook and session contract.

## Possible next steps

### Priority 0 — complete the Herdr host contract for Sentinel

Before native binding work begins, Herdr must provide or document:

1. lifecycle hook names, payloads, ordering, duplication, and timing;
2. stable run, workspace, role, and session identity across retries/restarts;
3. success, rejection, retryable failure, and backoff acknowledgement;
4. durable ownership of event IDs, receipt updates, and restart recovery;
5. artifact reference namespace, byte availability, immutability, and hashes;
6. callback authentication or provenance;
7. cancellation, unload, and shutdown behavior.

The version-pinned compatibility probe now captures the available host surface
without mutating Sentinel receipts. The highest-value remaining step is to
resolve the missing durable identity, timestamp, replay, acknowledgement, and
recovery semantics with the Herdr owner.
Its fixture target, `make sentinel-herdr-probe-fixture`, now validates the raw
envelope and emits machine-readable contract findings, preserving the
distinction between probe-local observation time and host event time.
The checked-in sanitized fixture and structured host-contract status snapshot
make the live finding reviewable, while the adapter regression keeps raw host
envelopes out of the normalized Sentinel ingress.
The read-only `herdr-host-envelope` inspector now gives future binding work an
explicit raw-input seam without allowing a callback to bypass normalized event
validation.
Herdr's documented asynchronous command logs and persistent plugin-owned state
are useful implementation primitives, but they do not close the delivery and
recovery gate.
Use the [host contract handoff format](notes/modules/sentinel-herdr-host-binding-contract.md#host-contract-handoff-format)
to collect the answers from the Herdr owner; an incomplete handoff remains a
blocker for native binding work.

### Priority 1 — implement the native Herdr binding after contract intake

Once the host contract exists, build only a thin translation layer:

```text
real Herdr callback
    → host identity/authentication validation
    → ingen.herdr-event/v1
    → existing Sentinel adapter and receipt boundary
```

The acceptance gate is already defined: valid delivery, idempotent retry,
conflict rejection, wrong-run/workspace rejection, artifact rejection, stale
timestamp protection, terminal protection, atomic batches, restart recovery,
and separate authentication testing. No new behavioral verdict should be
added at this layer.

### Priority 1 — preserve independent verification gates

- Run the alpha checkpoint with the host permission required for macOS process
  inspection, and keep the unprivileged `/bin/ps` failure classified as an
  environment limitation rather than weakening the telemetry assertions.
- Keep the remaining Paddock acceptance timeout separate from the Sentinel
  boundary; a future full-suite run should recheck it after the concurrent
  Paddock changes settle.

These are quality gates, not reasons to expand Sentinel semantics.

### Priority 2 — operationalize the proven local boundary

Possible follow-up work that does not require native Herdr semantics:

- Promote the fresh Sentinel collection proof from a Makefile convenience into
  a first-class workflow command if repeated operators need it.
- Extend the external-root pair with additional blocked or incomplete lifecycle
  cases only if a host-driven consumer needs that negative coverage.
- The local file receipt is sufficient for the current alpha; a durable store
  abstraction is deferred until a host contract or concrete consumer requires
  remote, distributed, replay, or stronger recovery semantics.
- `TestUpdateFileReloadsPersistedReceiptAcrossWriterBoundaries` now proves the
  local reload case under the focused Sentinel race check; native host restart
  recovery remains part of the future Herdr acceptance gate.
- The full focused `./herdr-sentinel/... ./nublar/... ./core/...` race
  checkpoint was re-run after that regression and remains green.
- Review whether the strict Go-provider workflow should remain the Sorna alpha
  default before adding another provider or SDK.

### Priority 3 — later product expansion

- Define hosted event delivery, retention, replay, and cross-process durability.
- Add authenticated artifact storage or independent attestation where the
  product requires stronger assurance.
- Expand Sorna mutation operators only when they represent meaningful contract
  risks and surviving mutations provide actionable feedback.
- Rework Nublar as a standalone product only after the envelope boundary has
  proven stable in real usage.

## Recommended sequence

1. Freeze the current Sentinel alpha surface and avoid adding native-looking
   Herdr APIs without host input.
2. Obtain the Herdr host-binding contract using the checklist above.
3. In parallel, clear the Sorna timing gate and track the Paddock repository
   gate separately.
4. Implement and test the thin native Herdr binding against the existing
   provider-neutral event adapter.
5. Run the native acceptance gate plus fresh positive and expected-failure
   Sorna/Nublar proofs.
6. Only then choose between operational hardening, another provider, expanded
   mutation depth, or Nublar productization.

## Source of truth

- [Herdr Sentinel agent alignment](herdr-sentinel/AGENT-ALIGNMENT.md)
- [Project status](status.md)
- [Sentinel alpha checkpoint](notes/modules/sentinel-alpha-interface-checkpoint.md)
- [Native Herdr host-binding contract](notes/modules/sentinel-herdr-host-binding-contract.md)
- [Herdr 0.9.0 compatibility boundary](notes/modules/sentinel-herdr-v0-9-compatibility.md)
- [Cross-boundary alpha invariants](ALPHA-INTERFACES.md)
- [Sentinel receipt](notes/modules/sentinel-run-receipt.md)
- [Sentinel/Sorna adapter](notes/modules/sentinel-sorna-adapter.md)
- [Sentinel/Nublar workflow](notes/modules/nublar-sentinel-verifier-workflow.md)
