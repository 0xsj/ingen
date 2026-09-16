# InGen notes

This is the learning corpus for InGen. It records language concepts, substrate
behavior, implementation techniques, reusable patterns, and verification
principles that the code cannot express by itself.

Start with [NOTES.md](../NOTES.md) for the protocol. Notes are written as the
work happens, not as a summary pass after the project is finished.

## Categories

| Directory | Use it for | Staleness |
| --- | --- | --- |
| [`modules/`](modules/) | one package or implementation slice | code changes |
| [`substrate/`](substrate/) | Go, the runtime, OS, tools, and dependencies | version changes |
| [`patterns/`](patterns/) | architectural shapes that recur | architecture changes |
| [`techniques/`](techniques/) | reusable ways of implementing or testing | rarely |
| [`language/`](language/) | Go and other language mechanics | language/runtime changes |
| [`concepts/`](concepts/) | InGen's verification and domain principles | concept changes |
| [`decision-adjacent/`](decision-adjacent/) | reasoning that needs more room than a decision | decision context changes |

## Reading orders

These are starting points, not a second architecture document.

| Intent | Begin with |
| --- | --- |
| I am about to write Go infrastructure | `language/` → `substrate/` → `techniques/` |
| I am about to add a Sorna capability | `concepts/` → `patterns/` → the relevant `modules/` |
| I just got an unexpected mutation result | `concepts/` → `techniques/` → the affected module note |
| I am changing a workflow boundary | `patterns/` → `concepts/` → decision records |

## Current notes

