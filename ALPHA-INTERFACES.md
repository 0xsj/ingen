# InGen alpha interfaces

This is the current Sorna/Nublar alpha boundary. It names the artifacts and
semantics that another tool may consume today; it does not promise that the
alpha is a long-term compatibility commitment.

## What this checkpoint freezes

For the current document-pipeline slice, a producer must keep the following
schema identities and meanings stable until an intentional interface review:

| Boundary | Schema | Owner | Current meaning |
| --- | --- | --- | --- |
| Contract source/sealed artifact | `ingen.contract/v1` | Sorna | Reviewed behavioral intent that can be canonicalized and sealed. |
| Capability policy | `ingen.policy/v1` | Sorna | Declared capabilities and enforcement requirements for a run. |
| Frozen oracle | `ingen.oracle/v1` | Sorna | Materialized cases and expectations produced before subject execution. |
| Mutation catalogue | `ingen.mutation-catalogue/v1` | Sorna | Reviewable mutation declarations before a campaign plan exists. |
| Campaign plan | `ingen.mutation-plan/v1` | Sorna | Hash-bound handoff from contract/oracle/baseline review to a provider. |
| Mutation provider | `ingen.mutation-provider/v1` | Provider | Prepared variants, capabilities, provenance, and executable identities. |
| Provider review | `ingen.mutation-provider-review/v1` | Sorna | No-execution capability and plan-binding decision. |
| Preparation summary | `ingen.mutation-preparation/v1` | Provider | Changed files, target resolution, hashes, and prepared variants before execution. |
| Campaign result | `ingen.mutation-campaign-result/v1` | Sorna | One clean comparison and one outcome for each planned mutation. |
| CI result envelope | `ingen.ci-result/v1` | InGen core | Language-neutral status, exit code, source identity, input hashes, and opaque producer report. |
| Nublar workflow | `ingen.nublar-workflow/v1` | Nublar | Required/optional CI result paths resolved under an artifact root. |
| Nublar aggregate | `ingen.nublar-result/v1` | Nublar | Preserved input envelopes plus severity composition. |
| Nublar decision | `ingen.nublar-decision/v1` | Nublar | Provider-neutral delivery projection without producer reports. |
| Nublar delivery receipt | `ingen.nublar-delivery-receipt/v1` | Nublar | One delivery attempt outcome, separate from the run decision. |
| Sentinel lifecycle receipt | `ingen.sentinel-run/v1` | Sentinel | Workspace lifecycle and opaque artifact lineage around a verifier handoff. |
| Herdr lifecycle event | `ingen.herdr-event/v1` | Sentinel adapter | Idempotent host-event ingress bound to one Sentinel run and workspace. |

The `v1` labels are alpha interfaces, not a claim that every field is already
ideal. A breaking field or semantic change must be deliberate, documented, and
accompanied by a version or migration decision; it must not be smuggled into a
producer because the current repository happens to be a monorepo.

## Cross-boundary invariants

These are the important guarantees of the current slice:

1. A campaign plan refers to the exact contract, oracle, catalogue, baseline,
   and policy identities used to create it.
2. A provider is reviewed before a subject is launched. Strict providers must
   bind to the plan hash.
3. Preparation must prove that the provider, plan, summary, source copy, and
   prepared binaries agree. Ambiguous or missing source targets and no-op
   mutations are preparation failures.
4. Every mutation is compared with the same clean baseline and frozen oracle.
   `killed` demonstrates contract sensitivity; it does not demonstrate that
   the contract is complete.
5. `ingen.ci-result/v1` owns orchestration status and exit-code meaning. The
   producer owns the nested report and explanation.
6. Nublar composes severity without interpreting Sorna findings:

   | Inputs | Aggregate | Exit code |
   | --- | --- | ---: |
   | all `passed` | `passed` | 0 |
   | at least one `failed`, no `error` | `failed` | 1 |
   | at least one `error` | `error` | 2 |

7. A path and SHA-256 identify bytes consumed by a run. They establish
   integrity and detect drift; they do not prove correctness, isolation, or
   that an agent never saw implementation details.

## What is deliberately not frozen

- The complete mutation-operator vocabulary and language-specific target
  model.
- Additional source providers and SDKs. The provider handoff is language
  neutral, but the Go provider is the only production-style provider currently
  exercised end to end.
- A cryptographic or independently attested proof of the oracle writer's
  non-observation of implementation details. The current isolation and
  telemetry are evidence toward that goal, not that proof.
- Sentinel's contract workspace, agent lifecycle, permissions, and user
  experience. Those belong to the Herdr-side workflow surface.
- Nublar as a standalone hosted product. The current Nublar code is a thin
  coordinator and integration proof surface.
- Producer-owned internal evidence and explanation schemas such as
  `sorna.evidence/v1`, `ingen.run/v1`, and
  `sorna.mutation-campaign-explanation/v1`. They are useful artifacts, but
  consumers should use the shared CI envelope rather than importing their
  internal meanings.

## Current verification command

Run the focused boundary check with:

```sh
make alpha-interface-check
```

Run the complete clean document workflow with:

```sh
make nublar-aggregate-fresh
make nublar-run-collect-fresh
```

These targets are the executable proofs for the current slice: they prepare
the strict Go provider, emit provider-review and preparation CI results,
execute the campaign, and aggregate or persist four required checks in a
temporary workspace.

The full repository check remains separate because unrelated pre-existing
Paddock compile errors currently prevent `go test ./...`; the focused command
covers the alpha surfaces named here.

## Related specifications

- [CI result envelope](core/CI-RESULT-SPEC.md)
- [Sorna mutation specification](sorna/MUTATION-SPEC.md)
- [Nublar README](nublar/README.md)
- [Nublar run artifact](nublar/RUN-ARTIFACT.md)
- [Nublar delivery boundary](nublar/DELIVERY-BOUNDARY.md)
- [Project status](status.md)
