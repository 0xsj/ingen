# InGen ecosystem completion plan

Prepared 2026-10-03. Target: the full nine-module ecosystem with native Herdr integration, as requested by the owner. This is the release execution plan; existing module specifications remain authoritative for their artifact semantics.

## Product intent

InGen should let a developer commission agent-written software and obtain reviewable evidence that it satisfies an independently approved contract. The ecosystem covers the entire chain: intent, approval, isolated oracle creation, implementation, architecture checks, behavioral verification, mutation challenge, provenance, custody, CI delivery, and comparison over time.

The inferred primary user is a developer operating agent sessions locally and consuming the same verification artifacts in CI. The first complete product should support a small stateful HTTP/JSON service, native Herdr role sessions, and a repeatable workflow outside the InGen checkout. All nine modules participate in that workflow. Separate hosted services are later deployment choices.

Completion means a developer can install the tools, run the documented workflow on a fresh project, inspect successes and failures, recover an interrupted session, and independently verify the resulting artifacts without knowledge of InGen internals.

## Evidence available at resumption

| Observation | Evidence | Status at initial review |
| --- | --- | --- |
| Last committed checkpoint | `5f2cfb8`, 2026-09-17, branch `dev` | Commit calls itself v1.0 alpha; README still describes v0.9.2 |
| Pending implementation | Sentinel scaffolding, preflight, sessions, public package handoffs, `.malc` migration | Substantial uncommitted work; preserve and review it |
| Focused pending-work checks | Tests for Sentinel project, preflight, and session packages | Passed locally during resumption review |
| Document workflow binding | Contract version 3; mutation catalogue contract version 2 | `make mutation-catalogue-validate` fails locally |
| General CI coverage | Three root workflows, with release/manual/path-filtered triggers | Full mainline gate needs implementation |
| Sandbox portability | `sandbox_unsupported.go` rejects non-Darwin enforcement | Linux enforcement needs implementation and proof |
| Session capabilities | Local session provider explicitly declares no sandbox | Native role enforcement needs implementation and proof |
| Herdr baseline | Installed CLI reports 0.9.3; repository probe describes 0.9.0 | Reassess current host capabilities before using old blockers |
| Cross-module integration | Several local and offline fixture proofs already exist | Full live nine-module workflow still needs an acceptance run |

The repository-wide suite and full integration workflows have not been rerun during this planning pass. September test results are historical evidence, not current release approval.

## Release scope and completion criteria

The release includes the existing module surfaces plus the integration needed to operate them together. Proposed platform coverage is macOS for the native Herdr workflow and macOS/Linux for standalone verification and CI. Confirm the precise versions and architectures during host discovery. Unsupported combinations must fail with actionable diagnostics before a run starts.

The release is complete when all of these conditions hold:

1. A fresh install provides versioned Malcolm, Sorna, Sentinel, Hammond, Paddock, Lockwood, Nublar, and Sattler commands, plus usable Amber Go and TypeScript packages and the Herdr integration.
2. A project brief becomes a Malcolm specification and exact sealed contract. Hammond authorizes that exact digest before oracle generation or implementation starts.
3. Herdr launches distinct role sessions with stable identities, declared inputs, and enforced capabilities. Contract and oracle roles cannot access implementation roots, transcripts, tools, or artifacts outside their allowed scope. Implementation roles cannot alter approval, oracle, or evidence artifacts.
4. Sorna creates the frozen oracle from the approved contract and permitted fixtures. The current deterministic oracle remains the supported generation path; learned or creative test generation is a separately reviewed extension.
5. A clean implementation passes mandatory behavioral rules and the selected Paddock architecture policy. Each required producer remains authoritative for its own result.
6. Mutation campaigns detect relevant wrong behavior. All six existing document mutations remain killed; survivors on new projects are recorded and diagnosed rather than hidden.
7. Amber records work, execution, retry, and handoff identities. Lockwood preserves exact contracts, approvals, policies, runs, evidence, and result bytes with digest relationships and verifiable custody.
8. Nublar consumes required producer envelopes, persists its decision, and supports the selected GitHub Checks delivery path. Missing, blocked, malformed, or failed required checks cannot produce a successful delivery gate.
9. Sattler compares two real workflow runs and traces changed rules, mutations, architecture results, governance references, provenance, and custody without rewriting producer verdicts or inferring causation.
10. Cancellation, duplicate callbacks, host restart, missing artifacts, and tampering leave explicit recoverable outcomes. Repeated runs use fresh outputs and cannot pass from stale artifacts.
11. Automatic CI protects every module and the supported workflow/platform matrix. Installation, compatibility, recovery, and assurance limits are documented and exercised.

