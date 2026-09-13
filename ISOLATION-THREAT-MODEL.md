# Sorna and Sentinel Isolation Threat Model

Status: Draft design specification

This document describes the threats to oracle independence and the controls
needed to make an agent's lack of implementation access credible.

The security claim is intentionally narrow:

> The oracle was generated and frozen using only the contract, explicitly
> permitted public inputs, and approved tooling, before it evaluated the
> implementation under test.

This is not a claim that the agent is generally trustworthy or that the
contract is correct.

## 1. Assets

The system protects:

- the sealed contract and its version lineage;
- the independence of generated oracle rules;
- implementation source and private design information;
- frozen oracle artifacts;
- execution observations and mutation results;
- evidence provenance and access policy;
- secrets and personal data in workspaces and logs;
- the user's ability to distinguish verified from self-reported results.

## 2. Actors

- **Contract author:** writes and seals the behavioral contract.
- **Oracle agent:** generates or authors oracle rules and cases.
- **Implementation agents:** build frontend and backend systems.
- **Verifier:** runs frozen oracles and mutation experiments.
- **Sentinel:** orchestrates workspaces, policies, and lifecycle.
- **Sorna runner:** executes adapters, rules, and mutations.
- **Host/platform:** provides processes, filesystem, network, and event
  controls.
- **Malicious or mistaken agent:** may intentionally or accidentally seek
  implementation information or alter evidence.

The initial threat model treats agents and implementation repositories as
untrusted with respect to oracle independence. The host policy and artifact
store are trusted only to the degree documented by the assurance level.

## 3. Trust boundaries

```text
                    trusted contract store
                              |
                              v
                    +-------------------+
                    | oracle workspace  |
                    | contract + public |
                    | fixtures only     |
                    +---------+---------+
                              |
                   frozen oracle artifact
                              |
                              v
                    +-------------------+
                    | verification      |
                    | runner             |
                    +---------+---------+
                              |
                 public observations from SUT
                              |
             +----------------+----------------+
             |                                 |
       implementation                    evidence store
       workspaces                         and Sentinel
```

Oracle generation and implementation workspaces are separate trust domains.
The verifier may see both, but must not allow implementation results to flow
back into oracle generation after freezing.

## 4. Threats and controls

| Threat | Example | Primary control | Evidence |
| --- | --- | --- | --- |
| Source inspection | Oracle agent searches backend files | Separate workspace and denied root | Sandbox policy and access events |
| Path discovery | Agent lists parent directories or worktrees | Capability-scoped filesystem and neutral paths | Denied attempts |
| Transcript leakage | Implementation agent output is included in oracle context | Separate sessions and artifact-only handoff | Session lineage |
| Cache leakage | Shared build or editor cache contains source | Isolated cache and environment | Environment manifest |
| Network leakage | Oracle agent fetches repository or endpoint metadata | Network disabled or allowlisted | Network events |
| Process leakage | Agent inspects process args or environment | Process namespace and secret filtering | Process policy |
| Symlink escape | Permitted fixture links outside workspace | Resolve and validate real paths | Workspace audit |
| Generated-artifact leakage | Implementation-generated schema is supplied as fixture | Provenance-required fixture promotion | Fixture hashes and source |
| Result adaptation | Oracle changes expectations after failures | Freeze before SUT access | Freeze event and hash |
| Evidence tampering | Agent rewrites results or logs | Append-only events and checksums | Manifest verification |
| Contract drift | Contract silently changes during run | Seal/version/hash contract | Contract lineage |
| Over-assertion | Oracle asserts volatile fields or snapshots | Unspecified fields and normalization policy | Rule review |
| Under-assertion | Oracle omits invalid and boundary behavior | Contract quality checks and mutation tests | Coverage and survivors |
| Secret exposure | Headers or logs contain tokens | Redaction and secret-aware adapters | Redaction records |

## 5. Required oracle capabilities

The oracle-generation process should have:

- read access to the sealed contract;
- read access to explicitly attached public fixtures;
- write access to its own output directory;
- access to deterministic generators and the Sorna SDK;
- no implementation checkout;
- no access to implementation transcripts;
- no unrestricted shell access if the platform can avoid it;
- no network access by default;
- no ability to invoke the SUT before freezing.

The process may receive a generic adapter description, but must not receive
implementation-derived metadata such as generated OpenAPI output or coverage.

## 6. Required verifier capabilities

The verifier may access:

- sealed contract and frozen oracle;
- implementation worktrees or images;
- public runner endpoint;
- mutation engine;
- evidence store.

The verifier must not mutate the frozen oracle during normal execution. A
change creates a new oracle revision and a new run lineage.

## 7. Information-flow rules

During generation:

```text
contract + public fixtures + generic tooling
                    |
                    v
             oracle artifact
```

The following flows are prohibited before freeze:

```text
implementation -> oracle agent
implementation failures -> oracle expectations
coverage/mutation results -> oracle generation
private transcripts -> oracle workspace
```

After freeze, the runner may send public observations to the oracle evaluator,
but it must not send source or private state.

## 8. Assurance tiers

### Development mode

Separate prompts and worktrees are used, but platform enforcement is limited.
Results must be labeled `independence-unverified`.

### Isolated mode

Filesystem, network, process, and workspace capabilities are enforced. Policy
and access events are recorded. This supports `capability-isolated` evidence.

### Attested mode

An independent trusted runtime attests to the process image, policy, and event
stream. This is future scope and should not be implied by ordinary Herdr or
terminal logs.

## 9. Verification checks

Before an isolated run, Sentinel or Sorna should verify:

- contract status is `sealed`;
- contract and fixtures match recorded hashes;
- oracle workspace contains only permitted inputs;
- implementation roots are absent from the oracle namespace;
- network policy is applied;
- tool policy is applied;
- output directory is writable and isolated;
- oracle generation cannot invoke the SUT;
- the clock/order metadata is available;
- event and artifact sinks are reachable.

After generation and before SUT access, verify:

- oracle artifact exists;
- oracle artifact hash is recorded;
- concrete cases are frozen;
- no policy violation occurred;
- all required generation checks passed;
- the run can transition only to verification or an explicit failure state.

## 10. Threats not solved by tool-call logs

Tool-call logs are useful evidence, but they do not independently guarantee
that an agent did not inspect implementation code. Logs may be incomplete,
altered, or unable to observe an indirect access through a tool or cache.

The system must distinguish:

- **observed non-access:** the platform saw no access event;
- **enforced non-access:** the platform denied access by policy;
- **unobserved access possibility:** the platform cannot establish what happened;
- **attested non-access:** a trusted external mechanism made the claim.

Only the strongest supported statement should appear in the final report.

## 11. Incident handling

If a policy violation, unexpected access, or evidence mismatch occurs:

1. Mark the run `independence-compromised`.
2. Preserve the original events and artifacts.
3. Stop oracle generation or verification as appropriate.
4. Do not silently regenerate expectations.
5. Record the suspected flow and affected artifacts.
6. Start a new run after the policy or workspace is corrected.

An incident may invalidate one run without invalidating the contract itself.

## 12. MVP controls

The first vertical slice should implement:

- separate oracle and implementation worktrees;
- a generated allowlist of input paths;
- a denied implementation root;
- network-disabled oracle generation;
- pre-freeze generation marker;
- artifact hashes;
- explicit `independence-unverified` versus `capability-isolated` status;
- access-event capture where supported;
- incident invalidation.

It should not claim complete tamper-proofing until the host enforcement and
evidence architecture justify that claim.

