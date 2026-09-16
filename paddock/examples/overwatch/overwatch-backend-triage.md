# Overwatch backend Paddock triage

Status: review note only. This document does not change the policy or its
sealed lock.

Initial run date: 2026-09-15

The portable Paddock gate was run against the sibling
`/Users/sj/Desktop/dev/builds/overwatch/overwatch-backend` repository using
`overwatch-backend-review.lock.json`.

- 136 packages and 1,607 dependency edges were analyzed;
- the locked gate returned exit `1` as expected;
- 40 findings were reported;
- no cross-context, shared-boundary, composition-root, cycle, or coverage
  findings were reported.

## Agent explanation replay

A fresh locked replay on 2026-09-16 was passed through `paddock explain` without
changing the source repository, policy, or lock:

- 136 packages and 1,607 edges were analyzed;
- the locked check returned exit `1` with the same 40 findings;
- the explanation grouped them as 38 `domain-is-pure`, one
  `application-not-infrastructure`, and one `layers-point-inward` finding;
- triage was `remediate`, with all 40 findings active and blocking.

The text and JSON explanation now expose the concrete constraint an agent needs
to inspect the boundary. For example, the application finding identifies the
denied `{role=infrastructure}` and `{role=adapter}` targets, while the domain
findings repeat the allowed standard-library, shared-error, and same-context
domain targets. The `audit/app/query` → `audit/infra/postgres` edge is reported
by both `application-not-infrastructure` and `layers-point-inward`; each
explanation retains its rule but identifies the other through `related_rules`.
An agent can narrow the handoff without reading the full report:

```sh
paddock explain /tmp/overwatch-backend-paddock-report.json \
  --rule application-not-infrastructure --status blocking --format json
```

This validates the explanation path as a review aid. It remains separate from
the authoritative CI verdict and does not imply that the Overwatch findings
should be fixed, waived, or baselined automatically.

The complete agent handoff was then exercised against the unsealed
`overwatch-backend-shared-kernel-proposal.yaml`:

- the policy review reported exactly two semantic changes: the candidate
  project name and the `domain-is-pure` allow-list;
- its expected review case passed and measured three remaining findings;
- the filtered application explanation isolated the one concrete
  `audit/app/query` → `audit/infra/postgres` edge while retaining its related
  `layers-point-inward` signal.

This is the intended workflow boundary: Paddock supplies evidence, proposed
policy effects, and deterministic explanations; an architecture owner still
decides whether the shared-kernel change is accepted.

## Finding groups

| Rule | Count | Observed boundary | Initial assessment |
| --- | ---: | --- | --- |
| `domain-is-pure` | 35 | 17 domain packages import `pkg/id` | Shared-kernel vocabulary decision; `id.ID` is a value object used throughout domain models. |
| `domain-is-pure` | 2 | `audit/domain` and `journal/domain` import `pkg/events` | Likely intentional cross-cutting event vocabulary; confirm that events are an approved shared kernel. |
| `domain-is-pure` | 1 | `source/domain` imports `pkg/blob` | Likely a real boundary issue because `pkg/blob` owns storage concerns; prefer a domain-owned value or port. |
| `application-not-infrastructure` | 1 | `audit/app/query` imports `audit/infra/postgres` | Likely a real layering issue; the application query currently reaches into infrastructure for reader/cursor types. |
| `layers-point-inward` | 1 | Same `audit/app/query` → `audit/infra/postgres` edge | Duplicate signal for the same boundary; it should disappear when the application/infrastructure dependency is removed. |

## Recommended order

1. Decide whether `pkg/id` and `pkg/events` are approved shared-kernel
   dependencies for domain code. If they are, propose—not directly apply—the
   following policy expansion and update its message:

   ```yaml
   allow:
     - {standard-library: approved}
     - {role: shared, package: errors}
     - {role: shared, package: id}
     - {role: shared, package: events}
     - {role: domain, context: same}
   ```

2. Refactor `audit/app/query` so its application-owned port and cursor types do
   not come from `audit/infra/postgres`. The infrastructure package should
   implement the application contract, not define the contract consumed by it.

3. Refactor `source/domain` so it does not accept or parse the storage package's
   `blob.Info` directly. A small domain-owned capture reference/value or an
   application port can carry the required digest and size without importing
   the storage implementation.

4. Re-run `policy diff`, `policy review`, and the locked gate after each
   accepted change. Do not baseline the shared-kernel findings before the
   vocabulary decision. Use a waiver or baseline only for explicitly accepted
   temporary debt, with an owner, reason, and expiry.

## Concrete remediation shape

The two remaining boundaries have identifiable ownership seams:

### Audit query port

`internal/audit/app/query/ledger.go` imports
`internal/audit/infra/postgres` only to reuse `Cursor`, `Facet`, and the
`Reader` method signatures. A minimal direction-preserving refactor is:

1. Define `Cursor`, `Facet`, and the `Reader` interface in the application
   query package.
2. Remove the application package's import of `audit/infra/postgres`.
3. Make `audit/infra/postgres` import the application query package and expose
   aliases or implementations of those application-owned types.
4. Keep SQL, database cursors, and row conversion in infrastructure.

That leaves the dependency pointing from infrastructure inward and removes
both current findings on the same edge.

### Source capture value

`internal/source/domain/source.go:123` accepts `blob.Info` and
`source.go:130` calls `blob.ParseRef`. The application command already owns
the `Blobs.Put` call. A boundary-preserving refactor is:

1. Add a small domain-owned capture value containing the validated SHA-256
   string and byte count.
2. Convert `blob.Info` to that value in `internal/source/app/command`.
3. Keep blob reference parsing, storage handles, and corruption checks in the
   application/storage path; let the domain validate only its own capture
   invariants.

This removes the domain's dependency on the storage package without weakening
the capture metadata checks.

The policy and lock remain unchanged until the Overwatch architecture owner
approves the shared-kernel decision and the code-boundary plan.

## Candidate measurement

The unsealed `overwatch-backend-shared-kernel-proposal.yaml` was evaluated
against the same backend graph. It adds only `pkg/id` and `pkg/events` to the
domain allow-list and keeps all other rules unchanged.

- the policy diff contains two semantic changes: the project name and the
  `domain-is-pure` rule;
- the policy review artifact passed its existing expected-failure test;
- the candidate gate dropped from 40 findings to 3;
- the remaining findings are the `source/domain` → `pkg/blob` boundary and the
  `audit/app/query` → `audit/infra/postgres` boundary, with the latter still
  reported by both application and layer rules.

This makes the proposal useful for review, but it is not sealed and must not
replace `overwatch-backend-review.lock.json` without approval.