Passing verification establishes sensitivity to the declared behavior and evidence integrity. Assurance labels must distinguish enforced access restrictions, observed telemetry, and independent attestation. Existing model knowledge or prior exposure cannot be disproved by filesystem isolation.

## Module acceptance map

| Module | Remaining integration work | Required release proof |
| --- | --- | --- |
| Malcolm | Finish `.malc` migration; package compiler; define supported lowering and diagnostics | Compile fresh-project source; reject unsupported semantics; preserve rule and contract identities through Sorna |
| Hammond | Bind live workflow transitions to approved bytes and active review policy | Missing, stale, rejected, superseded, or wrong-digest approval prevents the relevant transition; an amendment requires a new review and oracle |
| Sentinel | Finish fresh-project path; add native provider, lifecycle transitions, recovery, and operator commands | Real Herdr sessions complete the workflow; denied capability access, conflicting events, and interrupted handoffs remain explicit |
| Sorna | Repair binding drift; finish Linux enforcement; expose stable verification and mutation invocation | Clean baseline passes; six current mutations are killed; denied oracle access and tampered evidence fail on supported platforms |
| Paddock | Make selected architecture policy a required workflow producer | Allowed dependencies pass; a deliberate boundary violation fails and remains visible in Nublar and Sattler |
| Amber | Attach SDK provenance to coordinator/worker handoffs and subject boundary where declared | Work/execution identities and retry relationships survive the real workflow; Go/TypeScript conformance stays green |
| Lockwood | Replace fixture-only cross-module proof with live intake and a retrieval path | Preserved bytes verify; tampered/missing lineage fails; intact failed producer results can still be stored without changing their verdict |
| Nublar | Collect the actual required producers; enforce failure propagation and delivery receipts | Behavioral, architecture, governance, lifecycle, and custody failures block success; selected delivery persists a separate receipt |
| Sattler | Consume actual run pairs; stabilize the Amber comparison fields used by this workflow | A deliberate change is traced to producer evidence with stable IDs; incompatible or incomplete inputs remain explicit |

Use existing artifact contracts and generic intake where sufficient. Any new envelope needs a consumer, a producer owner, compatibility rules, and a failure proof. Keep integration logic in Sentinel/adapters; Nublar remains a result consumer.

## Milestone 1 Restore a reliable development baseline

Preserve the dirty worktree and inventory pending changes by responsibility before editing. Review the Sentinel additions and `.malc` rename as separate changes; check all tracked and untracked files needed for a clean checkout.

Reconcile the document mutation catalogue with contract v3 after validating all six mutations and rule bindings. Resolve the README/release version discrepancy. Rerun the full suites and classify actual code failures separately from unavailable host permissions.

Add automatic push and pull-request CI for root Go packages, Amber's separate Go module and TypeScript package, Malcolm Rust tests, schemas, and existing fresh workflow gates. Run platform-specific tests on matching runners. Inventory Linux-sensitive tests before splitting jobs; unsupported sandbox behavior needs explicit testing rather than blanket exclusions. Preserve producer artifacts for diagnostics while ensuring failures propagate to the required gate.

Exit: a clean checkout builds and tests, `make nublar-aggregate-fresh` passes all four current required checks and kills all six mutations, and reintroducing the version mismatch makes CI fail.

Existing checks to compose include `make build check`, `make alpha-interface-check`, `make nublar-aggregate-fresh`, `make -C amber release-check`, and `cargo test --manifest-path malcolm/Cargo.toml`.

## Milestone 2 Resolve native host and capability contracts

Inventory Herdr 0.9.3's bundled API/schema and current source/documentation if available. Re-run a version-pinned probe against a disposable test session and compare it with the 0.9.0 fixture. This is the first critical dependency and starts immediately after baseline triage.

Specify host/session identity, event IDs and time, ordering, acknowledgements, retries, durable event ownership, restart replay, callback authentication, artifact publication, cancellation, plugin unload, and cleanup. Determine what Herdr guarantees and what the integration must durably own. Plugin-owned IDs are valid only when publication and replay guarantees support them; payload hashes and local observation times must not be presented as host identities or event times.

Define how each agent obtains filesystem access, tools, credentials, network access, context, transcripts, and fixtures. A pane or worktree alone does not enforce those boundaries. Decide whether the host or an explicit launch wrapper enforces each capability, and record gaps before launch.

If host changes are required, produce a concrete Herdr patch request with protocol fixtures and acceptance tests. Locate the source checkout and establish the available write scope before editing another repository. Missing host guarantees remain explicit release dependencies while independent module work continues.

