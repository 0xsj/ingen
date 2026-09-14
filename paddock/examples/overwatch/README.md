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

The companion UI is currently graph-only dogfood. Its Next.js layout does not
yet have an agreed Paddock vocabulary, so generating a draft is the next UI
review step rather than a policy committed here.
