# Sorna

## Working identity

Sorna is a standalone contract/oracle testing framework for software built with independent agents or parallel implementation teams.

The name refers to Isla Sorna: the experimental Site B behind the public park. It signals a controlled laboratory for creating, observing, and learning from behavioural variants.

## The problem: the closed circle

An implementation and its tests can be mutually consistent while both are wrong.

An implementation agent sees the requirements, makes a mistaken interpretation, and writes code. A second agent then reads the implementation, its public shape, or its tests and generates tests that describe what the code already does. The suite passes, but the test suite has not independently established that the software does what was intended.

This is a closed circle:

```text
implementation → tests → passing implementation
```

The failure is not that the agents are dishonest. The failure is that the implementation becomes an unacknowledged oracle.

Sorna aims to open the circle:

```text
reviewed contract → isolated oracle tests → implementation under test
```

## What Sorna aims to solve

- Preserve a reviewed contract as the source of intended behaviour.
- Generate or execute tests from the contract without implementation access.
- Exercise frontend and backend implementations as black boxes where practical.
- Detect missing, weakened, or incorrect behaviour through deliberate mutations.
- Distinguish evidence from an agent's own claim that it followed the rules.
- Produce a run record that another person can inspect and reproduce.

## Product model

Sorna is not primarily another source-level mutation runner. Mutation testing is one part of a larger contract/oracle workflow.

```text
contract/specification
        ↓
oracle test generation
        ↓
frozen contract tests
        ↓
black-box implementation verification
        ↓
contract and implementation mutation testing
        ↓
evidence report
```

### 1. Contract and oracle

The contract describes intended behaviour: inputs, outputs, invariants, boundaries, failure semantics, and explicit ambiguities.

The contract must have provenance. A document written after seeing the implementation is not equivalent to a pre-implementation contract, even if the prose is accurate.

### 2. Information barrier

The oracle writer should receive only the approved contract, public interface, fixtures, and required test-writing capabilities.

The meaningful claim is not:

> The agent says it did not read the implementation.

It is:

> The agent could not read the implementation.

Transcript and tool-call records are useful evidence, but they are not a security boundary. Stronger enforcement comes from capability restrictions, a separate test package, a restricted checkout, a sandbox, or an external test-generation environment.

### 3. Frozen tests

The tests are preserved exactly as written before anyone runs them against the implementation. Compile fixes may be made mechanically, but expectations must not be changed to make the implementation pass.

### 4. Mutation testing

Sorna should support two mutation planes:

- **Contract mutations:** change a required field, type, enum, boundary, status, or error clause and verify that the test suite or review gate detects the change.
- **Implementation mutations:** introduce wrong statuses, missing fields, invalid validation, incorrect boundaries, stale frontend state, or other contract violations and run the tests against the mutated system.

A mutant is killed when a valid test run fails in response to the intended behavioural change. A surviving mutant is a possible testing gap, not automatically a defect: equivalent mutants, invalid mutants, timeouts, and inconclusive runs remain distinct outcomes.

Mutation testing measures sensitivity. It does not prove that the contract is complete or correct. A suite can strongly defend a flawed contract, so independent contract review remains necessary.

## Initial evidence model

Each run should retain enough information for someone who was not present to check the result:

```text
contract revision and hash
oracle inputs and provenance
capability policy / allowed roots
oracle output before execution
implementation revision(s)
test runner and environment
mutation catalogue and outcomes
tool/event/access evidence
human decisions and contract amendments
```

Suggested outcome vocabulary:

| Outcome | Meaning |
| --- | --- |
| Baseline passed | The unmutated subject satisfied the frozen checks in this environment |
| Mutant killed | A valid behavioural change triggered the intended failure evidence |
| Mutant survived | The checks did not reject that valid changed subject |
| Equivalent | The mutation was reviewed as behaviour-preserving |
| Invalid | The mutated subject could not be built or exercised under the run rules |
| Timeout | The defined execution budget was exceeded |
| Inconclusive | Evidence was missing or conflicting |

## First experiment

Start with one bounded frontend/backend boundary and one machine-readable contract format.

1. Write and review a small contract before implementation.
2. Materialise an oracle environment containing only the contract, examples, and public interface.
3. Generate contract tests without compilation or implementation access.
4. Freeze the tests and record the prediction about likely failures.
5. Implement the backend and frontend independently.
6. Run the frozen tests against both.
7. Run a small, curated set of contract and implementation mutations.
8. Record killed, survived, invalid, equivalent, timeout, and inconclusive results.

## Non-goals for the first version

- Replacing every language-specific mutation engine immediately.
- Treating line coverage as correctness.
- Treating an agent transcript as proof of isolation.
- Automatically amending a contract to match an implementation.
- Promising that an isolated test generator has discovered the complete intended behaviour.

## Relationship to Herdr Sentinel

Sorna should be usable without Herdr. Herdr Sentinel should provide the convenient multi-agent workspace around Sorna: worktrees, panes, agent roles, isolation setup, lifecycle state, notifications, and evidence navigation.

The LLM may generate tests. The deterministic runner and evidence model decide whether the run passed.
