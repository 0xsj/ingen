# The provider review report is now a published artifact contract

The provider handoff already had a published manifest schema and a shared CI
envelope, but its nested no-execution review report was only described by Go
types and prose. The report is now published as
[`ingen.mutation-provider-review/v1`](../../sorna/spec/ingen.mutation-provider-review-v1.schema.json).

The schema is intentionally structural. It covers the plan and provider
identities, exact binding states, declared capabilities, and per-mutation
entry/capability statuses. Sorna's runtime review remains responsible for
cross-object relationships such as whether every planned mutation is covered;
JSON Schema alone does not make those relationships trustworthy.

This completes the portability path for a consumer that wants to inspect the
provider-review report without importing Sorna's Go package:

```text
provider manifest
    → Sorna no-execution review
    → ingen.mutation-provider-review/v1
    → ingen.ci-result/v1
    → Nublar aggregate / stored run / decision
```

The schema is compiled by Sorna's published-schema test and included in the
normal `jq empty sorna/spec/*.json` readiness check.
