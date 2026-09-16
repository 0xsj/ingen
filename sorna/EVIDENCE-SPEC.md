# Sorna Evidence Specification

Status: Draft design specification

This document defines the evidence bundle produced by Sorna and the minimum
provenance needed to make a result reproducible and honest about its level of
assurance.

Evidence does not make an incorrect contract correct. It makes the relationship
between contract, oracle, implementation, execution, and result inspectable.

## 1. Goals

Sorna evidence should allow a reviewer to answer:

- Which contract was evaluated?
- Which oracle and concrete cases were used?
- Was the oracle frozen before implementation results were visible?
- What implementation revision ran?
- What public observations occurred?
- Which contract rules passed or failed?
- Which mutations were killed or survived?
- What access policy and environment were used?
- Can the run be replayed or explained?

## 2. Evidence levels

The result must state its assurance level rather than implying that all runs
have the same strength.

### Level 0: self-reported

The runner reports inputs and results. Host access observations may be present,
but their completeness is not independently attested, so they are not an
independence claim.

### Level 1: reproducible

Contracts, oracle artifacts, implementation revisions, seeds, environment
versions, and results are hashed and stored. A second party can replay the
run, but the oracle boundary is not structurally enforced.

### Level 2: capability-isolated

The oracle-generation process runs with explicit allowed inputs, denied roots,
and runner capabilities. Access policy and enforcement results are recorded.

### Level 3: externally attested

Level 2 plus an independent trusted system attests to the execution boundary
and artifact custody. This is future scope; Sorna should not claim it in the
initial release.

The manifest must contain one of these levels and a list of reasons for any
reduction in assurance.

## 3. Bundle layout

An evidence bundle is an immutable directory or archive:

```text
run/
  manifest.json
  contract/
    contract.yaml
    canonical.json
    hash.txt
  oracle/
    source/
    frozen-cases.jsonl
    hash.txt
  policy/
    isolation.yaml
    tool-policy.json
  sut/
    revision.json
  observations/
    case-0001.json
    case-0002.json
  results/
    rules.jsonl
    summary.json
  mutations/
    catalogue.jsonl
    results.jsonl
  campaign/
    plan.json
    provider.yaml
    provenance.json
  events/
    lifecycle.jsonl
    access.jsonl              # oracle-generation observations
    subject-access.jsonl      # managed-subject observations
    executables.jsonl         # oracle process identity observations
    subject-executables.jsonl # managed-subject identity observations
  review/
    approvals.json
    waivers.json
  checksums.sha256
```

The exact storage backend may change. Logical artifact names and their hashes
must remain stable.

## 4. Run manifest

The manifest is the entry point for the evidence bundle:

```json
{
  "schema": "sorna.evidence/v1",
  "run_id": "sorna-2026-0001",
  "created_at": "2026-09-12T12:00:00Z",
  "assurance": {
    "level": 2,
    "status": "capability-isolated",
    "observation_coverage": "not-observed",
    "limitations": []
  },
  "contract": {
    "id": "todo-api",
    "version": 1,
    "sha256": "..."
  },
  "oracle": {
    "artifact_sha256": "...",
    "case_manifest_sha256": "...",
    "generator_seed": 4242,
    "frozen_at": "2026-09-12T11:58:00Z"
  },
  "sut": {
    "revision": "git:abc123",
    "image_digest": null,
    "adapter": "http-json-v1"
  },
  "policy": {
    "isolation_sha256": "...",
    "tool_policy_sha256": "..."
  },
  "results": {
    "rules": {"passed": 15, "failed": 3, "inconclusive": 0},
    "mutations": {"killed": 8, "survived": 1, "equivalent": 1}
  },
  "artifacts_sha256": "..."
}
```

The manifest must not contain secrets. Secret-bearing observations should be
redacted with a recorded redaction rule and digest where possible.

## 5. Artifact integrity

Every material artifact is content-addressed or included in
`checksums.sha256`. At minimum, hash:

- canonical contract;
- contract fixtures;
- oracle source and generated cases;
- isolation and tool policies;
- managed-subject policy, when a subject was launched under one;
- implementation revision or image digest;
- normalized observations;
- rule results;
- mutation catalogue and results;
- exact campaign plan and provider inputs for each per-mutation bundle;
- approval and waiver records.

