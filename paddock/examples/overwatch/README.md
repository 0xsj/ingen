# Overwatch dogfood

This directory contains a review policy for the sibling
`/Users/sj/Desktop/dev/builds/overwatch/overwatch-backend` repository. It is a
Paddock integration example, not a dependency of Paddock and not a claim that
the current Overwatch code is already compliant.

Run it from the Ingen repository root:

```sh
go run ./paddock/cmd/paddock check \
  ../overwatch/overwatch-backend \
  --policy paddock/examples/overwatch/overwatch-backend-review.yaml
```

The policy intentionally uses `error` severity for architectural claims so
the command is a useful review gate. Current findings should be discussed
before enabling it in Overwatch CI; in particular, the policy tests whether
domain packages may import `pkg/id`, `pkg/events`, or `pkg/blob`, and whether
an application query may import infrastructure directly.

The companion policy-test manifest makes the current review reproducible:

```sh
go run ./paddock/cmd/paddock policy test \
  --policy paddock/examples/overwatch/overwatch-backend-review.yaml \
  --cases paddock/examples/overwatch/overwatch-backend-review.policy-tests.yaml
```

Its expected `fail` outcome is intentional: the manifest asserts that the
current graph still produces `domain-is-pure`, `application-not-infrastructure`,
and `layers-point-inward` findings. A passing policy test here means the review
expectations were reproduced, not that Overwatch is compliant.

The backend review policy is also sealed at
`overwatch-backend-review.lock.json`. Verify and run its lock-backed CI gate
with:

```sh
go run ./paddock/cmd/paddock policy verify \
  --policy paddock/examples/overwatch/overwatch-backend-review.yaml \
  --lock paddock/examples/overwatch/overwatch-backend-review.lock.json

go run ./paddock/cmd/paddock ci \
  ../overwatch/overwatch-backend \
  --policy-lock paddock/examples/overwatch/overwatch-backend-review.lock.json \
  --output overwatch-backend-ci-result.json
```

The backend command is expected to exit `1` while the current code has its
review findings; the latest locked run records 40 findings. The UI command
below exits `0`. Both use the same language-neutral CI result contract.

The companion UI now has a committed starter draft at
`paddock/examples/overwatch/overwatch-ui-draft.yaml`. It is intentionally
conservative: it records the source vocabulary Paddock can currently see
(`app`, `components`, `lib/http`, `lib/services`, and so on), classifies the
root layout explicitly, and leaves most components as `unclassified` until
the UI architecture is agreed.

Review it against the sibling UI with:

```sh
go run ./paddock/cmd/paddock check \
  ../overwatch/overwatch-ui \
  --policy paddock/examples/overwatch/overwatch-ui-draft.yaml
```

This is a vocabulary and graph-resolution check, not an architecture claim.
The next UI-specific step is to decide which `components/*` and `lib/*`
groups deserve stable roles and dependency rules.

The same expectation can be replayed as a policy test:

```sh
go run ./paddock/cmd/paddock policy test \
  --policy paddock/examples/overwatch/overwatch-ui-draft.yaml \
  --cases paddock/examples/overwatch/overwatch-ui-draft.policy-tests.yaml
```

The first enforcement-shaped proposal is also recorded separately in
`overwatch-ui-layered-proposal.yaml`. It assigns layers to the observed route,
presentation, runtime, service, transport, and shared-kernel areas, then checks
that service modules do not import UI code. It is review-only until the UI
owners approve those roles:

```sh
go run ./paddock/cmd/paddock policy test \
  --policy paddock/examples/overwatch/overwatch-ui-layered-proposal.yaml \
  --cases paddock/examples/overwatch/overwatch-ui-layered-proposal.policy-tests.yaml
```

That manifest also runs the small reusable TypeScript boundary fixture in
`paddock/examples/services/ui-boundary-ts/`: one compliant subject and one
subject that imports a presentation component from a service. The violating
case must fail specifically on `services-do-not-import-ui`.

Before sealing this proposal, review the policy diff carefully. The current
draft-to-proposal diff contains 22 changes, including replacing the generated
per-directory `components/*` entries with one broader `components/**`
presentation component. That collapse is intentional for this first boundary
proposal, but it is the main UI vocabulary decision still awaiting approval.

The proposal is now sealed at
`overwatch-ui-layered-proposal.lock.json`. Verify the lock and run the
authoritative UI gate with:

```sh
go run ./paddock/cmd/paddock policy verify \
  --policy paddock/examples/overwatch/overwatch-ui-layered-proposal.yaml \
  --lock paddock/examples/overwatch/overwatch-ui-layered-proposal.lock.json

go run ./paddock/cmd/paddock ci \
  ../overwatch/overwatch-ui \
  --policy-lock paddock/examples/overwatch/overwatch-ui-layered-proposal.lock.json \
  --output overwatch-ui-ci-result.json
```
