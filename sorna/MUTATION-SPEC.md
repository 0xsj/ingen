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

## 3. Operator families

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

## 4. Mutation lifecycle

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

## 5. Outcomes

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

## 6. Baseline requirements

Mutation results are valid only when:

- the unmutated baseline passes mandatory rules;
- the baseline and mutation use the same contract version;
- the same frozen oracle is used;
- the implementation revision and mutation are recorded;
- environment differences are recorded;
- flaky or nondeterministic cases are identified.

If the baseline fails, mutation scoring should stop or be reported as
`baseline-invalid` rather than producing a misleading score.

## 7. Score and denominator

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

## 8. Contract mutations

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

## 9. Flakiness and nondeterminism

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

## 10. Safety controls

Mutation execution must:

- run against disposable or isolated environments;
- never mutate the user's primary working tree without explicit opt-in;
- record the exact target revision;
- limit process, network, and resource usage;
- preserve baseline artifacts;
- stop on destructive or out-of-scope mutation attempts;
- redact secrets from observations and patches.

## 11. MVP mutation set

The first Sorna slice should implement at least:

- response status replacement;
- required JSON field removal;
- invalid-input acceptance;
- persistence skip;
- state-transition skip;
- frontend stale-state mutation if the frontend adapter is included.

It should produce a stable mutation catalogue and JSONL result file before
adding broad language-specific operator packs.

