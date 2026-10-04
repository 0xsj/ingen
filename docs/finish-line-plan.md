# InGen ecosystem completion plan

Prepared 2026-10-03. Target: the full nine-module ecosystem with native Herdr integration, as requested by the owner. This is the release execution plan; existing module specifications remain authoritative for their artifact semantics.

## Product intent

InGen should let a developer commission agent-written software and obtain reviewable evidence that it satisfies an independently approved contract. The ecosystem covers the entire chain: intent, approval, isolated oracle creation, implementation, architecture checks, behavioral verification, mutation challenge, provenance, custody, CI delivery, and comparison over time.

The inferred primary user is a developer operating agent sessions locally and consuming the same verification artifacts in CI. The first complete product should support a small stateful HTTP/JSON service, native Herdr role sessions, and a repeatable workflow outside the InGen checkout. All nine modules participate in that workflow. Separate hosted services are later deployment choices.

Completion means a developer can install the tools, run the documented workflow on a fresh project, inspect successes and failures, recover an interrupted session, and independently verify the resulting artifacts without knowledge of InGen internals.

## Current checkpoint — 2026-10-04

The local macOS nine-module HTTP workflow passes, with exact custody retrieval,
a killed generated mutation, explicit negative cases, and separate clean/failed
Nublar decisions. The reviewed producer changes remain uncommitted over
`84c1070`. See the evidence tables below for commands, hashes, and local proof
locations. Hosted CI has not run on these changes.

Native contained role execution and evidence handoffs have separate live Herdr
proofs. The HTTP fixture uses synthetic approval and an unsealed architecture
policy; the complete operator-reviewed native workflow still needs proof.

The typed Codex CLI launch boundary, wrapper leases, local UNIX peer checks,
and contained cancellation now have reviewed code and offline/live local
proofs. Both native cancellation modes retain their contained reports; repeated
collection is byte-identical and owned workspace cleanup preserves existing
workspaces and focus. The Codex executable in these proofs is explicitly an
offline interface fixture, not a model runtime.

The scoped API-key broker now has reviewed code, synthetic containment/custody
proof, and installed-CLI wire compatibility with a mock upstream. The installed
Codex 0.160.0 fails contained startup when macOS preferences synchronization is
denied. Keep that runtime compatibility gate open; the broker fixture is not a
real contained Codex run. The owner deferred real-provider testing.

The runtime/installation batch now provides an offline contained diagnostic, a verified local
nine-module bundle, installed HTTP handoffs outside the checkout, and bounded
Linux capability reports. Installed Codex remains unsupported under the current
policy. Both Linux container configurations report Landlock unavailable; these
observations do not implement enforcement. See the runtime/installation evidence
table below.

The distribution batch adds owner-approved MIT licensing, common machine-readable
version commands, selected build-input fingerprints, verified private source
snapshots, and standalone archive verification/installation. Native contained
broker launches now gate startup after read-only role/prompt validation and
before project writes or Herdr dispatch. The combined archive/workflow proof
is recorded in the distribution evidence section below.

The authenticated handoff batch adds operator-signed file ingress with atomic
receipt updates, bounded GitHub Checks reconciliation exercised with actual
workflow decisions against a local mock, and offline archive handling in an
unprivileged Linux container. These establish the local protocol and archive
boundaries. Herdr does not yet publish the signed batches, the GitHub service
has not accepted this workflow, and the Linux archive proof executes no native
tools.

The recovery and runtime review batch adds a read-only journal assessment,
bounded comparison of explicitly selected Codex executables, and a conservative
local release-evidence auditor. Installed acceptance verifies unchanged project
bytes and modes, repeated reports, capture drift, and receipt mismatch. The
auditor binds the installed callback and delivery binaries to the verified
bundle and keeps eight external release gates open. Codex 0.160.0 still fails
managed preferences synchronization under the existing policy, with zero broker
requests. See the recovery and runtime review evidence below.

Next: select a compatible contained Codex runtime or review its managed
preferences dependency, obtain a Linux environment with the required enforcement
interfaces, and implement the Linux backend. Signed host callbacks, host
restart/replay, current live GitHub Checks delivery, signed distribution,
clean-machine native installation, operator-reviewed governance, and hosted CI
remain release gates. Real-provider testing remains explicitly deferred.

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

### Hosted baseline follow-up on 2026-10-04

