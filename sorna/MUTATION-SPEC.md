# Sorna Mutation Specification

Status: Draft design specification

Mutation testing deliberately changes a contract or implementation to test
whether the oracle suite is sensitive to meaningful behavioral differences.
Mutation results are evidence about detection power, not proof that the
contract is complete or correct.

## 1. Mutation planes

Sorna distinguishes two mutation planes.

### Implementation mutations

Change the system under test while keeping the contract and frozen oracle
unchanged. The question is:

> Would the oracle detect this externally observable implementation defect?

### Contract mutations

Change or weaken a contract rule while keeping the implementation and original
oracle lineage visible. The question is:

> Would the testing and review process make this specification change visible?

Contract mutations may be evaluated by regenerating an oracle, comparing rule
coverage, or requiring a review decision. They are not always executable in the
same way as implementation mutations.

The mutation plane must always be present in a result. A single mutation score
must not combine the two planes without clear labels.

## 2. Mutation record

Every mutation has a stable record:

```yaml
mutation:
  id: impl.todo.create.status-200
  plane: implementation
  operator: response.status.replace
  target: POST /api/todos
  description: Return 200 instead of the contracted 201.
  change:
    from: 201
    to: 200
  expected_observable:
    rules: [todo.create.valid.status]
  status: candidate
```

Required fields are:

- `id`;
- `plane`;
- `operator`;
- `target`;
- human-readable `description`;
- reproducible change or patch;
- expected observable rule IDs where known;
- mutation status and outcome.

The first catalogue file uses the versioned `ingen.mutation-catalogue/v1`
shape. It binds a set of stable mutation records to a contract ID while
keeping execution out of the declaration:

```yaml
mutation_catalogue:
  schema: ingen.mutation-catalogue/v1
  id: document-pipeline-mutations
  version: 1
  contract_id: document-pipeline
  contract_version: 1
  mutations:
    - id: status-200-create
      plane: implementation
      operator: response.status.replace
      target: POST /documents
      description: Return 200 instead of the contracted 202.
      change: {from: 202, to: 200}
      expected_rule_ids: [document.create.valid.accepted]
      status: candidate
```

`sorna mutation validate` checks the declaration before a campaign can use it;
with `--contract`, it also checks the contract ID, version, and expected rule
IDs. `sorna mutation list` provides a compact review view. The catalogue is
not yet an instruction to mutate source code, and validation does not claim
that an operator is safe or supported by a provider.

## 3. Campaign plan

`sorna mutation plan` resolves a validated catalogue against a frozen oracle
and a passing, unmutated baseline. It writes `ingen.mutation-plan/v1`, which
contains:

- the exact catalogue file hash and ordered mutation entries;
- the oracle and sealed contract identities;
- the baseline evidence path, run ID, and comparison identities;
- the oracle and managed-subject policy hashes.

The plan is a deterministic handoff to a mutation provider. Creating it does
not edit a working tree, apply an operator, or launch a subject process. The
provider must consume the plan and preserve its identities in the resulting
evidence.

## 4. Provider and execution boundary

A provider maps a plan mutation ID to a prepared subject command. The first
provider manifest uses `ingen.mutation-provider/v1` and supports only literal
argv plus the explicit `${SORA_ADDR}` and `${SORA_URL}` runtime tokens. It does
not use shell interpolation.

The manifest also declares provider capabilities as exact plane/operator/target
tuples. `sorna mutation run` requires every plan mutation to have both a
prepared entry and a declared capability; a provider cannot silently claim
support for an operator merely because it has an executable entry.

The no-execution review command is:

```sh
sorna mutation provider inspect <plan> --provider <path> [--require-plan-binding] [--format text|json|ci-result] [--output <path>]
```

JSON output uses `ingen.mutation-provider-review/v1`. A `ready` report means
every plan mutation has a prepared entry and declared capability. By default,
an absent provider plan hash is reported as `unbound` but remains usable for
local fixture workflows; `--require-plan-binding` turns that state into a
blocked review. A mismatched declared hash is always blocked.

