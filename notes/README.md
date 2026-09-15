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

- [The contract can cross language boundaries](concepts/the-contract-can-cross-language-boundaries.md)
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
- [Provider preparation can cross the CI boundary before execution](modules/sorna-preparation-ci-result.md)
- [The alpha boundary should be named before it is expanded](modules/sorna-alpha-interface-checkpoint.md)
- [Alpha drift guards reject changed campaign inputs](modules/sorna-alpha-drift-guards.md)
- [State-transition mutations test lifecycle behavior](modules/sorna-state-transition-mutation.md)
- [Persistence mutations test cross-request identity](modules/sorna-persistence-mutation.md)
- [Input-validation mutations test rejected inputs](modules/sorna-input-validation-mutation.md)
- [Provider negative paths protect reviewed mutation targets](modules/sorna-provider-negative-paths.md)
- [A versioned provider schema keeps capability handoffs language-neutral](modules/sorna-provider-schema.md)
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
- [A subject run consumes the frozen oracle, not the contract source](modules/sorna-run-consumes-frozen-oracle.md)
- [A fair mutation comparison requires the same sealed contract](concepts/fair-mutation-comparison-requires-the-same-contract.md)
- [Go's JSON map ordering and canonical bytes](language/go-json-maps-and-canonical-bytes.md)
- [Go's HTTP handler boundary supports serving and isolated testing](language/go-http-handlers-and-httptest.md)
- [A generated boundary must be finite and reproducible](techniques/a-generated-boundary-must-be-finite.md)
- [The document subject keeps workflow transitions visible at the boundary](modules/document-pipeline-subject.md)
- [The first Sorna runner accepts a subject URL instead of a subject package](modules/sorna-http-runner.md)

This is intentionally a small corpus. New entries should come from
implementation surprises and experiments, not invented prose.
