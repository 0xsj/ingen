# Herdr Sentinel

## Working identity

Herdr Sentinel is a Herdr plugin for coordinating contract-first, multi-agent development and verification workflows.

It is the operational layer around Sorna. Sorna owns the testing framework; Sentinel owns the workspace, agent roles, isolation setup, and run visibility.

## The problem

Running multiple agents is easy to start and difficult to trust.

A frontend agent, backend agent, test-writing agent, reviewer, and mutation runner can all be active at once. Without a deliberate workspace model, they may:

- share files that should have been isolated;
- derive tests from the implementation they are meant to challenge;
- silently change the contract to make a failing test pass;
- report completion without independent evidence;
- leave the user searching through panes to discover what needs attention.

Sentinel should turn that collection of terminals into a visible, repeatable verification workflow.

## What Sentinel aims to solve

- Create and group the workspaces needed for contract-first development.
- Give frontend, backend, oracle, verifier, and mutation roles separate locations and permissions.
- Start agents with the correct contract revision, worktree, and policy context.
- Make agent state and blocked decisions visible across the project.
- Invoke Sorna runs and surface their evidence in Herdr.
- Preserve the relationship between contract, agent sessions, worktrees, tests, mutations, and results.

## Intended workflow

```text
1. Human approves and seals a contract.
2. Sentinel creates the contract, oracle, frontend, backend, and verifier workspaces.
3. The oracle writer receives the contract without implementation access.
4. The frontend and backend agents implement independently.
5. Sorna runs the frozen contract tests against both implementations.
6. Sorna runs contract and implementation mutations.
7. Sentinel reports what is working, blocked, complete, or inconclusive.
8. A human reviews failures and explicitly amends the contract when needed.
```

## Candidate actions

```text
sentinel bootstrap       create the project verification workspace
sentinel seal            record the approved contract revision
sentinel generate        launch the isolated oracle-test writer
sentinel verify          invoke Sorna against the frontend/backend targets
sentinel mutate          start a mutation campaign
sentinel audit           show the run receipt and access/evidence record
sentinel cleanup         close or remove completed workspaces safely
```

The exact interface may be Herdr plugin actions rather than a separate `sentinel` executable. The important boundary is that these actions invoke Sorna instead of reimplementing its test semantics.

## Isolation and audit model

Sentinel should help establish an information barrier, but Herdr pane visibility alone is not sufficient to guarantee one.

The preferred design is capability-based:

```text
oracle environment
├── approved contract
├── public interface
├── fixtures and examples
├── test-writing output path
└── no implementation checkout
```

The implementation is tested later by a separate runner. Tool-call transcripts, pane reads, Herdr lifecycle events, and filesystem/audit records can document what happened, but self-reported agent activity is not proof that an inaccessible file was not read.

Sentinel should therefore record both:

- **Declared evidence:** prompts, tool calls, pane output, agent/session identity, and lifecycle events.
- **Structural evidence:** allowed roots, sandbox policy, worktree provenance, contract hash, and whether the implementation was physically available to the oracle writer.

## Initial dashboard model

```text
contract        sealed
oracle-writer   done
frontend        working
backend         blocked
contract-tests  passed
mutation-gate  37/41 killed
```

The user should be able to jump directly to the agent or decision that needs attention, rather than inspect every pane manually.

## First plugin scope

The first useful Sentinel version should provide:

1. A project bootstrap action.
2. Worktree creation for frontend, backend, oracle tests, and verification.
3. A contract hash and run identifier passed to every relevant process.
4. An isolated oracle-writer launch path.
5. Sorna command invocation and result collection.
6. A concise audit/report pane.
7. Safe cleanup that never deletes branches or worktrees without explicit confirmation.

## Non-goals for the first version

- Becoming a general-purpose agent manager outside the contract workflow.
- Replacing Herdr's core workspace and agent state model.
- Treating pane output as a complete or tamper-proof tool-call trace.
- Hiding contract amendments inside automated fixes.
- Owning mutation operators that belong in Sorna or language-specific adapters.

## Open questions

- Which sandbox or restricted-runner mechanism should enforce the oracle barrier?
- How much of the agent tool trace can be captured independently for each supported agent?
- Should a contract be stored in the main repository, a dedicated contract worktree, or a separate reviewed artifact store?
- Which frontend/backend boundary should be the first end-to-end example?
- Should Sentinel expose a dashboard pane, a report popup, or both?
- What evidence is required before a human may mark a run complete?

## Relationship to Sorna

Sorna is the judge and evidence producer. Sentinel is the control room.

```text
Sorna          contract, tests, mutations, verdicts, receipts
Herdr Sentinel workspaces, agents, isolation setup, visibility, coordination
```
