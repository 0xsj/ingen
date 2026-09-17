# GitHub Actions consumer brief

Status: externally validated with live producer handoff, 2026-09-17

This brief treats one GitHub Actions job as Nublar's first concrete consumer
candidate. It is intentionally limited to a CI gate that collects producer
artifacts and returns Nublar's provider-neutral decision. It does not select a
GitHub API integration or change the frozen Nublar contract.

The executable workflow is
[`../.github/workflows/nublar-consumer.yml`](../.github/workflows/nublar-consumer.yml).
It is manually dispatched so it can validate the boundary without changing the
repository's normal pull-request or push checks.

## External validation

The workflow was dispatched from `dev` at commit `b036117` in
[GitHub Actions run 35186484244](https://github.com/0xsj/ingen/actions/runs/35186484244).
The job completed successfully, including the Nublar gate, correlation check,
artifact upload, and decision propagation steps.

The uploaded artifacts were downloaded and verified to contain:

- `ingen.nublar-run/v1` with `status=passed` and `exit_code=0`;
- `ingen.nublar-decision/v1` with the same run identity and decision;
- `correlation.system=github-actions`, the GitHub run ID, and attempt `1`.

This proves the GitHub Actions consumer mechanics with the checked-in producer
fixture. The workflow now also contains a separate producer job and Nublar
consumer job for live handoff validation.

The corrected live workflow was dispatched from `dev` at commit `3dd7c42` in
[GitHub Actions run 35200478467](https://github.com/0xsj/ingen/actions/runs/35200478467).
The macOS producer job published all four declared Sorna envelopes; the Ubuntu
Nublar job downloaded them and completed collection, correlation verification,
artifact upload, and decision propagation successfully.

The downloaded artifacts were verified to contain four passed
`ingen.ci-result/v1` producer envelopes and a passed `ingen.nublar-run/v1` plus
`ingen.nublar-decision/v1` with `exit_code=0` and the matching GitHub run
correlation.

## Consumer identity

- Name: GitHub Actions Nublar CI gate
- Owning system or repository: an InGen repository workflow job
- Invocation boundary: one job step after producer steps have written their
  `ingen.ci-result/v1` files
- Expected operating environment: the producer job runs on a macOS runner
  because the current Sorna host-enforcement backend is macOS-specific; the
  separate Nublar consumer job runs on a POSIX runner with the repository
  checkout, `bash`, `jq`, and the Go toolchain

The job must run after the producer workflows complete. Nublar remains a
consumer of their result envelopes; it does not launch Sorna, Paddock, or
Sentinel.

## Current surface

The existing provider-neutral gate is
[`examples/consumer/nublar-ci-gate.sh`](examples/consumer/nublar-ci-gate.sh).
The workflow's producer job invokes the existing producer targets, uploads only
the four declared result envelopes, and the separate Nublar job runs the gate
against those downloaded files.

The producer runner platform is part of the external executor contract. The
first Ubuntu attempt correctly reached the producer targets but produced no
envelopes because Sorna reported that no host enforcement backend was
available. The workflow therefore uses `macos-latest` for production and
retains Ubuntu for the provider-neutral Nublar consumer.

The equivalent consumer command is:

```sh
bash nublar/examples/consumer/nublar-ci-gate.sh \
  --workflow nublar/workflows/document-pipeline.yaml \
  --root .artifacts \
  --store .artifacts/nublar-runs \
  --output .artifacts/nublar-run.json \
  --external-system github-actions \
  --external-id "$GITHUB_RUN_ID" \
  --attempt "$GITHUB_RUN_ATTEMPT"
```

The existing surface provides:

- complete persisted `ingen.nublar-run/v1` output;
- a provider-neutral decision projection;
- `github-actions`, run ID, and run-attempt correlation;
- process exit codes `0` for passed, `1` for failed, and `2` for collection
  or export errors;
- immutable local history for the duration of the runner workspace;
- uploaded run and decision artifacts with an initial 14-day retention period.

## Required behavior

- Inputs: a checked-in Nublar workflow declaration, the artifact root, and
  complete producer envelopes at the declared relative paths. The external
  producer job owns creating those envelopes.
- Handoff: the producer job uploads only the four paths declared by
  `nublar/workflows/document-pipeline.yaml`; the Nublar job downloads them into
  its artifact root. Missing uploads remain visible to Nublar as collection
  errors.
- Correlation: `system=github-actions`, `id=$GITHUB_RUN_ID`, and
  `attempt=$GITHUB_RUN_ATTEMPT`.
- Outputs: the run JSON and decision JSON should be uploaded as workflow
  artifacts when the job needs post-run inspection.
- Gate result: the job must use Nublar's process exit code as its CI result;
  the JSON remains the machine-readable explanation.
- Authentication: no network delivery is required for this first consumer.
  If a later job publishes through `run deliver`, its webhook secret must be
  injected through the runner's secret store and passed with `--secret-env`.
- Retry semantics: a GitHub Actions rerun uses a new local `run_id` and the
  same external run ID with a new positive attempt value. Delivery retries,
  if added later, must reuse the same local run ID and create independent
  delivery receipts.
- Scale and retention: one local run store per job is sufficient for the
  initial consumer. The workflow uploads the run artifacts for 14 days;
  cross-runner history, hosted Nublar retention, and remote query semantics
  are not required by this brief.

## Acceptance proof

The consumer must demonstrate:

1. A complete passed fixture produces a persisted run, decision `passed`, and
   process exit code `0`.
2. A complete failed producer result produces a persisted run, decision
   `failed`, and process exit code `1`.
3. A missing required artifact or malformed envelope produces a persisted
   collection-error run and process exit code `2`.
4. A rerun can be located by the exact external correlation tuple without
   changing either run's opaque local identity.
5. `run list` succeeds as a read operation even when it lists failed runs, and
   `run show` returns the selected run's stored decision code.
6. The persisted run preserves producer reports as opaque data; the GitHub
   Actions consumer does not reinterpret Sorna, Paddock, or Sentinel findings.

The existing fixture-backed proof already covers these behaviors locally:

```sh
make nublar-consumer-check
```

The complete Nublar gate remains:

```sh
make nublar-freeze-check
```

## Scope decision

The frozen Nublar commands and schemas are sufficient for this first
GitHub-Actions-shaped consumer. No contract or schema change is proposed by
this brief.

The executable workflow now provides an externally validated
producer-to-consumer boundary. No contract or schema change was required. A
GitHub-specific status, annotation, API client, hosted receipt store, or
provider retry queue would be a separate consumer requirement behind the
provider-neutral decision projection.

## Non-goals

- Launching producer workflows from Nublar.
- Calling the GitHub API or publishing pull-request statuses.
- Treating a GitHub run ID as Nublar's local run identity.
- Persisting history across ephemeral runners.
- Reinterpreting producer-owned reports or explanations.