Exit: a pinned supported Herdr contract and capability matrix, executable positive/negative fixtures, and a documented owner for every durability and enforcement responsibility.

## Milestone 3 Complete native Sentinel sessions and isolation

Implement a thin Herdr provider using the existing provider-neutral event and receipt boundary. Support workspace/worktree setup, role launch, exact artifact attachment, lifecycle collection, cancellation, cleanup, and recovery. Reuse the locked receipt update path; preserve host provenance separately from structural validation.

Enforce the approved-contract and frozen-oracle transitions before implementation and verification. Launch fresh agent contexts for independent roles and keep forbidden context out of prompts, tools, logs, and mounts. Capture capability violations without converting them into a successful run.

Add Linux enforcement behind Sorna's existing platform seams. Prove allowed reads, denied implementation access, write restrictions, network/process behavior, cleanup, and truthful telemetry limits. Select the mechanism after an environment feasibility check; missing enforcement must fail closed rather than receive an isolated assurance label.

Exit: the documented native acceptance cases pass against the real host, including duplicate delivery, conflicting identity, wrong run/workspace, bad artifacts, stale callbacks, atomic batches, cancellation, host restart, and authentication. Oracle/implementation capability denial probes pass on supported platforms.

## Milestone 4 Connect all nine modules in one fresh workflow

Use a new project root and a reviewed stateful HTTP/JSON contract. Connect live module outputs in order:

1. Malcolm compiles the specification; Sorna validates and seals the contract.
2. Hammond records the exact approval and policy/authority references.
3. Sentinel launches the isolated oracle role, records the freeze, then launches implementation roles through Herdr.
4. Paddock evaluates the declared architecture policy. Sorna evaluates the public behavior and runs mutations against disposable variants.
5. Amber records work/execution/handoff provenance. Lockwood ingests the artifacts and verifies their exact digest relationships.
6. Producer-owned results enter Nublar's required workflow and persistent run history. Delivery remains a separate recorded action.
7. A second run introduces a deliberate change. Sattler compares the actual runs and supplies artifact references for investigation.

Add a repeatable operator entry point using existing Sentinel commands/adapters. State transitions should validate prerequisite artifacts rather than infer completion from agent exit or pane output. Human review must approve a concrete digest; automation must not manufacture approval to complete a demo.

Exit: every module contributes a real inspectable artifact, the clean run succeeds, and deliberate behavioral, architecture, governance, custody, and lifecycle failures are preserved through comparison and CI.

## Milestone 5 Prove adoption and recovery

Exercise the same installed tools on a fresh external project without source-tree-relative assumptions or hidden local stores. Use one settled InGen public boundary, such as `core/ciresult`, as an additional subject: expose it through the supported adapter, author its contract from published semantics, freeze independently, and challenge it with meaningful mutations.

Run an adversarial acceptance set covering contract edits after approval, oracle edits after freeze, forbidden reads/writes, fixture escapes, missing required producers, byte drift, delivery retries, session interruption, callback replay, and restart recovery. Keep each proof tied to a real risk or release invariant. Record survivors and observation gaps as findings.

Exit: a second operator can complete the external-project guide, verify/replay preserved evidence where supported, and recover an interrupted run without manually rewriting receipts. Self-verification produces a reviewable mutation result.

## Milestone 6 Package and release the ecosystem

Provide consistent installation and version reporting for all commands, the Herdr adapter, and Amber packages. Produce verified archives/packages, checksum manifests, source revision metadata, and an artifact compatibility matrix. Confirm license coverage and distribution metadata. Test installation from the actual release artifacts on clean supported environments.

Consolidate the user path into the root README, one getting-started guide, one operational/recovery guide, and linked normative module specifications. Replace stale status claims with current evidence; retain useful design and learning notes without making them required onboarding.

Add one required ecosystem release gate composed of module checks, fresh live workflows, host recovery proofs, capability probes, negative cases, and install checks. Proposed target names such as `ecosystem-check` and `ecosystem-acceptance-fresh` must be implemented and documented before they are advertised.

Exit: the gate passes from a clean checkout and installed artifacts, all required failures are demonstrated, and a release candidate has a reviewable compatibility/assurance report. Public tags, GitHub Releases, npm publication, and authenticated external delivery are separate release-owner actions after artifacts are ready.

## Sequence and release evidence

### Reviewed implementation batch on 2026-10-03

Three Luna agents implemented separate changes, with parent review and follow-up revisions before acceptance:

