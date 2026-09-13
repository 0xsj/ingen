# Sorna First Vertical Slice

Status: Proposed MVP experiment

This experiment is the smallest useful proof that Sorna can generate an
independent oracle, freeze it, evaluate a separate implementation, and use
mutation testing to expose weak coverage.

## 1. Question

Can a contract-first oracle agent produce a useful test artifact without seeing
the frontend or backend implementation, and can Sorna provide evidence that
the frozen artifact detects deliberately introduced behavioral changes?

## 2. Scope

Use one small Todo flow across an HTTP/JSON backend and a thin frontend client.
The experiment covers:

- creating a todo;
- rejecting invalid titles;
- listing a created todo;
- completing a todo;
- showing the result in a frontend view.

The browser is not the primary oracle boundary in the first run. The backend
HTTP boundary is normative; the frontend agent consumes that public contract
and the verifier may add one client-level smoke check.

## 3. Roles

| Role | Sees | Produces |
| --- | --- | --- |
| Contract author | Product requirement and public interface | Sealed contract |
| Oracle agent | Sealed contract, public fixtures, generic Sorna tooling | Oracle source and cases |
| Backend agent | Sealed contract and backend worktree | Backend implementation |
| Frontend agent | Sealed contract and public API description | Frontend implementation |
| Verifier | Contract, frozen oracle, implementations | Run and mutation evidence |
| Sentinel | Workspace metadata and artifact references | Orchestration and audit record |

The oracle agent must not receive either implementation worktree or the other
agents' transcripts.

## 4. Contract slice

The contract should define at least these rules:

| Rule ID | Requirement |
| --- | --- |
| `todo.create.valid.status` | A valid todo returns HTTP 201. |
| `todo.create.valid.shape` | The response contains a non-empty ID, the title, and `completed: false`. |
| `todo.create.empty-title` | An empty title returns HTTP 400 with `invalid_title`. |
| `todo.create.long-title` | A title beyond the declared maximum is rejected. |
| `todo.list.includes-created` | A successfully created todo appears in a later list. |
| `todo.complete.valid` | Completing an existing todo makes `completed` true. |
| `todo.complete.missing` | Completing an unknown ID returns the declared not-found result. |
| `todo.frontend.displays-state` | The frontend displays the public title and completion state. |

The contract must explicitly leave generated IDs, timestamps, and unrelated item
ordering unspecified unless they become product requirements.

## 5. Suggested workspace layout

```text
experiment/
  contract/
    todo-api.yaml
    fixtures/
  oracle/
    generated/
    frozen/
  backend/
    worktree/
  frontend/
    worktree/
  evidence/
  sentinel/
```

The oracle workspace must not contain `backend/` or `frontend/` paths. The
verifier may mount all worktrees after the oracle is frozen.

## 6. Execution protocol

### Phase A: author and seal

1. Write the narrow contract.
2. Review rule IDs, boundaries, errors, and unspecified behavior.
3. Validate the contract.
4. Seal and hash the canonical artifact.

### Phase B: generate the oracle

1. Create the isolated oracle workspace.
2. Provide only the sealed contract and public fixtures.
3. Generate fixed examples and boundary cases.
4. Run oracle self-consistency checks without SUT access.
5. Review the generated rules if required.
6. Freeze and hash the oracle and concrete cases.

### Phase C: implement independently

1. Give frontend and backend agents the sealed contract.
2. Give each implementation agent a separate worktree.
3. Allow implementation agents to test their own work locally.
4. Prevent implementation changes from modifying the frozen oracle.

### Phase D: verify

1. Start the backend and frontend at recorded revisions.
2. Run the frozen Sorna cases through the public adapter.
3. Record rule-level observations and verdicts.
4. Run implementation mutations.
5. Run contract mutations where supported.
6. Produce and verify the evidence bundle.

## 7. Deliberate defects

The reference implementation should contain a clean baseline and separately
apply these controlled defects:

| Mutation | Expected finding |
| --- | --- |
| Return 200 instead of 201 on create | `todo.create.valid.status` fails |
| Omit `completed` from create response | `todo.create.valid.shape` fails |
| Accept empty title | `todo.create.empty-title` fails |
| Accept overlong title | `todo.create.long-title` fails |
| Do not persist created todo | `todo.list.includes-created` fails |
| Ignore completion update | `todo.complete.valid` fails |
| Return 500 for missing ID | `todo.complete.missing` fails |
| Frontend renders stale completion state | `todo.frontend.displays-state` fails |

Each mutation must have a stable ID, an explicit patch or behavior description,
and a declared expected observability.

## 8. Acceptance criteria

The experiment succeeds when:

- the contract is sealed before implementation verification;
- the oracle artifact was generated without implementation workspace access;
- the oracle and cases have recorded hashes;
- the clean baseline passes all mandatory rules;
- each observable deliberate defect is killed by the relevant oracle rule;
- any surviving mutation is explained and recorded;
- the evidence bundle can be checksum-verified;
- a second verifier can replay the baseline and at least one mutation;
- the report distinguishes verified isolation from self-reported isolation.

The experiment is not a failure merely because an equivalent mutation survives;
it is a failure if the classification is hidden or unsupported.

## 9. Expected artifacts

```text
evidence/
  manifest.json
  contract/canonical.json
  contract/hash.txt
  oracle/frozen-cases.jsonl
  oracle/hash.txt
  policy/isolation.yaml
  results/rules.jsonl
  mutations/catalogue.jsonl
  mutations/results.jsonl
  events/lifecycle.jsonl
  checksums.sha256
```

## 10. Out of scope

- general-purpose browser automation;
- arbitrary programming-language support;
- distributed test execution;
- cryptographic proof of agent intent;
- complete mutation operator coverage;
- automatic contract amendment;
- claiming that a passing run proves the product is correct.

## 11. Follow-up decisions

After this experiment, decide:

- whether the contract syntax is expressive enough;
- whether the oracle result model supports frontend observations;
- which isolation controls can be enforced through Herdr;
- whether mutation operators are meaningful and deterministic;
- which evidence claims users understand and trust;
- whether Sorna should expose a stable CLI before deeper Sentinel work.