The owner committed the reviewed batch as `fab65dc` (v1.0.2). [Hosted CI run 37113746224](https://github.com/0xsj/ingen/actions/runs/37113746224) passed the Linux Amber/Malcolm job but failed the macOS root Go job in `TestCLIComponentMapCommand`: the acceptance test required `../overwatch/overwatch-backend`, which is absent on a clean runner. The fresh document workflow was not reached. Local success with a sibling checkout had concealed this dependency.

The repair uses the checked-in modular-monolith Go fixture to exercise context variant identities and retains checked-in TypeScript coverage for lock-backed mapping. Hosted green status remains pending until the repaired revision runs in CI. The downloaded failing job log is `/private/tmp/ingen-hosted-ci-job.log`.

### Reviewed native lifecycle batch on 2026-10-04

Three Luna agents implemented the journal, socket client, and provider/CLI in separate owned files. Parent and sibling review produced follow-up fixes for dispatch/claim/start races, terminal journal immutability, receipt and manifest revalidation, collection digest races, cancellation settlement, and truthful child exit/capture outcomes. No source commit or publication was made in this batch.

The opt-in `session spawn --provider herdr` path now launches noninteractive commands in new background workspaces through Herdr 0.9.3/protocol 22. A persistent Sentinel binary dispatches a wrapper; the original child argv remains in the journal rather than shell text. New `native-status`, `native-collect`, `native-cancel`, and `native-recover` commands operate on the exact recorded identities. The default local provider remains available. Operator instructions are in the [fresh-project guide](../herdr-sentinel/FRESH-PROJECT-GUIDE.md#native-herdr-session-operations).

| Responsibility | Current owner and limit |
| --- | --- |
| Host workspace/pane/terminal identities | Herdr response/snapshot; exact IDs are stored and checked before interruption. Host responses remain observations, not independent attestation. |
| Launch intent and event IDs/times | Sentinel-owned `ingen.sentinel-native-session/v1`; immutable intent, rooted files, advisory locking, atomic publication, file/directory sync, strict decoding and replay. Times are Sentinel's monotonic journal chronology, not host event times. |
| Child launch | Separate claim and start CAS gates permit at most one launch. A crash can leave an unexecuted claim; recovery never redispatches it automatically. |
| Cancellation | Durable intent blocks an unclaimed launch; a no-claim journal can settle without process evidence. A running wrapper monitors the intent and forwards signals to its own child process group. Failed/uncertain host interruption remains explicit. |
| Process outcome and artifact attachment | Wrapper records actual child exit and synced output hashes; capture failure leaves an indeterminate outcome. Collection verifies identity and exact bytes and replays without changing the receipt. |
| Role access and workflow prerequisites | Still declaration-only/unverified. No enforced filesystem/tool/network boundaries, interactive agent terminal capture, approval/oracle-freeze gates, authenticated callbacks, or host-restart proof are claimed. |

| Verification | Result and evidence |
| --- | --- |
| Paddock map acceptance without a sibling checkout | Passed in `/private/tmp/ingen-paddock-isolated.xAHlMb`; targeted `TestCLIComponentMapCommand` |
| Full root Go suite and vet in an isolated source snapshot | Passed in `/private/tmp/ingen-native-clean-go.o6d72P`, using `git archive fab65dc` plus current Sentinel/Paddock overlays and Git metadata copied from the checkout for provenance tests. Commands: `go test -timeout 10m ./...`, `go vet ./...`. Logs: `/private/tmp/ingen-native-clean-go.log`, `/private/tmp/ingen-native-clean-vet.log`. |
| Final Sentinel source after cancellation refinements | `go test -race -timeout 2m ./herdr-sentinel/...`, `go vet ./herdr-sentinel/...`, and CLI build passed with workspace caches and required local socket/process permissions. |
| Fake-host and durable journal failure cases | Passed: conflicting/duplicate events, strict JSON/replay, path escapes, claim/start cancellation races, late acknowledgement byte stability, missing executable, manifest tamper, output tamper, unknown host delivery, recovery without redispatch, and wrong terminal identity. |
| Live final-binary success | `native-e6755eda807a367fd8d2629c37c384eb`: `/bin/echo final-reviewed-success`, completed with exit 0 |
| Live final-binary failure | `native-826ecb9770626824338148aeb6ba13b4`: `/bin/sh -c 'exit 7'`, failed with exit 7 |
| Live final-binary cancellation | `native-c45a586d8ca50cc75f0444edb9583d68`: `/bin/sleep 120`, canceled through Sentinel, actual signal exit -1 and interruption reason preserved |
| Collection/replay and duplicate launch | All three live sessions collected twice successfully, including replay into failed receipts. Duplicate completed-wrapper execution was rejected with exit 1. |
| Host cleanup | Only owned test workspaces were closed. Before/after snapshots retained the original eight workspace IDs and focused pane `w23:p1`; no server stop/restart was performed. |

Final live artifacts and receipts are under `/private/tmp/ingen-native-proof.Qa5bqN/.ingen/artifacts/`; journals are in `native-sessions/` and the three final receipts are `rc-success.json`, `rc-failure.json`, and `rc-cancel.json`. The tested binary is `/private/tmp/ingen-native-sentinel-rc`, SHA-256 `b01bd59620ee3533e866621c740db4bfc98cdbad11ca5e2800e6637b7585c90e`. Earlier pilot records in the same root are separate from these final proofs. Temporary artifacts are local review evidence, not release packages.

Milestone 1 still awaits green hosted CI on the repaired revision. Milestones 2–3 have a pinned host surface and reviewed launch mechanics, but their complete host/capability/restart/authentication criteria remain open. Next work is enforced role isolation and approval/freeze transitions, followed by the live nine-module workflow; this batch does not mark the ecosystem release complete.

### Governed and contained execution batch on 2026-10-04

Three Luna agents implemented the public Sorna boundary, rooted Hammond/workflow gates, and contained Sentinel executor; the parent reviewed interfaces, protection rules, actual outcomes, and live integration. All changes remain uncommitted on top of `fab65dc`, including the previous native lifecycle and Paddock repair. No publication or real operator approval was created.

New behavior:

- Hammond exposes a read-only approved-contract verifier that reuses its existing policy and lifecycle replay. Contract, active/referenced policy, and nested authority bytes are hash-bound and read beneath an explicit project root. Missing, rejected, superseded, wrong-identity, stale-policy, and escaped/tampered artifacts fail.
- Sentinel's workflow gate seals draft source using rooted fixture bytes or canonicalizes an already sealed source. Implementation, verifier, and mutation roles additionally require a frozen oracle matching the approved contract and current oracle policy. Selected artifacts are checked again by the role wrapper before launch.
- `session spawn --isolate` selects a noninteractive macOS Seatbelt child boundary with exact manifest grants, fresh private scratch, sanitized environment, disabled networking, and prelaunch executable/tool hashes. Write grants remain independent of read grants. Protected control paths and in-project symlink aliases cannot be exposed by role capabilities. Reports and policy publication use rooted files, exclusive execution claims, and atomic report publication.
- `--governed --approval <path> --review-policy <path>` enables the prerequisite checks for contained sessions and oracle freeze. Bootstrap authoring remains separate. Sorna freeze/generate receive expected canonical hashes to refuse contract drift before generation.
- Sorna no longer grants blanket reads beside command executables. Explicit tool binaries get literal execution/read grants. Unsupported platform enforcement still fails closed.

The separate `ingen.sentinel-role-execution/v1` report records the contained command's policy, exact inputs, captures, actual exit, and enforcement limits. The unchanged native journal records the Sentinel wrapper's outcome and remains declaration-only/unverified. Role reports also remain unverified: these filesystem/tool/network restrictions do not isolate the process namespace or prior context, do not attest loaded images or every tool invocation, and do not establish independent host attestation. The local authority snapshot gate does not perform organization authentication or signature trust-store verification. Direct development commands without `--governed` retain their existing behavior.

| Verification | Result and evidence |
| --- | --- |
| Full root Go suite and vet | `go test -timeout 10m ./...` and `go vet ./...` passed with workspace caches and required host permissions, including the final project-root grant rejection. Final logs: `/private/tmp/ingen-governed-final-tests.log`, `/private/tmp/ingen-governed-final-vet.log`; earlier checkpoint logs are preserved separately. Environment: Darwin/arm64. |
| Race checks | Sentinel packages, Hammond governance, and Sorna sandbox passed; final roleexec/CLI/schema race suite passed after the native argv repair. |
| Actual Seatbelt probes | Allowed write and selected tool succeeded; unlisted source/sibling reads, outside writes, unlisted tools, and TCP were denied. Role tests also verified a synthetic parent token was absent from the child environment. |
| Governed test fixtures | Exact-pinned launch passed; contract/approval/authority/oracle drift, missing oracle pins, and write access to selected custom governance paths failed before child launch. Test fixtures are not operator approval. |
| Execution and cleanup regressions | Duplicate execution claims, project-root read/write grants, escaping/in-project symlink aliases, and executable/tool deny-root overlap were rejected. Cancellation retained graceful exit 0 as canceled; a signal-ignoring child was killed after the grace period, and same-group descendants stopped while a separate test-owned group remained running. |
| Fresh document workflow | `make nublar-run-collect-fresh` passed; all four required checks passed; six mutations killed, zero survivors/errors. Run `run-4c1aa15d01084f3989dd0c3701a5bd6f`, artifacts/store in `/private/tmp/ingen-nublar-workspace.Bwd2Hs/.artifacts`; log `/private/tmp/ingen-governed-fresh-workflow.log`. |
| Native contained success | `native-3e97e18d04a47fbcec75c4dcdc28ce4f`, role execution `648dcefac573d534c0c930c256b14400`: `/bin/echo contained-native-success`; command and wrapper exited 0. |
| Native contained failure | `native-127dcf60ac87916ae5332b39b7e82a54`, role execution `93902d38daf841ea79cad633914ac5a8`: `/bin/bash -c 'exit 7'`; command exit 7 and wrapper exit 1 preserved separately. |
| Native contained cancellation | `native-5fd37fa81964842f5025661ad7e712c1`, role execution `c156c7c23b53851e1c520983f0152fd3`: canceled running `/bin/sleep 600`; role report canceled with actual signal exit -1, native wrapper canceled with exit 1 and interruption reason. |
| Final binary confirmation | `native-bec8a0488054bfd717299fa806898f07`: `/bin/echo release-contained-success` completed with exit 0 after the project-root grant fix; collected twice. |
| Replay and host cleanup | Success/failure/cancellation journals collected twice. Only owned settled workspaces `w2B`–`w2G` were closed; the eight original workspace IDs remained. Initial lifecycle cleanup preserved focus `w23:p1`; after final confirmation the observed focus was `w1X:p1`. No focus commands or server restart were issued; the cause of that focus change was not established. |

Native artifacts are under `/private/tmp/ingen-contained-proof.NwrOd4/.ingen/artifacts/`, with lifecycle receipts `final-success.json`, `final-failure.json`, and `rc-canceled.json`. Those three proofs used `/private/tmp/ingen-contained-sentinel-final`, SHA-256 `9e682d3c3f581fcfbf383d382b81d8aabf8664921c5ba7f7718c773ee6fa23e0`. The final grant refinement was confirmed using `/private/tmp/ingen-contained-sentinel-release`, SHA-256 `2023411027c02ffd68225c0067eaf5a586ce0fd5f4b19377090a7788cf15a2e7`, with receipt `release-success.json`. A first pilot caught a missing Sentinel executable in the contained argv; it recorded a setup failure without child evidence, was collected, and prompted the repair and argv regression. Its binary and journal are preserved separately. A separate 120-second sleep completed normally before cancellation was requested; it is not the cancellation proof above.

The next release batch is the live nine-module workflow and required producer/custody intake for these new reports. Complete native agent context isolation, Linux enforcement, host restart/authentication proofs, packaging, and hosted CI on the repaired revision remain open. This batch does not mark the full ecosystem release complete.

### Role evidence handoff batch on 2026-10-04

The owner committed the previous reviewed work in `84c1070` (v1.0.4). This
batch is uncommitted on top of that clean baseline. Three Luna agents owned
the producer verifier, native/CLI integration, and custody adapter; the parent
reviewed the boundaries, fixed acceptance expectations, and ran the integration
checks. No real operator approval, commit, or publication was created.

The new public `herdr-sentinel/evidence` boundary verifies exact role report,
manifest, sealed policy, and capture bytes beneath an explicit root. Governed
reports also bind source/fixture bytes, canonical contract and policy identities,
Hammond approval/authority snapshots, and the approval record identity. Reports
and returned file snapshots cannot be changed before CI construction or custody
intake. The separate role CI kind/explanation retains `unverified` assurance;
passing describes execution and evidence integrity. Behavioral correctness stays
with Sorna. A v1 report has a frozen oracle digest but no oracle path, so standalone
report verification cannot retrieve it; native collection rechecks the original
governed argv's oracle path.

`sentinel role verify` writes a shared producer envelope with decision codes
0/1/2. `native-collect` requires inner evidence for a started role wrapper and
attaches it in one receipt update. Existing native collection events keep their
original identities; contained evidence has a separate deterministic event.
Repeated collection rechecks bytes and does not rewrite the receipt. A started
canceled wrapper missing its report fails closed. A canceled or indeterminate
command with actual exit 0 cannot make its wrapper return success.

Lockwood's new `import-role-execution` stores exact producer bytes and every
located related file with existing `references` lineage. Its immutable producer
snapshot check rejects constructed or modified verification results. Damaged
parent blobs fail custody verification. Accepted failed/canceled evidence keeps
its producer outcome, and partial publication errors identify accepted records
for recovery. The normal in-project custody directory is supported while store
overlap with selected inputs is rejected.

| Verification | Result and evidence |
| --- | --- |
| Full reviewed root Go suite and vet | `go test -timeout 10m ./...` and `go vet ./...`; logs `/private/tmp/ingen-role-evidence-reviewed-tests.log` and `/private/tmp/ingen-role-evidence-reviewed-vet.log`. Darwin/arm64, workspace caches, required host permissions. |
| Race checks | Sentinel packages and changed Lockwood adapter/CLI passed; `/private/tmp/ingen-role-evidence-final-race.log`. The final approval record identity comparison subsequently passed its focused regression and the full suite. |
| Fresh execution evidence handoff | `make sentinel-role-evidence-check` passed against completed producer/custody code. Actual echo exit 0 and bash exit 7 became passing/failed required Nublar checks; Sattler retained that transition. Exact custody retrieval matched the original report, source capture damage blocked verification/intake, and custody retained intact original bytes. Root `/private/tmp/ingen-role-evidence.LjViIs`; log `/private/tmp/ingen-role-evidence-release-acceptance.log`. |
| Existing native receipt upgrade | Earlier success/failure/cancellation receipts under `/private/tmp/ingen-contained-proof.NwrOd4` gained four contained artifacts each. Original base events were unchanged; a second collection was byte-identical. Pre-upgrade receipt backups end in `.before-role-evidence`; summary `/private/tmp/ingen-role-native-upgrade.json`. |
| New native success | `native-a83b1e87cb05f0ba64b0a52a516451b5`, role `b130b0c6fd4f9cbfd67427307b139876`: echo with a literal child `--` argument, role/wrapper exit 0, CI passed. |
| New native failure | `native-d656cdbaec44d03bcdbdd6f8aa693310`, role `14e26fe6106b75920380f3649364fdb7`: bash exit 7, role/wrapper exit 7, CI failed with shared exit 1. |
| New native cancellation | `native-9248687aae780558b4d3f87ea285e722`, role `a3aa72e7884df59e240a93b8cbc6888b`: cancellation after the child emitted its readiness marker; graceful actual role exit 0 preserved, wrapper exit 1, CI error with shared exit 2. |
| Native replay/cleanup | Each new session collected twice with byte-identical second receipts. Only owned settled workspaces `w2H`, `w2J`, and `w2K` were closed. All eight original workspace IDs remained; focused workspace `w23` remained. No focus or server restart command was issued. |

New native evidence is under `/private/tmp/ingen-role-native-proof.aotqztmn`,
with `summary.json` and before/after host snapshots. Log:
`/private/tmp/ingen-role-native-acceptance.log`. Its persistent binary is
`/private/tmp/ingen-role-evidence-sentinel`, SHA-256
`1e8f2ffc19f1ce10542c52695f68ca1e7d1dc8508bef7c5de90046303c0d8bd1`.
These live runs preceded the final approval record identity check and CLI usage
text addition; those final changes were verified through the full suite/vet
and CLI build. Temporary evidence is local review material, not a release package.

[Hosted baseline CI run 37190965155](https://github.com/0xsj/ingen/actions/runs/37190965155)
passed Amber/Malcolm on Linux and build/vet on macOS, then failed
`TestAccessCaptureCanMissShortLivedTransitionBetweenSamples`: a scheduler-dependent
test expected the sampler to miss an executable that it observed. The repaired
test controls readiness and the before/after sample points. Ten runs with race
detection passed; it also passed in the full suite. Failing hosted log:
`/private/tmp/ingen-ci-84c1070-failed.log`. Hosted green status awaits the next
committed revision. CI now includes the fresh role evidence target and uploads
its artifacts separately.

The next integration batch is the nine-module governed HTTP workflow, with
actual Amber provenance, architecture/governance/behavioral/mutation producers,
complete custody handoffs, Nublar collection/delivery, and Sattler run comparison.
Fresh agent context isolation, Linux enforcement, host restart/authentication
proofs, packaging, and hosted CI remain release requirements. This batch completes
the role evidence handoff, not the full ecosystem release.

### Nine-module HTTP integration batch on 2026-10-04

This batch remains uncommitted over `84c1070`. Luna agents implemented the
producer commands and fresh acceptance workflow; the parent reviewed the
handoffs and ran the final checks. The repeatable macOS command is
`make ecosystem-http-check`. CI now runs this gate and uploads its local
evidence. Hosted green status requires a committed revision.

The workflow compiles Malcolm's document specification, verifies an explicitly
synthetic Hammond approval, freezes the governed Sorna oracle, and scans the
same HTTP subject with Paddock. A real Go provider prepares and executes one
HTTP response mutation. Sentinel uses Amber's public SDK to record coordinator
root/child contexts around clean and defective managed HTTP verification.
Nublar requires governance, architecture, behavior, mutation, provider review,
preparation review, and selected custody results. It persists separate clean
and failed decisions and delivers the failed decision to a local HTTP receiver.

`hammond gate` verifies an existing approval against the explicit selected
contract and current review policy. It rechecks returned artifact bytes and
publishes an exclusive shared CI envelope. It creates no approval.
`lockwood ci-result` requires explicitly selected custody IDs, verifies exact
record/blob bytes and accepted lineage, and publishes a separate shared CI
envelope outside the store. A failed producer result can have valid custody;
its original outcome stays intact.

The custody fixture stores contract/governance inputs, policies, frozen oracle,
raw Amber contexts, coordinator receipts/captures, baseline and mutant evidence,
the complete prepared mutation source tree and binary, producer CI results,
immutable Nublar runs/decisions, delivery receipts, and the observed webhook
body. It reproduces the provider's full prepared-source hash. Identical bytes
share content identity, so copied project state receives no reverse preparation
edges that would create content-lineage cycles. Bundle/index associations remain
explicit where the existing Sorna importer offers no lineage flags. The final
custody selection includes the original required records and every extra phase.

Review fixed two consumer/host issues exposed by actual execution: Sorna's
`localhost` campaign endpoint now binds the subject on literal `127.0.0.1`
while retaining the client URL; Sattler accepts Go's valid signal exit sentinel
`-1` and preserves it independently of the contract verdict. Sattler compares
the CI, Sorna, Nublar, and custody transitions. Distinct Amber child work IDs
remain an explicit provenance incompatibility in its overall result.

The synthetic governance fixture and unsealed Paddock test policy establish
acceptance wiring only. Coordinator execution is `not-isolated`/`unverified`;
HTTP context propagation, fresh native agent context, running-image attestation,
and independent host attestation are not established. Subject/oracle enforcement
uses the existing macOS Sorna boundary. This HTTP gate performs no new live
Herdr acceptance or remote delivery proof.

| Final verification | Result and evidence |
| --- | --- |
| Root Go suite and vet | `go test -timeout 10m ./...` and `go vet ./...` passed on Darwin/arm64 with workspace caches; logs `/private/tmp/ingen-ecosystem-final-tests.log` and `/private/tmp/ingen-ecosystem-final-vet.log`. |
| Changed package race checks | `go test -race -count=1 ./herdr-sentinel/internal/provenance ./hammond/cmd/hammond ./lockwood/cmd/lockwood ./sorna/cmd/sorna ./sattler` passed; `/private/tmp/ingen-ecosystem-final-race.log`. |
| Independent fresh HTTP acceptance | `make ecosystem-http-check` passed with required macOS process/socket permissions; `/private/tmp/ingen-ecosystem-final-acceptance.log`. Proof root `/private/tmp/ingen-ecosystem-http.FHfEeZ`. |
| Architecture and generated mutation | Actual subject scan: 2 packages, 18 dependency edges. Provider/preparation reviews passed; 1 planned mutation, 1 killed, no survivors/errors/inconclusive results. |
| Decision and delivery | Both Nublar runs require 7 checks. Clean run/decision passed with shared exit 0; defective run/decision failed with shared exit 1. The loopback webhook accepted that failed decision and its exact body matched the immutable decision bytes. |
| Complete custody | All 116 explicitly selected records verified. Parent blob damage produced custody failed/1; original bytes were restored after preserving the negative proof. |
| Exact custody retrieval | Parent retrieved the canonical contract and both behavioral envelopes byte-identically, and retrieved every prepared-source file to reproduce the producer's complete source-tree hash. Command `python3 /private/tmp/ingen-ecosystem-custody-retrieve-review.py /private/tmp/ingen-ecosystem-http.FHfEeZ`; result `/private/tmp/ingen-ecosystem-final-custody-retrieval.json`. |
| Other negative cases | Missing Hammond approval error/2; forbidden architecture edge failed/1; missing mandatory Nublar mutation input error/2. Actual exit codes and emitted result fields both asserted. |
| Comparison | CI/Sorna/Nublar/custody subsystem comparisons compatible; behavior and Nublar transitions pass to fail. Overall Sattler comparison remains incompatible solely for the two distinct Amber work IDs. |

The proof root's `acceptance-index.json` records exact producer artifacts,
canonical contract and frozen oracle identities, CLI binary hashes, outcomes,
counts, and assurance limits. Its SHA-256 is
`7cf79346d47215d4f9e865f1bc71f193ed954aaf21101b1efba4d34b9a96d7de`.
Canonical contract SHA-256:
`2de6546837414a358578cc7146600f51ce65f29aa11571d30b7130c5b407e4d1`.
Frozen oracle SHA-256:
`ec52b723657fb5c74890024ad429610463da02a2ae940f0d3821838de6d43a88`.
Persistent Sentinel binary SHA-256:
`33e6204fdb1a015cb2416bfaa2a9682c24b7b1e282c79b7d6efe8d4cd454c8ce`.

Native context isolation, Linux enforcement, host restart/authentication
recovery, packaging, and hosted CI remain release requirements. This batch
completes the local nine-module HTTP handoff; it does not establish the full
ecosystem release criteria.

### Fresh Codex boundary and wrapper recovery batch on 2026-10-04

The owner selected Codex CLI as the first typed runtime. Luna agents implemented
the launch, recovery, socket, and acceptance changes; the parent reviewed them
and ran the combined verification. Changes remain uncommitted over `84c1070`.

`session spawn --isolate --agent codex --agent-executable ABS --prompt-file REL`
generates a fixed ephemeral exec command, rejects arbitrary trailing arguments
and resume/fork options, pins a bounded UTF-8 prompt, supplies its exact bytes on
stdin, and publishes a separate prompt snapshot. An optional model token is
explicit. Each execution has unique private HOME, CODEX_HOME, and XDG state
outside the role workspace grants. Role evidence verification and native
collection bind both prompt files and the context to the execution and immutable
wrapper arguments; Lockwood preserves the prompt bytes for exact retrieval.

New native journals have optional execution leases. A busy lease prevents
recovery from changing an active journal. A claimed wrapper with a free lease
becomes indeterminate with no inferred exit or capture hashes, and repeated
recovery preserves exact bytes. Legacy journals remain readable. A real owned
wrapper-crash regression verifies kernel lease release and natural bounded child
exit without guessing or signaling a child PID. Native process groups are
cleaned before output hashing; uncertain cleanup produces an indeterminate
outcome. Generic interrupt grace stays at two seconds; strictly bound contained
role wrappers get five seconds so the inner runner can finish its two-second
escalation and publish evidence. Review caught and corrected a mistaken
comparison between the contained child's pin and Sentinel's pin.

Herdr transport checks canonical nonsymlink UNIX socket paths, socket ownership,
and connected peer credentials before sending request bytes. This is a local
same-effective-UID boundary, not Herdr application or callback authentication.
`native-process-info` and `native-snapshot` are read-only observations.
`native-close` requires a settled journal and verifies the exact stored host
binding before closing its workspace with group closure disabled.

| Verification | Evidence |
| --- | --- |
| Final full Go suite and vet | `go test -timeout 10m ./...` and `go vet ./...` passed with required macOS permissions; `/private/tmp/ingen-codex-context-final-tests.log` and `/private/tmp/ingen-codex-context-final-vet.log`. |
| Changed launch/recovery/custody race checks | Passed; `/private/tmp/ingen-codex-context-reviewed-race.log`. Includes Sentinel launch, peer transport, native journal/session, evidence, CLI, schemas, and Lockwood role adapter/CLI. |
| Offline acceptance | `make sentinel-native-context-check` passed; proof `/private/tmp/ingen-native-context.pqvGjT`, log `/private/tmp/ingen-codex-context-offline-acceptance.log`. Exact argv/stdin, private local state, seeded prior-context denial, filtered credentials, explicit network denial against a reachable loopback control, verified CI, custody intake, and byte-identical prompt retrieval. |
| Actual wrapper disappearance | Acceptance reruns native-session tests with `-count=1 -v`; `TestRecoverAfterWrapperCrashReleasesExecutionLease` launches a real wrapper, kills only that owned process, observes bounded child natural exit, and verifies indeterminate/repeated recovery without redispatch or invented process evidence. See each proof's `native-session-tests.log`. |
| Live native acceptance | `bash acceptance/native-context-recovery.sh --live --socket /Users/sj/.config/herdr/herdr.sock` passed inside the existing `HERDR_ENV=1` terminal; proof `/private/tmp/ingen-native-context.Zplcoz`, log `/private/tmp/ingen-codex-context-live-acceptance.log`. |
| Graceful cancellation | Owned workspace `w2N`, pane `w2N:p1`; contained outcome canceled, actual child exit 0, native role wrapper exit 1. Report SHA-256 `daf34eb6cafe8c4637a46a27431c27e8e4853da8988119cab75f202f13fac6fa`. |
| Ignored-interrupt cancellation | Owned workspace `w2P`, pane `w2P:p1`; contained outcome canceled, actual child exit -1, native role wrapper exit 1. Report SHA-256 `7d24e5152f242fd83657a9551a9f97bf6c175c579221d41df6adbe6041d03ac6`. |
| Collection and host preservation | Both contained reports collected; parent repeated collection and compared receipt bytes unchanged. Both owned workspaces closed. The nine original workspace IDs and focus `w1Z` / `w1Z:t1` / `w1Z:p1` matched before/after. Details in `root-review.json` under the live proof. |

Live acceptance index SHA-256:
`0852f8e803b413d45fed4e3e80d1b039e08d380e7912d86dd80eea027074de73`.
The macOS CI workflow now runs the offline gate and uploads its proof directory;
hosted CI has not run these changes. Linux peer support was cross-compiled;
this batch does not claim Linux enforcement or runtime acceptance.

All Codex-shaped executions here use a declared offline compatible fixture.
No real Codex inference, model account, provider retention/context attestation,
authenticated host callbacks, or host restart/replay was exercised. Networking
and inherited provider credentials remain disabled. The remaining native gate
is a scoped auth/network path for real Codex work, followed by the complete
operator-reviewed workflow and host recovery requirements.

### Scoped Codex broker batch on 2026-10-04

The owner selected an OpenAI API key held by a local broker and explicitly
deferred the real-provider smoke test. Luna agents implemented the broker,
launcher, evidence checks, and synthetic acceptance; the parent reviewed the
code and schemas and ran combined checks. All changes remain uncommitted over
`84c1070`.

The opt-in `openai-broker` launch requires an explicit model and bounded
request/output/deadline limits. Sentinel keeps the selected environment
credential in its trusted process and passes the child a random token. The
broker binds IPv4 loopback, forwards only Responses requests to the fixed
OpenAI endpoint, requests `store:false`, strips local client metadata, rejects
hosted tools and stored conversation/file references, blocks redirects and
ambient proxies, sanitizes upstream errors, and settles admitted handlers
before recording closure. Public reports include configuration and counters,
with strict matching to the sealed policy and immutable native wrapper flags.
They establish local request observations, not provider inference or retention.

Review found that Darwin Seatbelt rejects literal IP host syntax. The actual
policy declares its supported `localhost` selector at the pinned port; the
broker HTTP endpoint remains `127.0.0.1`. Real socket probes measured IPv4
loopback, IPv6 loopback, and an assigned nonloopback local address at that port
as reachable, with another reachable port denied. Reports require the exact
local-address scope limitation. Sorna now rejects unsupported literal-IP host
rules before launch instead of silently broadening their permission.

| Verification | Evidence |
| --- | --- |
| Full Go regression and vet | Passed with required macOS process/socket permissions; `/private/tmp/ingen-codex-broker-full-tests.log` and `/private/tmp/ingen-codex-broker-vet.log`. |
| Changed package race checks | Passed for broker, launcher, role execution, public evidence, native sessions, CLI, schemas, Sorna sandbox, and Lockwood role adapter; `/private/tmp/ingen-codex-broker-race.log`. Final broker metadata/schema changes additionally have focused checks below. |
| Final broker/schema checks | `go test -race -count=1 ./herdr-sentinel/internal/codexbroker ./herdr-sentinel/spec` and focused vet passed after the final metadata and schema changes; `/private/tmp/ingen-codex-broker-final-focused.log`. |
| Synthetic containment/custody acceptance | `make sentinel-codex-broker-check` passed at `/private/tmp/ingen-codex-broker.h4VCcW`. The driver with its published evidence index also passed at `/private/tmp/ingen-codex-broker.luzU4B`; index SHA-256 `69f66d89741dbaf37531e85d1fd87538e8a0667834ea852ef57a62bdd78989b4`. |
| Broker negative probes | Exactly four local rejections: wrong model 400, wrong bearer 401, hosted tool 400, wrong route 404. Received/rejected 4; forwarded/upstream failures/in-flight 0. Separate host-prevalidated loopback port explicitly denied with EPERM/EACCES. |
| Public verification and custody | Role CI passed, Lockwood verified the record, and report/prompt retrieval was byte-identical. Report SHA-256 `e94d557a76b3a59bbc442d9e44f1d966a9bc949dcbbb82e59344ffafb284e07b`; exact referenced digests and binary identities are in the index. Synthetic credential and scoped token values were absent from persisted project artifacts. |
| Installed CLI wire compatibility | `INGEN_CODEX_INTEGRATION_BINARY=/opt/homebrew/Caskroom/codex/0.160.0/bin/codex go test -race ./herdr-sentinel/internal/codexbroker -run '^TestInstalledCodexBrokerProtocol$' -count=1 -v` passed; `/private/tmp/ingen-codex-broker-installed-wire.log`. One synthetic Responses stream completed through the production broker and a mock transport. This CLI probe runs outside Sorna and establishes no containment assurance. |
| Installed CLI contained startup | `INGEN_CODEX_CONTAINED_INTEGRATION_BINARY=/opt/homebrew/Caskroom/codex/0.160.0/bin/codex go test ./herdr-sentinel/internal/codexbroker -run '^TestInstalledCodexContainedBrokerProtocol$' -count=1 -v` failed at macOS managed preferences initialization before a turn; `/private/tmp/ingen-codex-broker-installed-contained.log`. This release gate remains open. |

Installed `codex-cli 0.160.0` binary SHA-256:
`112fae7a5a1223e673c8a1791d32338f37df8b527ff1159bb8adac6c4dbf1b4b`.
Its managed preferences dependency is separate from private CODEX_HOME and
`--ignore-user-config`; see the [official loader source](https://github.com/openai/codex/blob/main/codex-rs/config/src/loader/macos.rs).
The wrapper keeps host preference services inaccessible. A compatible runtime
or a reviewed mechanism for forced managed configuration is needed before
claiming real contained Codex support. No real credential, real model inference,
live Herdr mutation, or host restart was exercised in this batch. Hosted CI is
wired for the synthetic gate but has not run these changes.

### Runtime diagnostics and local installation batch on 2026-10-04

Luna agents implemented the offline Codex diagnostic, local bundle builder and
verifier/installer, and Linux capability diagnostic. The parent reviewed them,
added installed-tool selection to Sentinel's workflow commands, published strict
diagnostic schemas, and ran the combined proofs. Changes remain uncommitted over
`84c1070`; bundle metadata explicitly records the dirty source state.

`sentinel agent diagnose` uses private disposable state, fixed Codex arguments,
mandatory Sorna containment, the production broker with an in-process synthetic
transport, and a filtered environment. It publishes bounded metadata instead
of raw child output. A supported result requires one mock round trip, a
structured completed CLI reply, observed zero exit, complete output capture,
and settled broker closure. Unsupported and indeterminate outcomes remain
distinct. The installed runtime's result is an explicit incompatibility, with
no provider traffic or inference claim.

The local bundle contains eight module commands and the Sorna Malcolm bridge,
plus Amber Go source and the TypeScript npm tarball: nine modules, nine binary
files, 38 files overall. It records hashes, modes, target, toolchains, source
revision/dirty state, and build settings. Verification rejects unsafe paths,
duplicates, unknown artifacts, wrong targets, symlinks, and tampering.
Installation requires a fresh/empty prefix and verifies its final bytes; tests
also reject a changed manifest during copying. No system installation or
publication occurred. This is a local review bundle: CLI license coverage,
consistent version surfaces, archives/signing, independent reproducibility,
and clean-machine/platform coverage still need completion.

`--tool-dir` selects installed binaries for Sentinel contract creation, oracle
freeze, subject verification, and evidence checking/gating, with no compiler or
checkout fallback. It preserves declared policy grants. The acceptance fixture
declares the installed Sorna executable in its oracle policy before sealing.

| Verification | Evidence |
| --- | --- |
| Full Go regression and vet | Passed; `/private/tmp/ingen-runtime-install-tests.log` and `/private/tmp/ingen-runtime-install-vet.log`. |
| Changed package race checks | Passed for agent probe, Sentinel CLI/schemas, Linux platform probe/CLI, and Sorna schemas; `/private/tmp/ingen-runtime-install-race.log`. Includes real child-group cancellation with a descendant holding the output pipe. |
| Packaging negative checks | `make ingen-package-check` passed, six tests covering exact installation, byte tampering, pinned manifest mismatch, traversal, wrong target, duplicate keys, symlinks, nonempty prefix preservation, and manifest-copy drift. |
| Final bundle/install/workflow | `make ingen-install-check` passed; `/private/tmp/ingen-local-install.DjgtQs`, command log `/private/tmp/ingen-runtime-install-acceptance.log`. Native Darwin/arm64 bundle and installed prefix independently verified. |
| Installed HTTP handoffs | `/private/tmp/ingen-installed-tools.CsRmVx`: installed Malcolm compilation, bridge translation, contract validation/sealing, contained oracle freeze, managed HTTP verification, evidence verify/gate, and Nublar pass. Commands ran from an unrelated directory with Go/Cargo absent from PATH. Acceptance index SHA-256 `c425f697d4fadff59289298069d1e55a19faec4952c471f7ec3bbc72a48a18ed`. No approval or native session claim. |
| Amber installed consumers | Agent's Go consumer and offline TypeScript package smoke passed against `/private/tmp/ingen-local-tools-20261004-1`. Parent compared every SDK file digest with the final bundle/install; SDK bytes are identical. This does not establish hosted SDK distribution. |
| Installed Codex diagnostic | `/private/tmp/ingen-readiness-sentinel agent diagnose --agent-executable /opt/homebrew/Caskroom/codex/0.160.0/bin/codex --timeout-seconds 30` returned exit 1 and `unsupported`, reason `managed-preferences-unavailable-under-policy`. Report `/private/tmp/ingen-codex-readiness.json`, SHA-256 `d16f8fee710287b2b8cdbe37aca937eba76d0583d300848549b2b80b6b49f384`. Child started/exited 1; capture complete; broker closed with all request counters zero. Public schema validation passed. |
| Linux capability proof | Final Linux/arm64 binary and two reports under `/private/tmp/ingen-linux-platform.3IJ70b`. Kernel `6.10.14-linuxkit`; Landlock query returned ENOSYS both with Docker's default filter and with it removed. NNP and allow-all seccomp attachment on an owned child thread, plus procfs self-executable observation, were available. Both reports passed their published schema. |
| Parent review | `/private/tmp/ingen-local-install.DjgtQs/root-review.json`, SHA-256 `b9eb4bb1020ecca5556cd9d2fb6b1959f8a42c69941c49ad9bf5fe3728c6f13f`: all bundle/installed and indexed workflow hashes checked, SDK digest equivalence, diagnostic results, logs, and pinned Linux probe identity. |

Final bundle manifest SHA-256:
`7fd7b2d8c06744be1b4744b983c6a2123f61b995a9c432ca04dbbbd7772a040a`.
The final Linux probe binary SHA-256 is
`2814b396118d76c36150b0600441b6785527922e9666c80665e5d09f992e7ff1`.
Both disposable containers used the existing local Alpine image, UID 65534,
no network, a read-only filesystem, all capabilities dropped, no-new-privileges,
and only the read-only probe binary mount. No host engine changes or unrelated
container mutation occurred. An unavailable interface may reflect kernel
configuration or another enclosing boundary; this diagnostic does not identify
the cause. TCP Landlock observations concern ports, not remote-host allowlists.
See the authoritative [Landlock](https://docs.kernel.org/userspace-api/landlock.html)
and [seccomp](https://docs.kernel.org/userspace-api/seccomp_filter.html) documentation.
No Linux enforcement, arbitrary-process observation, or TSYNC correctness is
claimed. Linux containment still fails closed.

The CI definition now includes package validation, the macOS installed workflow,
and Linux capability reports/schema validation. YAML parsing and local gates
passed; hosted CI has not run these changes. Real credentials/provider calls,
live Herdr mutation, operator governance, and host restart were not exercised.

### Distribution and native readiness batch on 2026-10-04

The owner selected **MIT across the ecosystem**. The root license preserves
Amber's existing MIT notice; dependencies and compiler components retain their
own licenses. Bundles now contain root/Amber licenses and selected Go, YAML,
and Rust notices, with an exact license inventory. This is not a complete
platform license review.

All eight module commands and the Sorna Malcolm bridge expose `--version` and
`version --format json`, using the published `ingen.tool-version/v1` schema.
Metadata includes the compiled platform, toolchain, selected release version,
revision, dirty state, and source-input digest. Legacy Sorna/Paddock version
injection remains supported; the bundle also injects their artifact-producing
version variables. Unstamped development builds preserve unknown source
identity rather than substituting a dirty marker for a digest.

The `ingen.local-toolchain-bundle/v2` builder inventories selected sources,
embedded assets, locks, and distribution/license files, copies their exact
bytes into a verified private snapshot, builds from it, and rejects input
drift. The fingerprint identifies those selected inputs, including uncommitted
work; it does not attest the whole checkout, caches, compiler images, or
running executables. The final local build is labeled `0.0.0-local-review`.

Archive export normalizes ordering, timestamps, modes, and ownership. The
standalone Python helper verifies and installs without executing bundled
commands or needing Go/Cargo/Node/a checkout. Its own authenticity and the
manifest digest must come from a separately trusted channel. Archive checks
bound compression, expansion, member count, and extension headers; reject
unsafe paths, duplicate paths, links, sparse/special files, unsafe modes, and
nonzero trailing payload; and drain both outer and nested gzip streams to
validate checksums. Installation verifies a private bounded archive snapshot
and the final prefix. Cleanup preserves a competing output it did not create.
See [distribution instructions](../packaging/README.md).

Native isolated Codex sessions using `openai-broker` now run the production
synthetic readiness diagnostic after read-only role/prompt validation and
before policy persistence, journals, or Herdr dispatch. It binds the selected
executable digest before/after diagnosis and to the prepared role. Unsupported
and indeterminate results return distinct exits and safe metadata on stderr.
This check is advisory for local startup/mock protocol compatibility; actual
role execution still requires its own policy and evidence. It does not resolve
the installed runtime's managed-preferences incompatibility.

| Verification | Evidence |
| --- | --- |
| Full Go regression and vet | Passed: 103 packages with tests, 24 without tests; `/private/tmp/ingen-distribution-tests.log` and `/private/tmp/ingen-distribution-vet.log`. Local socket tests required execution outside the workspace sandbox. |
| Rust formatting/tests | Passed: 46 library and seven CLI tests; `/private/tmp/ingen-distribution-rust.log`. |
| Native preflight failures | Actual `/usr/bin/false` diagnostic plus indeterminate and invalid-prompt fixtures verify zero Herdr connections, unchanged project tree/receipt, and no role/native artifacts. Sentinel CLI, agentprobe, roleexec, and public schemas passed race tests and vet; fallback indeterminate schema preserves absent process/executable evidence. |
| Packaging failures | All 18 tests passed; `/private/tmp/ingen-distribution-packaging.log`. Includes changed source snapshot, publication race, traversal/tampering, duplicate paths, link/sparse/special/mode rejection, extension bounds, nested expansion, trailing data, and gzip checksum regressions. |
| Final archive/install/version gate | `make ingen-install-check` passed; `/private/tmp/ingen-local-install.zkysrT`, log `/private/tmp/ingen-distribution-acceptance.log`. Native Darwin/arm64, 625 selected source files, 57 artifacts, 17 license/notice entries, nine version reports matching the manifest and public schema. Standalone helper ran from an unrelated directory with compilers absent from PATH. |
| Installed HTTP handoffs | `/private/tmp/ingen-installed-tools.TgXLtH`: installed Malcolm, bridge, contract sealing, contained oracle, subject verification, evidence verify/gate, and Nublar pass. Acceptance index SHA-256 `7583893cfc94ac4a315ef3ac625403df0191adf34df8a57e4ae41f63e77da66c`. |
| SDK continuity | All 29 SDK artifact digests match the previous bundle whose Go/TypeScript consumer smokes passed. No hosted SDK distribution claim. |
| Parent review | `/private/tmp/ingen-local-install.zkysrT/root-review.json`, SHA-256 `2b4a0d2ab1932c69bc814ae2d9893b97cac674c58df7b1384bedfce7d48d6ce6`: verified bundle, archive, installed prefix, current selected source inventory, all indexed workflow hashes, report metadata, SDK equivalence, and byte-identical repeated archive export. |

Final identities:

- Manifest: `5c65021ac3fa077a5edfed06cda1e88fa82f05bd403dc7044d95d0f94b5e9174`.
- Selected source inputs: `4411c47561c1f04fdfd095cc02d25300dae64f2b1760c43852d4486f04881339`.
- Archive: `7f33e4cb4a5815774020759fdec0d5ca2ee8b4e32090a678f1f6c983279ca323`.
- Standalone helper: `68a343b92c8f8265726163313fa5a1c0a9bb3435e8e100f6d847c4fcd7ae9b0d`.

The final build recorded Go 1.27.1, Rust/Cargo 1.86.0, Node 22.21.1, npm
11.20.0, and TypeScript 5.9.3. Installation used Python 3.12.5 on the same
Darwin/arm64 host. CI now runs this archive/version/workflow gate and installs
the Rust documentation component needed for notice files. YAML and shell
syntax validation passed; hosted CI remains unrun. Repeated archive export
proves archive stability, not independent cross-host compilation
reproducibility. Clean-machine installation, signatures/publication, Linux
enforcement, compatible contained Codex, and operator-reviewed native workflow
remain open. No provider call, live Herdr mutation, approval, commit, or
publication occurred in this batch.

### Authenticated handoffs and archive adoption batch on 2026-10-04

Sentinel now offers `adapter herdr-sign-events` and `herdr-auth-events` for
operator-signed JSONL file batches. The closed public envelope binds the exact
payload, sender, key identifier, run/workspace identity, and bounded validity
window with HMAC-SHA-256. Parsing rejects duplicate, unknown, noncanonical-case,
and null fields. The private raw 32-byte key must be outside the project and
owned by the effective user with mode 0400 or 0600. Ingestion checks the whole
batch before one locked receipt update, rechecks expiry and live identity inside
the lock, and preserves byte-identical replay. This proves shared-key possession;
the current Herdr host does not publish signed envelopes. Legacy unsigned
ingress remains unauthenticated. See the [fresh-project guide](../herdr-sentinel/FRESH-PROJECT-GUIDE.md#signed-offline-callback-batches).

Nublar's GitHub Checks transport follows bounded same-origin pagination with
`filter=all`, checks list completeness and unique identity, and validates the
acknowledged check ID, commit, status, and conclusion. Creates are single-shot;
an ambiguous result triggers lookup and an update of a uniquely matching check,
rather than a repeated create. Redirects are disabled even on supplied clients;
requests, responses, retries, and total duration are bounded. The configured
window covers at most 1000 matching results. Upstream visibility/retention and
concurrent publishers prevent a durable exactly-once guarantee. See GitHub's
[Checks API](https://docs.github.com/en/rest/checks/runs).

The archive acceptance now also runs in a disposable Linux container using an
already-present pinned image, no network, UID 65534, read-only root, dropped
capabilities, no-new-privileges, resource limits, and an owned noexec temporary
filesystem. Isolated standard-library Python verifies separately selected
helper/archive/manifest hashes, installs the archive, compares all 59 files
byte-for-byte, and rejects a tampered copy without leaving installed files.
No native Darwin program is executed. The filtered helper PATH contains no
compiler/package manager; this does not attest their absence elsewhere in the
image. The new hosted Linux job exercises archive handling with filtered Python
without claiming the local container's isolation restrictions.

| Verification | Evidence |
| --- | --- |
| Full Go regression and vet | Passed; `/private/tmp/ingen-handoffs-tests.log` and `/private/tmp/ingen-handoffs-vet.log`. Focused callback/CLI and Nublar race and vet checks also passed. |
| Packaging regressions and CI definition | All 18 packaging tests passed; `/private/tmp/ingen-handoffs-packaging.log`. Shell syntax, Python compilation, and CI YAML parsing passed. Hosted CI remains unrun. |
| Final archive/install/version/HTTP gate | `make ingen-install-check` passed; `/private/tmp/ingen-local-install.hbpDHg`, log `/private/tmp/ingen-handoffs-install-final.log`. Darwin/arm64, 632 selected input files, nine modules/version reports, 57 artifacts, 17 license entries. Installed HTTP proof `/private/tmp/ingen-installed-tools.3mccPG`, index SHA-256 `f286162396e8656ba95606ea2fee428db4df13c6a9f289e14a75154e3ce8726b`. |
| Fresh nine-module workflow | `make ecosystem-http-check` passed; `/private/tmp/ingen-ecosystem-http.GsLbO8`, index SHA-256 `2675d7660c22209440a4758b68276252f546913862d944105530ef9132c75e0c`. Includes synthetic approval/unsealed architecture fixture, clean/failed decisions, killed mutation, custody, and comparison. |
| Installed authenticated ingress | `bash acceptance/callback-auth.sh --bin /private/tmp/ingen-local-install.hbpDHg/install/bin/sentinel` passed. Proof `/private/tmp/ingen-callback-auth.8bLJin/project`, index SHA-256 `c48a558c351edc46fbd736a1ae9fe7539f155e5bf0a167492a040d09d336c7f2`. Valid append, byte-stable replay, wrong sender, altered payload/MAC, conflicting replay, and expiry verified. Produced envelope passed its public Go schema gate. Ephemeral key removed. |
| Installed delivery of actual decisions | `bash acceptance/github-checks.sh --bin /private/tmp/ingen-local-install.hbpDHg/install/bin/nublar --ecosystem-proof /private/tmp/ingen-ecosystem-http.GsLbO8` passed. Proof `/private/tmp/ingen-github-checks.s2zlpmrj`, index SHA-256 `8b2930efcf52cfbb616a4ee4616f22d7d3e87502eaf866c5336b0ebe5fee9178`. Thirteen sanitized local requests cover two-page lookup, committed create with 503 response, reconciliation/update to the same ID, failure conclusion, and permission denial. Source store and producer evidence unchanged; no GitHub service contacted. |
| Offline Linux archive handling | `acceptance/archive-cleanroom.sh` with the final archive and explicit identities below passed; `/private/tmp/ingen-handoffs-cleanroom-final.log`. Existing local image `sha256:10ca2cfc3a29b70e13fe0a2a9244fe7e5d24fbd7350ac4205028335c9541f926`; 59 files compared, no native execution. |
| Parent review | `/private/tmp/ingen-local-install.hbpDHg/root-review.json`, SHA-256 `e3327cddfc4eb685a9e732bcacc822e55008069525ffcd2d63c87ec02e396265`: bundle/archive/prefix independently verified, selected current sources matched, 67 indexed references and 39 installed workflow files checked, all 29 SDK digests unchanged from the consumer-tested bundle. Harness and log hashes retained. |

Final identities:

- Manifest: `af402ff15eba8acf91644df0e1ad84306d37c65733b816c8d15d33d6542710d6`.
- Selected source inputs: `1906e7db2583c130a89e22655c1fb9a9a2ad333bcc6ceac55008bbbfd0773c9e`.
- Archive: `75717a51374a94f97af28e29e5add95141ff046f2e78a15acff0a02a1f23ddd7`.
- Standalone helper: `68a343b92c8f8265726163313fa5a1c0a9bb3435e8e100f6d847c4fcd7ae9b0d`.

CI now preserves trusted archive inputs and runs installed callback, actual
decision delivery to a synthetic API, and Linux archive-handling checks. This
batch remains uncommitted over `84c1070`, with no live Herdr mutation, host
restart, operator approval, real-provider call, GitHub service call, signature,
or publication. Compatible contained Codex, Linux enforcement, signed host
callbacks, native governance/recovery, live delivery, hosted CI, and clean
native platform installation remain release gates.

### Recovery and runtime review batch on 2026-10-04

`sentinel session native-assess --root ABS --path REL --format json` reads
bounded journal, receipt, workspace, and terminal evidence snapshots. It binds
the observed bytes and reports uncertainty on drift, mismatch, missing evidence,
or a changing journal. It does not contact Herdr, signal, relaunch, or write
project files. Existing nonterminal leases are `not-probed`: briefly acquiring
one could interfere with a wrapper claiming execution. Terminal lease probes
are transient observations, and a free lease does not prove child liveness.
Exit 0 means assessed, 1 uncertain, and 2 invalid request/read failure.

`sentinel agent compare --agent-executable ABS` accepts one to four explicitly
selected canonical executables, fingerprints all candidates before starting,
and runs the existing contained synthetic diagnostic sequentially with a
bounded timeout. It checks executable bytes again after each probe. It never
selects or replaces a runtime. Exit 0 means at least one candidate supported the
synthetic exchange, 1 all were unsupported, and 2 invalid or indeterminate.
These results establish neither real inference nor host attestation. The
[official Codex configuration reference](https://learn.chatgpt.com/docs/config-file/config-reference)
documents managed configuration precedence; it does not establish a supported
bypass for the observed preferences failure.

`python3 packaging/release_evidence.py audit --inventory ABS --output FRESH_ABS`
audits a closed, bounded local-review inventory through trusted checkout code.
It snapshots and verifies referenced bundle/archive bytes, checks documented
local acceptance fields, and binds the callback and GitHub protocol CLI digests
and version identities to the verified bundle. Child evidence references must
be listed separately to receive auditor verification; the parent review below
also checks the indexed producer files. This is a maintainer tool in the
checkout, not a distributed bundle helper. External declarations require
independent review and always remain blockers. Provider testing stays explicitly
deferred. This tool never grants release approval.

| Verification | Evidence |
| --- | --- |
| Go regression and vet | `go test ./...` and `go vet ./...` passed with workspace caches; `/private/tmp/ingen-runtime-review-tests.log` and `/private/tmp/ingen-runtime-review-vet.log`. Focused journal/assessment and matrix/CLI race checks also passed. |
| Packaging and definitions | All 27 packaging tests passed, including nine auditor tests; `/private/tmp/ingen-runtime-review-packaging-final.log`. Shell syntax, CI YAML parsing, and `git diff --check` passed. Hosted CI has not run. |
| Final installed bundle | `make ingen-install-check` passed; `/private/tmp/ingen-local-install.MGjKNs`, log `/private/tmp/ingen-runtime-review-install.log`. Darwin/arm64, 643 selected inputs, nine modules/version reports, 57 artifacts, 17 license entries. Installed HTTP proof `/private/tmp/ingen-installed-tools.4QO5os`, index SHA-256 `03cc57b4a6709dbf55cdfbf8b6d90a83855f5adb5080ef7660fdcddded99497c`. |
| Read-only assessment and comparison acceptance | `bash acceptance/runtime-review.sh --bin /private/tmp/ingen-local-install.MGjKNs/install/bin/sentinel` passed; `/private/tmp/ingen-runtime-review.3P0oet`, index SHA-256 `9497ae23824836e4ab630b91d94594f2636e9444c69be3567594a065341f8d71`. Four assessment reports, baseline/variant byte-and-mode snapshots, preserved tampered inputs, restored baseline, and public report schemas verified. Uses fake Herdr identities and an actual uncontained `/usr/bin/true` wrapper; `/usr/bin/false` comparison received zero broker requests. No governed native execution or provider call is established. |
| Actual contained Codex comparison | Installed `sentinel agent compare --agent-executable /opt/homebrew/Caskroom/codex/0.160.0/bin/codex --agent-executable /usr/bin/false --timeout-seconds 30` exited 1. Report `/private/tmp/ingen-runtime-review-installed-codex.json`, SHA-256 `28838468dc6213ee520c8987866e3f1b98d1991ef805098b85ff2f88a9bcbba2`; public schema passed. Pinned Codex remained `unsupported`, reason `managed-preferences-unavailable-under-policy`, unchanged executable hash, closed broker, zero requests. |
| Current installed ingress and delivery | Callback proof `/private/tmp/ingen-callback-auth.axNiPH/project`, index SHA-256 `e58491873469e26569ec562d69b167cf6dcb87fe7778bc0f594ada737204eaf9`; ephemeral key removed. GitHub loopback proof `/private/tmp/ingen-github-checks.c9z3l0xs`, index SHA-256 `41d9cef2b6904fc46a88c3684f84a1f7e18692b574631fdcb04de1f9e6c20fdd`. Both use final installed binaries. Delivery reuses the separately built synthetic nine-module workflow `/private/tmp/ingen-ecosystem-http.GsLbO8` recorded above; its binaries are not claimed to match this bundle. |
| Release evidence audit | Inventory `/private/tmp/ingen-runtime-release-audit.vm48v1th/inventory.json`, SHA-256 `dbce66815c66761dba9cb1d4c2779c81030e163252fb312b55dbb37df29d709e`; report `report.json`, SHA-256 `a3d5038ed04e4b4419118b20bfdd50ed725b71d43dff8dac0b529d82a546117c`. Expected exit 1: four local gates pass, local review ready for human review, eight external blockers, provider deferred, release not approved. |
| Parent review | `/private/tmp/ingen-local-install.MGjKNs/root-review.json`, SHA-256 `0d5cb9e56c04104f318817e1900692158791ac180c1be389a8c84ee28db6786f`: bundle/archive/prefix verified, selected current inputs matched, 108 indexed references, 39 installed workflow files, 32 runtime review files checked, 29 SDK digests unchanged from consumer-tested bundle. Reproduced the audit, checked repeated assessments and restored project bytes/modes, and retained evidence/log/harness digests. |

Final manifest SHA-256 is
`ca2a1a18fd7c4313ca11397546be123690e0eb495b60e8feae9c41b9e7343ea8`;
selected-input fingerprint
`d7fc4d9b234e66535c035748c1226f411744828f5ffadd899aed33c91a730b53`;
archive SHA-256
`5b41d53bbbd715346222175b5e8612fa20f7d18a80cd24895d0500dbb04171be`.
The standalone helper is unchanged from the previous batch. CI now runs and
preserves the installed synthetic runtime/recovery acceptance; Make exposes
`sentinel-runtime-review-check SENTINEL_BIN=ABS`.

This batch remains uncommitted over `84c1070`, with no live Herdr mutation,
policy weakening, host restart, provider call, GitHub service call, signature,
or publication. Compatible contained runtime, Linux enforcement, native governed
workflow, signed host callbacks/restart, live delivery, native install matrix,
hosted CI, and signed distribution/independent rebuild remain release gates.

### Current Herdr assessment

The installed Herdr 0.9.3 bundled schema still reports protocol 22/schema version 1. Both event envelopes require only `event` and `data`; the agent-status event has workspace/pane identity and status, with no host event ID or event timestamp. Subscription parameters expose subscriptions without a durable cursor.

The current [official socket documentation](https://herdr.dev/docs/socket-api/#event-subscriptions) explicitly describes non-durable history, `events_lost`, and reconciliation through a new subscription plus authoritative snapshot. That recovery restores current state rather than missing history. This narrows the remaining integration question: durable lifecycle evidence must come from a reviewed host guarantee or an explicitly owned Sentinel launch journal, with raw UI events retained as observations. Do not manufacture host identities or mark snapshot reconciliation as historical replay. No new live callback or restart proof was performed in this batch.

The dependency path is baseline restoration, current Herdr contract, native sessions and enforcement, live ecosystem integration, adoption/recovery proof, then packaging and release. Governance, architecture, provenance, custody, comparison, and Linux feasibility work can progress independently once the baseline and required artifact handoffs are defined.

Maintain one release evidence index in this document as work proceeds. Each milestone needs its relevant source revision, exact commands, supported environment, artifact locations/digests, outcomes, and remaining gaps. Mark completion only when its exit criteria are demonstrated.

No calendar estimate is reliable until the current Herdr contract and pending-work baseline are evaluated. Estimate the remaining work after milestones 1 and 2; native host guarantees and capability enforcement are the largest uncertainties.

The baseline repair, current local suites, mainline CI definition, native role
evidence handoffs, and local nine-module workflow have evidence above. The
current checkpoint identifies the next batch. Hosted registries, remote retention
platforms, additional languages/transports, and independent attestation require
separate requirements.

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
