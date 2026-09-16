# The replay matrix is a frozen expectation boundary

## Finding

A replay regression suite can contain intentional reds. A coordinator must
therefore distinguish “this member failed unexpectedly” from “this known defect
produced the failure we expected.” The matrix makes that distinction explicit
by storing expected CI and nested replay classifications beside the observed
values.

The matrix is still a Sorna-owned report. It does not reinterpret the replay
rules or turn a failed member into a passing replay. It only decides whether
the set of producer results matches the reviewer's declared regression
expectations.

## Implementation

`BuildReplayMatrixCIResult` loads each `behavioral-replay` CI envelope, strictly
validates its nested `sorna.replay/v1` report, checks the outer status mapping,
and records the member path and SHA-256 under `replay:<id>`. The generated
`sorna.replay-matrix/v1` report keeps the contract and recorded/replay run IDs
for each member. Its explanation is a compact list of classification
mismatches.

The six-case example is declared in a separate
`sorna.replay-matrix-manifest/v1` YAML file. The manifest is hashed as a
`matrix_manifest` input, and verification checks that its paths and
expectations still agree with the generated report.

The public shapes are closed JSON schemas under `sorna/spec/`. The schemas
describe structure; runtime validation remains responsible for cross-field
invariants such as count totals, status consistency, and unique IDs.

## Independent verification

Creation and verification are separate operations:

```sh
make sorna-replay-matrix-ci-result
make sorna-replay-matrix-verify
```

`VerifyReplayMatrixCIResult` re-hashes every available member, revalidates
member reports, checks the matrix explanation against the full report, and
compares the stored lineage with current member bytes. A valid error envelope
can be verified as an error outcome, but it cannot silently become a passing
matrix.

The Make-level reproducibility check is intentionally separate from the
in-process verifier:

```sh
make sorna-replay-matrix-ci-result-fresh
```

It excludes existing artifacts and caches from a temporary source copy,
regenerates the six members, writes the aggregate, and invokes independent
verification in the same clean workspace. This catches accidental dependence
on a developer's prior artifact root while preserving the generated workspace
for inspection.

## Limit

The matrix does not establish that the expectations are correct or complete.
It proves that the declared regression set produced the declared
classifications. The quality of those expectations still comes from contract
review, mutation design, and independent fixture selection.