The final manifest is written only after these artifact hashes exist. If an
artifact changes, the run ID must be treated as a new run or explicitly marked
invalid.

Sorna should provide a verification command that checks hashes and reports the
first mismatch. Hashing is an integrity mechanism, not proof that an artifact
was originally correct.

## 6. Lifecycle events

Events are append-only JSON Lines records. Each event includes:

```json
{
  "event_id": "evt-0007",
  "run_id": "sorna-2026-0001",
  "sequence": 7,
  "timestamp": "2026-09-12T12:01:00Z",
  "actor": "oracle-runner",
  "kind": "oracle.frozen",
  "payload": {
    "artifact_sha256": "...",
    "case_count": 18
  }
}
```

Important lifecycle events include:

- `run.created`;
- `contract.loaded`;
- `contract.sealed-verified`;
- `oracle.generation.started`;
- `oracle.generation.completed`;
- `oracle.frozen`;
- `sut.access.granted`;
- `case.started` and `case.completed`;
- `mutation.injected` and `mutation.completed`;
- `review.approved`, `review.waived`, and `run.finalized`.

Events must be ordered, but wall-clock time alone must not be treated as a
security boundary. Structural access policy remains authoritative.

## 7. Access evidence

Where the platform supports it, record:

- process identity and sandbox identity;
- resolved launch executable path and SHA-256 digest;
- host-observed live executable path, digest, and observation timestamp;
- coalesced executable identity observations for the root and observed
  descendants, including path/digest transitions and observation gaps;
- executable sampling interval, attempted-sample count, and UTC sampling
  start/stop timestamps;
- root and observed descendant process IDs, with completeness treated as
  best-effort unless the platform independently attests the process tree;
- allowed and denied roots;
- attempted file opens and denied accesses;
- network policy and connection attempts;
- tools invoked and arguments after secret redaction;
- workspace and worktree identifiers;
- artifact reads and writes;
- policy violations and enforcement outcomes.

The evidence record must distinguish:

- an action that was not attempted;
- an action that was attempted and denied;
- an action that was allowed;
- an action for which the platform has no observation.

“No event recorded” is not equivalent to “the action did not happen.”

Oracle-generation and managed-subject access streams must remain separate. A
subject's access report cannot be used to claim that oracle generation was
independent, and oracle-generation telemetry cannot be transferred to the
subject run.

For a mutation campaign, the per-mutation bundle also contains the exact bytes
of the campaign plan and provider manifest that selected the subject variant.
`campaign/provenance.json` records their source paths, bundle paths, SHA-256
digests, mutation sequence, and mutation ID. These files are included in the
bundle manifest and `checksums.sha256`; changing either input invalidates the
bundle rather than silently changing its meaning.

The campaign aggregate must record the SHA-256 values of the verified
per-mutation `manifest.json` and `checksums.sha256` files. A path alone is not
an evidence binding because a later run could reuse or replace that directory.

Executable identity streams follow the same separation. `executables.jsonl`
belongs to oracle generation and `subject-executables.jsonl` belongs to the
managed subject. The Darwin collector samples the observed process tree while
it runs and writes a record when a PID's executable path or digest changes;
repeated identical samples are coalesced. The stream is therefore a timeline
of parent-side observations, not an attestation that every process transition
was seen. Observation errors remain in the manifest rather than being treated
as successful coverage. A verifier requires a positive interval and complete
sampling window whenever samples were attempted, and requires the JSONL record
count to match the manifest observation count. The top-level
`assurance.observation_coverage` field makes this limitation machine-readable:
`not-observed`, `unavailable`, `periodic-best-effort`, or
`periodic-best-effort-with-gaps`.

Sorna's `gate` command applies CI policy after evidence verification. By
default, ordinary contract failures, non-killed mutations, and incomplete
oracle executions block. A killed mutation is a passing sensitivity result even
though its contract verdict is expected to fail. Observation coverage is
reported but does not block. A caller may require `periodic-best-effort`, or
allow known gaps by requiring `periodic-best-effort-with-gaps`. The gate emits
`ingen.gate/v1` and uses exit code `0` for a passing policy, `1` for a policy
failure, and `2` for invalid inputs or unverifiable evidence.

