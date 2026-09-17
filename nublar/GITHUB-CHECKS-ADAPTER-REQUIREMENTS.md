# GitHub Checks adapter requirements

Status: implemented and externally validated, 2026-09-17

This document defines the next concrete delivery target for Nublar: publishing
one provider-neutral Nublar decision as a GitHub Check Run attached to a commit
that may be reviewed through a pull request. It is a destination requirement,
not a commitment to change the frozen Nublar schemas or to add a GitHub client
in this slice.

The adapter should use GitHub's REST Checks API. GitHub documents check-run
creation and update at
<https://docs.github.com/en/rest/checks/runs>. Workflow token permissions are
documented at
<https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax>.

## Destination identity

- Transport: GitHub REST Checks API.
- Destination: one check run named by configuration, attached to a supplied
  repository and immutable head commit SHA.
- Default check name: `Nublar / <workflow-id>`; callers may provide an
  explicit name when several workflows share a commit.
- Required adapter inputs: repository owner/name, head SHA, check name, a
  stored Nublar decision, and a GitHub token.
- Optional inputs: details URL, display title, and a pull-request number used
  only for caller-side validation or links. A PR number must not replace the
  head SHA as the destination identity.

The adapter consumes `ingen.nublar-decision/v1` from Nublar. It must not read
or interpret nested producer reports to construct the check.

The live acceptance proof ran in the manual Nublar workflow at
[run 35204322692](https://github.com/0xsj/ingen/actions/runs/35204322692).
It created GitHub Check Run `105146774540` with `completed/success`, the
matching Nublar `run_id` in `external_id`, and an uploaded accepted receipt
with HTTP status `201`.

## Decision mapping

The check run is always published as `status=completed`:

| Nublar decision | Exit code | GitHub conclusion | Required meaning |
| --- | ---: | --- | --- |
| `passed` | 0 | `success` | The declared gate passed. |
| `failed` | 1 | `failure` | A producer check failed. |
| `error` | 2 | `failure` | Collection or coordinator delivery input was unusable; the summary must retain `error` and `2`. |

GitHub has no `error` check conclusion in the Checks API. Mapping Nublar's
coordinator error to a failing check is intentional: it fails closed for
review gates while preserving the precise Nublar status and exit code in the
check summary and Nublar's own decision artifact.

The check name, summary, and details must include:

- Nublar workflow ID, opaque `run_id`, status, and exit code;
- external correlation when present, including the GitHub Actions run ID and
  attempt;
- every check's ID, tool, required flag, status, reason, and result path/hash
  reference when available;
- links to the stored Nublar run, decision, or workflow artifacts when the
  caller has made them available.

The adapter must not copy producer `report` or `explanation` objects into the
GitHub check. It may link to an opaque artifact for review.

Annotations and pull-request comments are outside the first adapter. A later
consumer requirement may add them without changing the decision projection.

## Authentication and permissions

- The default credential is the workflow-provided `GITHUB_TOKEN`, injected via
  the environment or an equivalent secret mechanism; it must never appear in
  command arguments, logs, receipts, or persisted Nublar records.
- The publishing job needs the least permission that can create and update
  check runs: `checks: write`. It should retain only other permissions it
  demonstrably needs, such as `contents: read` for checkout.
- The adapter must surface missing, expired, or insufficient credentials as a
  failed delivery receipt and exit code `2`; it must not alter the stored run
  or decision.
- Pull requests from forks and Dependabot may receive read-only tokens. The
  adapter must fail closed when GitHub refuses the write; it must not require
  `pull_request_target`, elevated repository permissions, or secret exposure
  as a workaround.

## Idempotency and retries

- A repeated delivery of the same local Nublar `run_id` must update one GitHub
  check run, not create duplicates.
- Set the GitHub check run `external_id` to the Nublar `run_id`. Before
  creating a check run, the adapter must look for an existing check with the
  same repository, head SHA, check name, and `external_id`, then update it when
  found.
- A GitHub Actions rerun has a new Nublar `run_id` and a higher external
  attempt value. It may create a separate check run, but its summary must show
  the attempt and opaque local identity. The adapter must not merge two Nublar
  run records or rewrite Nublar history.
- Retry policy belongs to the adapter. It may retry bounded transient
  failures such as timeouts, rate limits, and 5xx responses with backoff. It
  must not retry malformed decisions, authentication failures, permission
  failures, or other permanent 4xx responses without an explicit policy.
- A successful response must be validated as the expected check run. A
  non-2xx response, timeout, or invalid response becomes a failed
  `github-checks` delivery receipt and command exit code `2`.
- Delivery attempts must not create new Nublar runs, change the stored Nublar
  decision, or turn a delivery failure into a producer result.

## Receipt and observability

Use the existing `ingen.nublar-delivery-receipt/v1` record with
`transport=github-checks`. Record accepted or failed status, attempted time,
HTTP status when available, and concise error detail without credentials or
producer report content.

The adapter should emit enough structured log context to diagnose a delivery:
repository, head SHA, check name, Nublar `run_id`, external attempt, retry
count, and remote check ID when known. Secrets and full response bodies must
not be logged.

## Acceptance proof

Before implementation is considered complete, an executable consumer proof
must demonstrate:

1. A passed decision creates a completed `success` check and returns exit code
   `0`.
2. A failed decision creates a completed `failure` check, retains the Nublar
   failed status, and returns exit code `1` from the Nublar gate.
3. An error decision creates a completed `failure` check whose summary still
   states Nublar `error` and exit code `2`.
4. Repeating delivery of one `run_id` updates the same remote check and leaves
   one Nublar run plus independent delivery receipts.
5. A rerun with a new `run_id` and higher attempt remains distinguishable in
   both Nublar history and the GitHub check metadata.
6. Permission failure, permanent API failure, timeout, and bounded transient
   retry paths produce failed receipts without mutating the stored decision.
7. The published check contains no producer-owned report or explanation
   fields, while the local Nublar run still preserves them opaquely.

## Contract and scope decision

This requirements slice proposes no schema change. The existing decision
projection and receipt record are sufficient for the payload and audit
boundary. Implementation may add a `github-checks` delivery transport and
destination-specific CLI configuration behind the existing delivery
projection.

Explicit non-goals:

- GitHub PR comments, annotations, approvals, or branch-rule management.
- Hosted Nublar storage or a remote delivery queue.
- Nublar launching producer workflows.
- Reinterpretation of Sorna, Paddock, Sentinel, or any producer report.