`sorna mutation run` launches one fresh managed Sorna run per plan entry. Each
entry receives its own address and evidence directory, and the campaign result
records the entry's evidence path, verified manifest/checksum hashes, run ID,
exit code, and mutation outcome. The runner verifies the supplied oracle and
policy hashes against the plan before launching anything and rejects output
paths that overlap the clean baseline.

Each per-mutation evidence bundle also copies the exact plan and provider bytes
under `campaign/`. Its manifest and `checksums.sha256` bind those copies to the
run; `campaign/provenance.json` records the source paths, hashes, sequence, and
mutation ID. This proves which campaign inputs the executor consumed, while
remaining distinct from a future attestation of how a provider built a
mutated subject. The executor accepts the same `--require-plan-binding` flag,
so campaign execution can enforce the policy independently of preflight.

`sorna mutation verify` re-verifies each referenced evidence bundle and compares
its current manifest and checksum-file hashes with the campaign result. A
result with a changed or replaced evidence directory is therefore rejected by
the same workflow that produced it.

The document-pipeline provider is a temporary prebuilt fixture provider. It
proves the workflow boundary using the existing controlled defect; it is not
the final language-specific mutation system.

The first source-level Go provider is an optional replacement for that fixture.
It reads one captured canonical plan, copies the source root once per mutation,
applies a reviewed AST transformation to the copy, builds a fresh executable,
and emits the same `ingen.mutation-provider/v1` handoff. The source root is
never an output target. This keeps Go build mechanics separate from the
language-neutral campaign semantics without making the provider a second
campaign executor. Generated providers may include `plan_sha256`; Sorna
compares it with the exact plan bytes captured by the campaign before launch.
Source-level entries may also include `provenance` with a relative copied
source directory, source-tree and binary SHA-256 digests, and human-readable
`location`, `before`, and `after` edit descriptions. Sorna verifies those
source and binary digests immediately before launching the prepared subject.
They may also include `target_resolution`, containing the provider's selector,
candidate count, and applied count. A successful source mutation should report
one candidate and one applied target. If a provider finds zero or multiple
targets, it should return the structured
`ingen.mutation-target-resolution-error/v1` preparation error with the same
counts and apply nothing.

The Go source provider also writes an `ingen.mutation-preparation/v1` summary
(`preparation.json` by default). It binds the plan hash to each prepared
variant, lists changed source files, records source and binary hashes, and
retains the semantic provenance. A provider rejects a no-op mutation before
building or publishing that variant.
Sorna can expose the summary to a coordinator with:

```sh
sorna mutation provider preparation preparation.json \
  --provider provider.yaml --format ci-result \
  --output mutation-preparation-ci-result.json
```

The resulting `mutation-preparation` envelope retains the summary and binds it
to the provider manifest and plan without interpreting source-language details.

## 5. Operator families

The first HTTP/JSON implementation should support a small, deterministic set.

### Response operators

- replace status code;
- remove required field;
- change field type;
- change enum value;
- alter a required boolean;
- change error code;
- replace a response with an empty body.

### Input and validation operators

- remove required-field validation;
- widen a maximum length;
- narrow a valid range;
- accept an invalid enum value;
- skip malformed-input handling;
- change whitespace treatment.

### State operators

- skip persistence;
- skip state transition;
- return stale state;
- make an idempotent operation non-idempotent;
- delete the wrong record;
- return success without performing the operation.

### Frontend operators

- render stale response state;
- ignore an error response;
- map a field to the wrong display value;
- omit a required user-visible state;
- submit the wrong payload.

Operators should target public behavior whenever possible. Source-level
operators are acceptable as an implementation mechanism but the mutation's
description and expected effect must be observable at the contract boundary.

## 6. Mutation lifecycle

```text
candidate -> validated -> executed -> classified -> reported
```