## 8. Rule result schema

Each contract rule result should include:

```json
{
  "rule_id": "todo.create.rejects-empty-title",
  "case_id": "case-0004",
  "status": "pass",
  "subject": "POST /api/todos",
  "observation_sha256": "...",
  "assertions": [
    {"path": "status", "expected": 400, "actual": 400, "status": "pass"},
    {"path": "error.code", "expected": "invalid_title", "actual": "invalid_title", "status": "pass"}
  ],
  "duration_ms": 12
}
```

Allowed statuses are `pass`, `fail`, `error`, `timeout`, `inconclusive`, and
`skipped`. Every non-pass result requires a reason. Every result must point to
a contract rule and a concrete case.

## 9. Mutation evidence

Mutation records must include:

- mutation ID and plane;
- target revision or artifact hash;
- mutation description and operator;
- baseline result reference;
- affected rule IDs if known;
- outcome;
- execution duration;
- reason for `equivalent`, `invalid`, or `inconclusive` classification;
- supporting observations.

Mutation score must be calculated from the declared denominator and must not
silently exclude survivors. Exclusions require a reason and reviewer identity.

The current Sorna CLI enforces the baseline requirement before a mutation run:
`--baseline-evidence` must point to a checksum-valid, unmutated run whose
contract verdict is `pass`. The baseline contract, oracle, oracle-policy, and
managed-subject-policy identities must match the candidate. The candidate's
`run.json` and manifest retain the baseline path and run ID.

## 10. Review and waivers

Review records should identify:

- reviewer or agent identity;
- artifact hash reviewed;
- decision and timestamp;
- comments;
- scope of any waiver;
- expiration or follow-up issue.

A waiver changes the acceptance decision; it does not rewrite a failed result.

## 11. Replay

A replay requires:

- the same contract version;
- the same oracle artifact;
- the same fixture hashes;
- the same generator seed;
- an equivalent adapter and environment;
- the implementation revision or mutation patch;
- the recorded policy.

Replay may produce a different result when nondeterminism is part of the
system. In that case, Sorna must report the divergence rather than replacing
the original evidence.

The initial replay command follows this boundary explicitly:

```sh
sorna evidence replay --oracle <frozen-oracle.json> --base-url <equivalent-subject-url> <evidence-directory>
```

It verifies the stored evidence checksums and semantics before executing the
caller-supplied canonical oracle. The contract source is not loaded, the
original subject URL is not selected implicitly, and the evidence directory is
read-only. Contract-visible rule and verdict changes are outcome drift. Replay
also fingerprints the ordered public request intent for each case: setup and
target method/path/query/body are compared, while host and port are ignored.
Request-intent changes are drift even when the response still satisfies the
contract. Observation hash changes are reported separately because a different
raw observation can still satisfy the same contract. A replay that cannot
evaluate the subject is an execution error or inconclusive result, not
behavioral drift.

For CI consumers, the replay report is preserved inside the shared
`ingen.ci-result/v1` envelope with `kind: behavioral-replay`. The envelope maps
`matched` to `passed`, `drifted` to `failed`, and `error` or `inconclusive` to
`error`.

Saved producer reports can be structurally checked with:

```sh
sorna evidence replay verify <replay-report.json>
```

## 12. Retention and redaction

Evidence should be retained long enough to investigate regressions and compare
contract versions. Retention policy must define:

- artifact lifetime;
- who can read raw observations;
- what is redacted;
- how redactions are represented;
- whether a redacted bundle remains replayable;
- deletion and legal-hold behavior.

Logs must not accidentally expose credentials, personal data, or private
implementation source. Redaction must itself be recorded.

## 13. MVP acceptance criteria

The first implementation is evidence-complete when it can:

- produce a manifest with contract, oracle, SUT, and policy hashes;
- emit rule results as JSONL;
- emit mutation results as JSONL;
- verify artifact checksums;
- identify the assurance level;
- distinguish denied access from missing access telemetry;
- preserve failed results and waivers without overwriting them;
- replay a deterministic HTTP/JSON experiment.
