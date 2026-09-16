# Herdr Sentinel Plugin Specification

Status: Draft design specification

Herdr Sentinel is the orchestration layer for Sorna's contract-first,
multi-agent workflow. Sorna owns contract interpretation, oracle execution,
mutation analysis, and evidence semantics. Sentinel owns workspaces, roles,
lifecycle, artifact handoff, and operator visibility.

## 1. Responsibilities

Sentinel should:

- create named workspaces and worktrees for each role;
- attach the correct contract and policy artifacts;
- launch agents with role-specific capabilities;
- keep oracle generation separate from implementation work;
- invoke Sorna only through a stable command or adapter boundary;
- display lifecycle and verification state;
- preserve links among sessions, worktrees, artifacts, and results;
- report limitations instead of overstating independence.

Sentinel should not:

- redefine contract semantics;
- silently amend a sealed contract;
- own mutation operator behavior;
- treat pane output as tamper-proof evidence;
- claim that separate panes alone prove information isolation;
- replace Herdr's general workspace management.

## 2. Roles and capabilities

| Role | Inputs | Write scope | SUT access before freeze |
| --- | --- | --- | --- |
| Contract author | Requirements, public interface | Contract workspace | No |
| Oracle writer | Sealed contract, public fixtures | Oracle workspace | No |
| Backend implementer | Sealed contract, backend worktree | Backend worktree | Own worktree |
| Frontend implementer | Sealed contract, public API | Frontend worktree | Own worktree |
| Verifier | Contract, frozen oracle, implementations | Evidence workspace | After freeze |
| Mutation runner | Frozen oracle, disposable target | Mutation worktree | After freeze |

The plugin should generate a capability policy for each role. A role that
cannot be structurally isolated must be labeled accordingly in the run.

## 3. Workspace model

Conceptual workspace:

```text
sentinel-run/
  contract/       sealed contract and fixtures
  oracle/         isolated oracle generation workspace
  frontend/       frontend implementation worktree
  backend/        backend implementation worktree
  verifier/       verification session and reports
  mutations/      disposable mutated revisions
  evidence/       Sorna evidence bundle
  policy/         role and isolation policies
```

The actual Herdr layout may use workspaces, tabs, panes, worktrees, or future
plugin resources. These logical roles must remain stable even if the UI
changes.

## 4. Lifecycle

```text
created
  -> contract-sealed
  -> oracle-generating
  -> oracle-frozen
  -> implementations-running
  -> verifying
  -> mutating
  -> reviewed
  -> finalized
```

Failure states are explicit:

- `blocked`: a role cannot proceed and needs intervention;
- `policy-violation`: a capability or access rule was breached;
- `independence-compromised`: oracle independence cannot be claimed;
- `invalid-baseline`: implementation fails before mutation analysis;
- `aborted`: operator stopped the run;
- `cleanup-pending`: disposable resources remain.

Sentinel must not move to `oracle-frozen` without a recorded oracle hash, and
must not move to `verifying` without a successful freeze and policy check.

## 5. Candidate actions

These are conceptual plugin actions; exact Herdr API bindings may evolve.

### `sentinel bootstrap`

Creates a run ID, logical workspaces, role policies, and an evidence skeleton.

Inputs:

- contract path or contract ID;
- project root;
- requested roles;
- isolation mode;
- adapter configuration.

Output:

- run ID;
- workspace and worktree references;
- policy artifact hashes;
- initial report location.

### `sentinel seal`

Validates and seals the contract through Sorna. It records the canonical hash,
reviewers, and version lineage.

### `sentinel generate`

Launches the oracle writer with only the permitted contract inputs. It waits
for generation, performs self-consistency checks, and freezes the oracle.

### `sentinel verify`

Starts the verifier, invokes the Sorna adapter against the implementation, and
publishes rule-level results.

### `sentinel mutate`

Runs the declared mutation catalogue in disposable targets and publishes
outcomes without changing the frozen oracle.

### `sentinel audit`

Checks hashes, policy events, role lineage, access evidence, and missing
artifacts. It reports assurance level and limitations.

