# Sentinel's alpha boundary should freeze before native Herdr binding

## Claim

The current Sentinel receipt, provider-neutral Herdr event ingress, and shared
CI handoff have enough executable evidence to freeze their local alpha
semantics. Native Herdr host binding should remain a separate future boundary.

## What is stable in this slice

- `ingen.sentinel-run/v1` records workspace identity, ordered lifecycle events,
  exact artifact references, and the receipt status.
- `ingen.herdr-event/v1` binds callback identity to one run/workspace, supports
  idempotent retries, verifies referenced bytes when rooted, and publishes
  batches all-or-nothing.
- `ingen.sentinel-ci-explanation/v1` gives the producer-owned lifecycle and
  audit context a closed nested shape without making Nublar interpret it.
- Terminal Sentinel lifecycle status maps through `ingen.ci-result/v1` to
  `passed/0`, `failed/1`, or `error/2`, and the Nublar workflow preserves that
  decision while keeping the receipt report opaque.
- Terminal CLI emission audits one validated receipt snapshot before writing
  the shared envelope.
- Rooted Herdr artifact verification and the terminal Sentinel audit share one
  symlink-aware path resolver, so an artifact cannot escape the supplied root
  in one boundary while passing through the other.
- Workspace bootstrap and artifact registration apply the same resolver when
  creating new receipt file references.
- Capability-plan loading and the Sorna oracle/verifier handoffs use the same
  resolver for workspace, policy, and frozen-oracle inputs.
- Verifier preparation resolves the subject root under the supplied project
  root before Sorna launch.
- This rooted reference rule is now an explicit cross-boundary alpha invariant
  in ALPHA-INTERFACES.md.

## Why freeze this now

The positive and expected-failure host-enabled proofs, together with direct
blocked-path and retry regressions, establish the local ownership boundaries.
Freezing these semantics prevents later convenience code from moving Herdr
lifecycle meaning into Nublar or turning a producer report into a behavioral
verdict.

## Deliberately open

- Herdr's actual plugin hook, session identity, callback authentication, and
  persistence API.
- The complete Herdr lifecycle transition graph beyond the narrow terminal
  regression guard.
- Remote event queues, replay retention, hosted retries, and independent
  attestation of callback origin or non-observation.

## Proofs

```sh
make nublar-sentinel-run-collect-fresh
make nublar-sentinel-run-collect-failure-fresh
GOCACHE=/private/tmp/ingen-sentinel-go-cache go test -race ./herdr-sentinel/... ./nublar/... ./core/...
```

The expected-failure target uses the controlled webhook duplicate-idempotency
defect and succeeds only when the final Nublar run is `failed/1`.
The latest host-enabled positive and expected-failure runs were inspected in
`/private/tmp/ingen-sentinel-workspace.fqFUDU` and
`/private/tmp/ingen-sentinel-failure-workspace.aoumsG`.

## Verification caveat

The focused Sentinel/Nublar/core race slice passes. The broader
`make alpha-interface-check` currently reaches the Sentinel-adjacent Sorna
packages but is not green because the existing macOS timing-sensitive sandbox
test `sorna/internal/sandbox/TestAccessCaptureCanMissShortLivedTransitionBetweenSamples`
does not consistently observe the expected short-lived process transition. No
Sentinel package is the failing package in that checkpoint.

## Related

- [`ALPHA-INTERFACES.md`](../../ALPHA-INTERFACES.md)
- [Sentinel verifier receipts enter Nublar through the shared CI envelope](nublar-sentinel-verifier-workflow.md)
- [A failed Sentinel receipt must remain non-passing at the Nublar boundary](sentinel-negative-collection.md)
- [A Sentinel integration proof must start with a fresh artifact root](sentinel-fresh-collection.md)
- [Herdr events should enter Sentinel, not Nublar](sentinel-herdr-event-adapter-boundary.md)