- Rebound the document mutation catalogue to contract v3, retained all six mutation IDs/rule bindings, and added a regression guard invoking Sorna validation against the real repository inputs.
- Added automatic push/PR CI for root Go and the fresh document workflow on macOS, plus Amber Go/TypeScript and Malcolm Rust on Linux. The manual Nublar producer now propagates failure while downstream collection still runs.
- Hardened fresh-project paths against escaping/dangling symlinks and scaffold overwrites. Session output reservations reject collisions/overwrites, and rejected terminal transitions clean up reservations without launching the child. Preflight no longer reports a passing policy binding after policy load failure.
- Removed two needless Rust lifetimes so the required Clippy gate passes, and replaced the stale README release label with the current alpha status.

| Current check | Result |
| --- | --- |
| Root Go build and vet | Passed |
| Root `go test -timeout 4m ./...` with host permissions | Passed; Paddock acceptance also passed |
| Focused race checks for scaffold, preflight, sessions, and mutation catalogue | Passed |
| Malcolm formatting, Clippy with warnings denied, and Cargo tests | Passed; 51 unit tests |
| Amber `make check` with workspace caches and required network/host access | Passed, including Go/TypeScript and package/consumer checks |
| `make nublar-aggregate-fresh` with host permissions | Passed; all four required checks passed; six mutations killed and none survived |
| Exact CI entry point `make nublar-run-collect-fresh` | Passed; six mutations killed; persisted Nublar record matches the collected run |
| Deliberately stale v2 catalogue against v3 contract | Rejected as expected with exit 1 |
| Hosted GitHub Actions | Not triggered in this batch; local workflow validation is not a hosted execution result |

The aggregate workflow artifacts are in `/private/tmp/ingen-workspace.tS16fi/.artifacts`; its command log is `/private/tmp/ingen-first-batch-document.log`. The exact CI workflow artifacts are in `/private/tmp/ingen-nublar-workspace.R2rNs1/.artifacts`, with log `/private/tmp/ingen-first-batch-ci-workflow.log`. These are local temporary evidence, not release distribution artifacts. Existing uncommitted work remains preserved and no commit or publication has been made. Milestone 1 still requires a clean-checkout/hosted-CI checkpoint before it is marked complete.

### Current Herdr assessment

The installed Herdr 0.9.3 bundled schema still reports protocol 22/schema version 1. Both event envelopes require only `event` and `data`; the agent-status event has workspace/pane identity and status, with no host event ID or event timestamp. Subscription parameters expose subscriptions without a durable cursor.

The current [official socket documentation](https://herdr.dev/docs/socket-api/#event-subscriptions) explicitly describes non-durable history, `events_lost`, and reconciliation through a new subscription plus authoritative snapshot. That recovery restores current state rather than missing history. This narrows the remaining integration question: durable lifecycle evidence must come from a reviewed host guarantee or an explicitly owned Sentinel launch journal, with raw UI events retained as observations. Do not manufacture host identities or mark snapshot reconciliation as historical replay. No new live callback or restart proof was performed in this batch.

The dependency path is baseline restoration, current Herdr contract, native sessions and enforcement, live ecosystem integration, adoption/recovery proof, then packaging and release. Governance, architecture, provenance, custody, comparison, and Linux feasibility work can progress independently once the baseline and required artifact handoffs are defined.

Maintain one release evidence index in this document as work proceeds. Each milestone needs its relevant source revision, exact commands, supported environment, artifact locations/digests, outcomes, and remaining gaps. Mark completion only when its exit criteria are demonstrated.

No calendar estimate is reliable until the current Herdr contract and pending-work baseline are evaluated. Estimate the remaining work after milestones 1 and 2; native host guarantees and capability enforcement are the largest uncertainties.

The first implementation batch is: inventory pending changes, repair the v3 mutation binding, establish current full-suite results, add mainline CI, and refresh the Herdr host-contract assessment. This plan defines a bounded completion target for the ecosystem. Hosted registries, remote retention platforms, additional languages/transports, and independent attestation require separate requirements.

## Source documents

- [Agent operating guide](../AGENT-GUIDE.md)
- [Existing roadmap](../roadmap.md)
- [External review](../review.md)
- [Alpha interface invariants](../ALPHA-INTERFACES.md)
- [Isolation threat model](../ISOLATION-THREAT-MODEL.md)
- [Native Herdr contract and acceptance cases](../notes/modules/sentinel-herdr-host-binding-contract.md)
- [Fresh project acceptance guide](../herdr-sentinel/FRESH-PROJECT-GUIDE.md)
- [Sorna readiness](../sorna/ALPHA-READINESS.md)
- [Cross-module custody handoff](../lockwood/CROSS-MODULE-EVIDENCE-SPEC.md)