- [Malcolm's first parser preserves shape and defers meaning](modules/malcolm-first-parser.md)
- [Malcolm validates completeness after parsing](modules/malcolm-semantic-validation.md)
- [Malcolm's JSON IR is a transport boundary, not an evaluator](modules/malcolm-json-ir.md)
- [Malcolm carries typed request bodies and stateful setup across the boundary](modules/malcolm-request-body-lowering.md)
- [Malcolm preserves target negative assertions as Sorna rule strength](modules/malcolm-negative-assertions.md)
- [Malcolm event assertions use an explicit HTTP signal](modules/malcolm-event-assertions.md)
- [The Malcolm event mutation is killed by the frozen oracle](modules/malcolm-event-mutation.md)
- [Malcolm's CLI keeps the compile pipeline visible](modules/malcolm-cli.md)
- [Malcolm reaches Sorna through a rejecting IR adapter](modules/malcolm-sorna-bridge.md)
- [The Malcolm contract must be sealed before Sorna can produce behavioral evidence](modules/malcolm-sorna-run.md)
- [The Malcolm stateful flow produces evidence through separate Sorna policies](modules/malcolm-sorna-flow-run.md)
- [Rust ownership makes parser state explicit](language/rust-ownership-in-first-parser.md)
- [Rust enums make a small literal grammar explicit](language/rust-enums-for-typed-literals.md)
- [Rust's explicit string building makes a small JSON serializer teachable](language/rust-manual-json-serialization.md)
- [Rust's Result keeps CLI failures separate from successful output](language/rust-cli-errors-and-io.md)
- [The contract can cross language boundaries](concepts/the-contract-can-cross-language-boundaries.md)
- [A webhook lab needs idempotency before delivery infrastructure](modules/webhook-validation-lab.md)
- [A webhook mutation should break idempotency across requests](modules/webhook-idempotency-mutation.md)
- [A source provider should share preparation mechanics but own target meaning](modules/webhook-go-source-provider.md)
- [A second vertical should enter Nublar through the same opaque CI envelope](modules/nublar-webhook-workflow.md)
- [A Sentinel workspace assembles handoffs without becoming a second verifier](modules/sentinel-contract-workspace.md)
- [A Sentinel receipt connects lifecycle events to opaque artifacts](modules/sentinel-run-receipt.md)
- [A capability plan makes isolation declarations adapter-ready without claiming enforcement](modules/sentinel-capability-plan.md)
- [Sentinel should delegate enforcement instead of duplicating Sorna policy semantics](modules/sentinel-sorna-adapter.md)
- [A verifier handoff must preserve the oracle and subject policies separately](modules/sentinel-verifier-adapter.md)
- [Sentinel verifier receipts enter Nublar through the shared CI envelope](modules/nublar-sentinel-verifier-workflow.md)
- [A Herdr event batch must publish atomically](modules/sentinel-herdr-event-batch.md)
- [A receipt callback update needs a local lock around the read-modify-publish cycle](modules/sentinel-receipt-update-lock.md)
- [Sentinel's native Herdr binding needs a host contract first](modules/sentinel-herdr-host-binding-contract.md)
- [An operator report must separate lifecycle, integrity, and producer meaning](modules/sentinel-operator-report.md)
- [Artifact identity makes retries safe without accepting replacement bytes](modules/sentinel-artifact-registration.md)
- [Lockwood handling history belongs in append-only events](modules/lockwood-handling-events.md)
- [Evidence reports must publish as complete files](modules/sentinel-evidence-publication.md)
- [Audit and emission must share one receipt snapshot](modules/sentinel-receipt-snapshot.md)
- [A nested CI explanation needs its own closed contract](modules/sentinel-ci-explanation-contract.md)
- [A Sentinel integration proof must start with a fresh artifact root](modules/sentinel-fresh-collection.md)
- [A failed Sentinel receipt must remain non-passing at the Nublar boundary](modules/sentinel-negative-collection.md)
- [Sentinel's alpha boundary should freeze before native Herdr binding](modules/sentinel-alpha-interface-checkpoint.md)
- [Hammond approval governs identified bytes, not behavior](modules/hammond-governance-artifact-identity.md)
- [A governance schema must be enforced at the load boundary](modules/hammond-schema-runtime-validation.md)
- [A valid governance lineage is more than an acyclic graph](modules/hammond-lineage-is-semantic.md)
- [A review decision must belong to the review cycle it closes](modules/hammond-review-cycle-binding.md)
- [A governance state must name the policy that materializes it](modules/hammond-review-policy-boundary.md)
- [A role quorum counts distinct authorized actors](modules/hammond-quorum-policy.md)
- [A policy-dependent governance state needs a policy identity](modules/hammond-policy-identity.md)
- [A policy digest is not verified until the policy bytes are loaded](modules/hammond-policy-artifact-loading.md)
- [A role field is a claim until Hammond can verify authority](modules/hammond-role-authority-boundary.md)
- [A contract reference is only integrity-checked when its bytes are loaded](modules/hammond-contract-artifact-loading.md)
- [Policy requirements and actor authority should be separate artifacts](modules/hammond-authority-artifact.md)
- [Authority trust rotation must reject replay](modules/hammond-authority-trust-rotation.md)
- [Root-key rotation needs a caller-owned bootstrap](modules/hammond-authority-root.md)
- [Authority must be evaluated at decision time](modules/hammond-authority-time.md)
- [Membership windows belong to the authority source](modules/hammond-effective-membership.md)
- [An authenticated membership response is still a provider boundary](modules/hammond-membership-provider.md)
- [Transport can fetch membership bytes without owning provider identity](modules/hammond-membership-transport.md)
- [Endpoint discovery is caller-owned and URL-validated](modules/hammond-membership-endpoint.md)
- [Provider authentication is a caller-owned request capability](modules/hammond-membership-authentication.md)
- [A bearer token belongs to the caller's token source](modules/hammond-membership-bearer.md)
- [Decision provenance should name the membership snapshot](modules/hammond-membership-provenance.md)
- [Membership snapshot rotation must reject replay](modules/hammond-membership-rotation.md)
- [A provenance-carrying verifier must match the decision reference](modules/hammond-membership-verifier-binding.md)
- [Endpoint authorization must be explicit after URL validation](modules/hammond-membership-endpoint-policy.md)
- [Normalization can reject an incomplete provider view](modules/hammond-membership-normalization.md)
- [Freshness is a caller policy, not signature metadata](modules/hammond-membership-freshness.md)
- [Hammond's organization provider remains intentionally unselected](decision-adjacent/hammond-provider-integration-boundary.md)
- [Atomic replacement does not serialize independent store processes](modules/hammond-file-store-concurrency.md)
- [A digest-keyed blob store makes the URI a locator, not identity](modules/hammond-artifact-store.md)
- [A state label is not an executable transition](concepts/a-state-label-is-not-a-transition.md)
- [A subject URL is not an isolation boundary](concepts/a-url-is-not-an-isolation-boundary.md)
- [A killed mutation proves sensitivity, not correctness](concepts/a-killed-mutation-proves-sensitivity.md)
- [A mutation result needs a passing baseline](modules/sorna-baseline-precondition.md)
- [A mutation catalogue makes the experiment reviewable before execution](modules/sorna-mutation-catalogue.md)
- [A campaign plan freezes mutation inputs before execution](modules/sorna-campaign-plan.md)
- [A campaign executor must preserve one clean comparison per mutation](modules/sorna-campaign-execution.md)
- [A Go provider mutates a copy and hands off a prepared binary](modules/sorna-golang-provider.md)
- [A provider capability review should happen before execution](modules/sorna-provider-review.md)
- [A mutation campaign can cross the CI boundary without losing its semantics](modules/sorna-campaign-ci-result.md)
- [Contract-mutation inspection must stay separate from subject campaigns](modules/sorna-contract-mutation-inspection.md)
- [Provider preparation can cross the CI boundary before execution](modules/sorna-preparation-ci-result.md)
- [The alpha boundary should be named before it is expanded](modules/sorna-alpha-interface-checkpoint.md)
- [Alpha drift guards reject changed campaign inputs](modules/sorna-alpha-drift-guards.md)
- [State-transition mutations test lifecycle behavior](modules/sorna-state-transition-mutation.md)
- [Persistence mutations test cross-request identity](modules/sorna-persistence-mutation.md)
- [Input-validation mutations test rejected inputs](modules/sorna-input-validation-mutation.md)
- [Provider negative paths protect reviewed mutation targets](modules/sorna-provider-negative-paths.md)
- [A versioned provider schema keeps capability handoffs language-neutral](modules/sorna-provider-schema.md)
- [A shared CI envelope needs one status-to-exit-code contract](modules/ci-result-exit-code-contract.md)
- [Mutation results separate target sensitivity from setup fallout](modules/sorna-mutation-result-model.md)
- [A live process check adds lifecycle evidence without upgrading isolation assurance](modules/sorna-live-process-check.md)
- [A managed subject lifecycle improves reproducibility without proving isolation](modules/sorna-managed-subject-lifecycle.md)
- [A checksum-verified bundle proves artifact integrity, not isolation or correctness](modules/sorna-evidence-bundle.md)
- [A policy declaration is not an enforcement result](modules/sorna-policy-definition.md)
- [A host policy backend needs bootstrap permissions and canonical paths](modules/sorna-sandbox-enforcement.md)
- [Process execution is a capability, not an ambient convenience](modules/sorna-process-capabilities.md)
- [Adversarial boundary probes test negative claims, not just green paths](modules/sorna-adversarial-boundary-probes.md)
- [Host access telemetry is evidence, not an absence proof](modules/sorna-access-event-evidence.md)
- [Executable identity is a timeline, not a startup fact](modules/sorna-executable-identity-history.md)
- [CI should gate behavior first and observation quality explicitly](modules/sorna-assurance-gates.md)
- [A producer report can cross the CI boundary without losing its semantics](modules/sorna-ci-result-envelope.md)
- [A coordinator should compose verdicts, not reinterpret findings](modules/nublar-envelope-coordinator.md)
- [A CI workflow should declare its expected evidence](modules/nublar-workflow-declaration.md)
- [A coordinator should bind result paths to consumed bytes](modules/nublar-input-provenance.md)
- [Nublar workflows should resolve results under a fresh artifact root](modules/nublar-artifact-root.md)
- [The current Nublar slice is an integration harness, not the final product](decision-adjacent/temporary-nublar-integration-boundary.md)
- [A managed subject policy is separate from an oracle policy](modules/sorna-subject-isolation.md)
- [A frozen oracle is a separate artifact from a subject run](modules/sorna-oracle-freeze.md)
- [A frozen oracle must be non-empty and unambiguous at load time](modules/sorna-frozen-oracle-validation.md)
- [Replay must preserve the frozen boundary](modules/sorna-evidence-replay.md)
- [Replay results can cross the shared CI boundary](modules/sorna-evidence-replay-ci-result.md)
- [A replay report must validate its own interpretation](modules/sorna-replay-report-validation.md)
- [A replay matrix freezes expected regression classifications](modules/sorna-replay-matrix-interface.md)
- [Sorna alpha readiness is an executable stopping point](modules/sorna-alpha-readiness.md)
- [The contract schema must agree with the runtime identity](modules/sorna-contract-schema.md)
- [The capability policy needs the same cross-language boundary as the contract](modules/sorna-policy-schema.md)
- [A frozen oracle needs a language-neutral handoff shape](modules/sorna-oracle-schema.md)
- [A subject run consumes the frozen oracle, not the contract source](modules/sorna-run-consumes-frozen-oracle.md)
- [Run and evidence schemas keep the execution handoff language-neutral](modules/sorna-run-and-evidence-schemas.md)
- [A mutation campaign result needs a closed, canonical handoff](modules/sorna-mutation-campaign-schema.md)
- [A fair mutation comparison requires the same sealed contract](concepts/fair-mutation-comparison-requires-the-same-contract.md)
- [Go's JSON map ordering and canonical bytes](language/go-json-maps-and-canonical-bytes.md)
- [Go's HTTP handler boundary supports serving and isolated testing](language/go-http-handlers-and-httptest.md)
- [A generated boundary must be finite and reproducible](techniques/a-generated-boundary-must-be-finite.md)
- [The document subject keeps workflow transitions visible at the boundary](modules/document-pipeline-subject.md)
- [The first Sorna runner accepts a subject URL instead of a subject package](modules/sorna-http-runner.md)

This is intentionally a small corpus. New entries should come from
implementation surprises and experiments, not invented prose.