1. Establish a passing baseline using the frozen oracle.
2. Apply one mutation to a clean implementation revision.
3. Verify that the mutation is syntactically and operationally valid.
4. Run the same frozen oracle without changing it.
5. Record the first decisive outcome and supporting evidence.
6. Restore the baseline or create a fresh mutated revision.
7. Repeat for the mutation set.

Mutation runs must not modify the contract or frozen oracle. A mutation that
requires changing the oracle is a different experiment and must be labeled.

## 7. Outcomes

### `killed`

At least one mandatory oracle rule fails because of the mutation.

### `survived`

The mutation executes successfully and all relevant oracle rules pass. This is
a signal that the contract/oracle suite may not detect a meaningful change.

### `equivalent`

The mutation changes implementation representation without changing behavior
observable under the declared contract. The classification needs a recorded
reason and, where practical, independent confirmation.

### `invalid`

The mutation cannot produce a valid executable subject, such as a syntax or
startup failure unrelated to the contract behavior.

### `timeout`

The mutated subject exceeds the configured execution limit. Timeout is not
automatically equivalent to killed; the evidence must explain whether the
timeout is a contract-visible failure.

### `inconclusive`

The run cannot distinguish the mutation because the contract or adapter lacks
the required observation. This must remain visible in the report.

## 8. Baseline requirements

Mutation results are valid only when:

- the unmutated baseline passes mandatory rules;
- the baseline and mutation use the same contract version;
- the same frozen oracle is used;
- the implementation revision and mutation are recorded;
- environment differences are recorded;
- flaky or nondeterministic cases are identified.

The current CLI enforces this precondition with `--baseline-evidence`: it
requires a checksum-valid passing run with no mutation and records the
baseline run ID and comparison identities in the mutation run.

If the baseline fails, mutation scoring should stop or be reported as
`baseline-invalid` rather than producing a misleading score.

## 9. Score and denominator

For implementation mutations, a basic sensitivity score is:

```text
killed / (killed + survived + equivalent-reviewed)
```

`invalid`, `timeout`, and `inconclusive` mutations must be reported separately
unless the project explicitly defines a different gate. Exclusions must never
silently disappear from the denominator.

Scores should be shown alongside:

- mutation count by operator;
- contract rules exercised;
- survivors and their impact;
- equivalent-mutant rationale;
- baseline stability;
- contract and oracle coverage.

A high score does not establish that the contract is correct. A low score does
not by itself identify the missing requirement; it identifies a place to
investigate.

## 10. Contract mutations

Initial contract mutation operators may include:

- remove a required rule;
- change `must` to `may`;
- widen a range;
- remove an invalid-input case;
- remove a required field;
- change a required status;
- remove a state invariant;
- add an incorrect requirement.

The report should identify whether the mutation was detected by:

- an existing oracle;
- a regenerated oracle diff;
- a contract review check;
- a reference implementation discrepancy;
- no mechanism.

Contract mutation testing should not silently treat an implementation that
already violates the original contract as a valid oracle of the mutation.

## 11. Flakiness and nondeterminism

If a mutation changes timing, generated IDs, random ordering, or another
nondeterministic factor, Sorna should use the contract's declared normalization
and repeat policy. It must record:

- random seeds;
- retry count;
- repeated outcomes;
- whether the decisive assertion was stable;
- whether the mutation classification depends on a flaky observation.

Unstable outcomes are `inconclusive` until the experiment defines a justified
classification policy.

## 12. Safety controls

Mutation execution must:

- run against disposable or isolated environments;
- never mutate the user's primary working tree without explicit opt-in;
- record the exact target revision;
- limit process, network, and resource usage;
- preserve baseline artifacts;
- stop on destructive or out-of-scope mutation attempts;
- redact secrets from observations and patches.

## 13. MVP mutation set

The first Sorna slice should implement at least:

- response status replacement;
- required JSON field removal;
- invalid-input acceptance;
- persistence skip;
- state-transition skip;
- frontend stale-state mutation if the frontend adapter is included.

It should produce a stable mutation catalogue and JSONL result file before
adding broad language-specific operator packs.