### `sentinel cleanup`

Stops agents and removes disposable resources only after preserving the
evidence bundle and recording cleanup status.

## 6. Role prompt contract

Sentinel should pass agents structured metadata rather than relying solely on
free-form prompts:

```json
{
  "run_id": "sorna-2026-0001",
  "role": "oracle-writer",
  "contract": {"path": "/run/input/contract.yaml", "sha256": "..."},
  "allowed_roots": ["/run/input", "/run/output"],
  "denied_roots": ["/run/backend", "/run/frontend"],
  "network": "disabled",
  "completion_artifact": "/run/output/oracle-manifest.json"
}
```

The prompt may explain the task, but the capability policy must be enforced by
the execution environment where possible.

## 7. Event model

Sentinel should emit or preserve events that connect Herdr activity to Sorna
artifacts:

- workspace created;
- worktree created and revision recorded;
- role launched;
- role blocked or completed;
- artifact produced and hashed;
- policy applied or violated;
- Sorna command started and completed;
- review approved or waived;
- cleanup completed.

Each event should include run ID, role, session/workspace reference, timestamp,
artifact references, and outcome. Herdr events supplement Sorna evidence; they
must not replace Sorna's result manifest.

The Herdr-facing event adapter belongs in the plugin integration layer, not in
Nublar or Sorna. It should normalize host lifecycle hooks into these Sentinel
events, validate the active run and artifact lineage, and update the
`ingen.sentinel-run/v1` receipt. Nublar should receive only Sentinel's shared
`ingen.ci-result/v1` projection; it must not consume host events directly.

## 8. Operator report

The first report should make the current state visible in one place:

```text
Run: sorna-2026-0001
Contract: todo-api v1 [sealed]
Oracle: frozen [hash verified]
Oracle isolation: capability-isolated
Frontend: complete
Backend: complete
Rules: 15 passed, 3 failed
Mutations: 8 killed, 1 survived, 1 equivalent
Evidence: complete
Review: pending
```

The report must distinguish:

- agent-reported state;
- Sorna-produced result;
- independently verified artifact state;
- unresolved or unavailable telemetry.

## 9. Failure and recovery

If an agent fails, Sentinel should preserve its worktree and events and allow
the role to be resumed or replaced. If the oracle role fails before freezing,
the oracle may be regenerated. If a policy violation occurs after generation,
the run must be marked `independence-compromised` and cannot be presented as a
verified independent run.

If the contract changes, Sentinel starts a new contract lineage or explicitly
records an amendment. It must not overwrite the prior run's contract hash.

## 10. MVP plugin scope

The first plugin should implement:

- `bootstrap`, `seal`, `generate`, `verify`, `mutate`, `audit`, and `cleanup`;
- one frontend role, one backend role, one oracle role, and one verifier role;
- separate worktree references;
- role-specific input and output paths;
- Sorna CLI invocation;
- run IDs and artifact hashes;
- a text report or pane showing lifecycle and evidence status;
- explicit unverified/isolation limitations.

It should not begin with a complex dashboard, automatic agent replacement, or
custom mutation engine.

## 11. Open integration decisions

- Which concrete Herdr plugin lifecycle hooks should map to Sentinel lifecycle
  events? The placement and translation boundary is recorded in
  [`sentinel-herdr-event-adapter-boundary.md`](../notes/modules/sentinel-herdr-event-adapter-boundary.md);
  the host API is not present in this repository yet. The required host-side
  inputs and acceptance gate are recorded in
  [`sentinel-herdr-host-binding-contract.md`](../notes/modules/sentinel-herdr-host-binding-contract.md).
- How should capability policies be enforced on the target platform?
- Where should evidence bundles be stored and retained?
- How should Sentinel detect that a role has completed its required artifact?
- Which Herdr events are durable enough to reference in a final report?
- Should Sorna be invoked as a CLI, local service, or library in the MVP?
- How should users review and approve contract amendments from the workspace?

No native binding should be added until the host supplies those primitives and
the adapter acceptance cases pass against the real hook implementation.
