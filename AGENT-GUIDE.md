# InGen agent guide

This guide is the operating agreement for using InGen on a fresh project. It
is written for an agent that is asked to create a contract, implement a
subject, and prove the subject without allowing the implementation to define
its own test.

## Objective

For each project, preserve this order:

1. Capture the public behavior as an independent Malcolm specification.
2. Validate and canonicalize the specification through Malcolm and Sorna.
3. Have a human or governance agent approve the exact contract snapshot.
4. Freeze the oracle before the implementation agent can inspect it.
5. Build the subject behind its public boundary.
6. Run black-box verification, then mutation and evidence checks.
7. Preserve provenance, custody, and delivery results for later review.

Passing tests are evidence about the declared contract and observed run. They
are not a claim that the implementation is universally correct.

## Ownership map

| Concern | Owner | Agent-facing responsibility |
| --- | --- | --- |
| Specification syntax and IR | Malcolm | Write and explain the executable specification. |
| Contract validation, sealing, oracle, mutation, evidence | Sorna | Be the authority for behavioral verification. |
| Approval, amendment, authority, lineage | Hammond | Review and approve the exact contract identity. |
| Role workspaces and lifecycle | Herdr Sentinel | Coordinate sessions, handoffs, and status. |
| Architecture constraints | Paddock | Check dependency and boundary rules independently. |
| Provenance | Amber | Record work identity, causality, attribution, and retries. |
| Artifact custody | Lockwood | Store hashes, custody events, lineage, and attestations. |
| CI and delivery collection | Nublar | Aggregate producer results and project delivery decisions. |
| Comparison and regression analysis | Sattler | Compare runs and correlate changes without rewriting verdicts. |

An agent must not move verification, mutation, governance, or custody
authority into Sentinel merely because Sentinel coordinates the workflow.

## Fresh-project layout

Create the following project-local namespace before starting role sessions:

```text
.ingen/
  brief.md
  contract/
    spec.malc
    spec.ir.json
    contract.json
    canonical.json
    hash.txt
  governance/
    approval.json
  policy/
    oracle.yaml
    subject.yaml
  fixtures/
  artifacts/
    sentinel-run.json             # optional Herdr lifecycle receipt
    oracle/
    evidence/
    evidence-ci-result.json
    nublar-result.json
    mutations/
    provenance/
    custody/
  sessions/
    contract-author/
    governance-reviewer/
    oracle-writer/
    implementation/
    verifier/
    mutation-runner/
  nublar/
    workflow.yaml
```

The directories are role and artifact boundaries, not proof of isolation by
themselves. A host integration must enforce the read/write policy and record
the session identity.

## Role instructions

### Contract author

Read the project brief and public requirements. Write the contract in
`.ingen/contract/spec.malc`. Do not inspect or import the implementation.
Define observable behavior, failure behavior, stable identifiers, and the
evidence needed to distinguish a real pass from a shallow one.

### Governance reviewer

Review the proposed contract for scope, authority, ownership, and ambiguity.
Approve or reject the exact canonical contract bytes. An approval must include
the contract hash and an amendment path; editing a sealed contract silently is
not allowed.

### Oracle writer

Read only the approved contract and public fixtures. Generate the oracle with
Sorna while the implementation roots remain unavailable. Freeze the oracle and
record its hash before implementation work begins.

### Implementation agent

Read the sealed contract and permitted project materials. Implement only under
the declared implementation roots. Do not edit the contract, oracle, evidence,
or custody records to make a run pass.

### Verifier

Run Sorna against the public boundary using the frozen oracle and subject
policy. Preserve the complete evidence bundle and report the producer-owned
verdict without broadening its meaning.

### Mutation runner

Use disposable controlled defects or mutation operators against the subject.
Record which declared rules are killed or survive. A clean baseline is not
credible until the verification process demonstrates sensitivity to a relevant
wrong behavior.

## Current local path

The current repository can exercise the handoffs manually. From the InGen
checkout, set `PROJECT_ROOT` to the fresh project's directory and use the
following sequence:

```sh
# Create the project-local scaffold once.
PROJECT_ROOT=/path/to/project
go run ./herdr-sentinel/cmd/sentinel project init \
  --root "$PROJECT_ROOT" --id my-project

# Check the declarations before asking an agent to act.
go run ./herdr-sentinel/cmd/sentinel project check \
  --root "$PROJECT_ROOT" --format json

# Validate the Sentinel role and policy declaration.
go run ./herdr-sentinel/cmd/sentinel workspace validate \
  "$PROJECT_ROOT/.ingen/workspace.yaml"

# Compile Malcolm source to IR, lower it to a Sorna contract, and validate it.
go run ./herdr-sentinel/cmd/sentinel contract create \
  --root "$PROJECT_ROOT" --ingen-root "$PWD"

# Validate and seal the exact contract snapshot through Sorna.
go run ./herdr-sentinel/cmd/sentinel contract validate \
  .ingen/contract/contract.json --root "$PROJECT_ROOT"
go run ./herdr-sentinel/cmd/sentinel contract seal \
  .ingen/contract/contract.json --root "$PROJECT_ROOT" \
  --output-dir .ingen/contract

# Freeze the oracle while the implementation root is still out of scope.
go run ./herdr-sentinel/cmd/sentinel oracle freeze \
  --root "$PROJECT_ROOT" --ingen-root "$PWD"

# The implementation agent now builds the subject under the declared root.
# After it returns, the verifier runs only through the public HTTP boundary.
go run ./herdr-sentinel/cmd/sentinel verify \
  --root "$PROJECT_ROOT" --ingen-root "$PWD" \
  --subject-command /path/to/project/bin/subject \
  --subject-arg=-addr --subject-arg 127.0.0.1:8080

# Independently verify and gate the resulting evidence bundle, then write the
# shared CI result Nublar consumes.
go run ./herdr-sentinel/cmd/sentinel evidence verify \
  --root "$PROJECT_ROOT" --ingen-root "$PWD" .ingen/artifacts/evidence
go run ./herdr-sentinel/cmd/sentinel evidence gate \
  --root "$PROJECT_ROOT" --ingen-root "$PWD" \
  --output .ingen/artifacts/evidence-ci-result.json \
  .ingen/artifacts/evidence

# Nublar can aggregate the Sorna result even when Herdr is unavailable.
go run ./nublar/cmd/nublar aggregate \
  --workflow "$PROJECT_ROOT/.ingen/nublar/workflow.yaml" \
  --root "$PROJECT_ROOT" \
  --output "$PROJECT_ROOT/.ingen/artifacts/nublar-result.json"

# Check the declaration-only capability handoff and create a lifecycle receipt.
go run ./herdr-sentinel/cmd/sentinel workspace capabilities \
  --workspace .ingen/workspace.yaml --root "$PROJECT_ROOT"
go run ./herdr-sentinel/cmd/sentinel run bootstrap \
  --workspace .ingen/workspace.yaml --root "$PROJECT_ROOT" \
  --output "$PROJECT_ROOT/.ingen/artifacts/sentinel-run.json"

# Launch one role through the local process provider.
go run ./herdr-sentinel/cmd/sentinel session spawn \
  --workspace .ingen/workspace.yaml \
  --receipt .ingen/artifacts/sentinel-run.json \
  --root "$PROJECT_ROOT" --role contract-author -- your-agent-command

# Inspect role handoffs after a session returns.
go run ./herdr-sentinel/cmd/sentinel session list --root "$PROJECT_ROOT"

# Once required artifacts exist, close the multi-role receipt for audit.
go run ./herdr-sentinel/cmd/sentinel run status \
  --receipt .ingen/artifacts/sentinel-run.json \
  --root "$PROJECT_ROOT" --status completed
```

The `sentinel contract create` command owns the Malcolm-to-Sorna handoff. The
resulting canonical bytes are validated, sealed, and hash-bound before oracle
or implementation work. The project ID and Malcolm contract ID must agree
with the generated policy subject ID; otherwise Sorna correctly rejects the
oracle freeze.

## Execution and evidence boundaries

Sentinel supports local process sessions and native Herdr sessions. Native
launches use an owned Sentinel journal for lifecycle evidence; raw Herdr UI
events remain observations. The current host event stream does not provide
durable history, host event IDs, or host event timestamps. Snapshot recovery
therefore cannot prove missing historical transitions.

On macOS, `session spawn --isolate` applies the declared filesystem grants,
tool restrictions, sanitized environment, and disabled networking to a
noninteractive child. Add `--governed --approval <path> --review-policy <path>`
to require Hammond's exact approved contract and the role's frozen-oracle
prerequisites. Local authority snapshots are verified without organization
authentication or signature trust-store validation.

The optional typed `--agent codex` profile requires `--isolate`, an absolute
`--agent-executable`, and a bounded read-only `--prompt-file`. It generates fixed
ephemeral CLI arguments, pins exact stdin bytes, and uses unique private local
state outside role workspace grants. Its default mode provides no credentials
or networking. The experimental `--agent-provider openai-broker` mode keeps the
API key in Sentinel, gives the child an execution token, and enforces a fixed
Responses endpoint plus explicit model and request/output/deadline limits.
Seatbelt grants its `localhost` selector at one pinned port; this includes local
host addresses, while the broker binds only IPv4 loopback. Reports preserve this
scope limitation and local request counters without claiming provider inference.
The installed Codex 0.160.0 fails contained startup at managed preferences
synchronization. Real contained runtime support and provider context attestation
remain open; the owner deferred real-provider testing. See the fresh-project
guide for the synthetic gate and separate installed CLI compatibility checks.

The role execution report is separate from the native wrapper receipt. Both
retain `unverified` assurance: process namespace isolation, independent host
attestation, and loaded-image attestation remain open. Arbitrary commands do
not have the typed profile's fresh local state guarantee. New native journals
use a wrapper execution lease: recovery leaves an active wrapper unchanged,
and a disappeared claimed wrapper becomes indeterminate without a guessed exit.
Local UNIX socket peer checks establish a same-effective-UID boundary, not
application or callback authenticity. Actual child exit codes and cancellation remain distinct from the
wrapper's outcome. Unsupported containment platforms fail closed.

`sentinel role verify` checks the rooted report and related exact bytes before
producing a shared CI result. `lockwood import-role-execution` preserves those
bytes with custody lineage. An intact failed execution can be valid custody
evidence; custody does not change its execution verdict. Nublar consumes the
producer result, and Sattler compares results without inventing causality.

`sentinel project check` can report structural coherence while the project
remains `incomplete`. Declaration checks do not prove enforcement or a
completed release. See [the finish-line plan](docs/finish-line-plan.md) for
verified checkpoints and the remaining native recovery, Linux, packaging,
and release requirements.

For a repeatable handoff check, run `make ecosystem-http-check` on macOS.
Its approval record and architecture policy are explicitly test fixtures.
For a real project, supply the reviewed artifacts for that project; a successful
fixture run does not authorize its contract or seal its architecture policy.
Amber identities around coordinator commands describe work lineage, without
proving the HTTP subject received or propagated that context.

To exercise GitHub Checks delivery with an installed Nublar binary, run:

```sh
acceptance/github-checks.sh \
  --bin /absolute/prefix/bin/nublar \
  --ecosystem-proof /absolute/path/to/ingen-ecosystem-http-proof
```

The proof directory comes from `make ecosystem-http-check`; the script delivers
its clean and defective decisions to a local synthetic API with a synthetic
token. It makes no GitHub calls. Nublar scans a configured window of at most
1000 matching list results, sends a create request once, and reconciles an
ambiguous create response before updating a unique matching run. Upstream
visibility or retention may omit older checks. This bounds accidental
duplicates within that window, but does not provide durable exactly-once
delivery or protect concurrent publishers. See GitHub's
[Checks API](https://docs.github.com/en/rest/checks/runs) and
[pagination guide](https://docs.github.com/en/rest/using-the-rest-api/using-pagination-in-the-rest-api).

## Completion protocol

Use `sentinel agent diagnose --agent-executable <absolute-path>` to check local
contained startup and mock protocol compatibility before a native launch. It
uses synthetic authentication and never calls a provider. Support is limited
to that diagnostic's exact scope. The selected installed runtime remains an
open release gate when this check reports unsupported or indeterminate.

Native isolated Codex launches using `openai-broker` perform that synthetic
check automatically after role/prompt validation and before project writes or
Herdr dispatch. Preserve unsupported/indeterminate results and their scope;
readiness does not establish real inference or attest the eventual role policy.

Ecosystem sources use MIT, retaining Amber's existing notices and dependency
licenses. All packaged commands expose `version --format json`. Distribution
manifests fingerprint selected build inputs copied into a verified private
snapshot. This identifies those inputs without attesting compiler images or
the complete checkout. Authenticate a standalone archive helper separately
before executing it; an untrusted helper cannot authenticate itself.

For local bundles and checkout-independent Sentinel invocations, see
[packaging](packaging/README.md). Kernel capability reports from
`make linux-platform-probe` describe observed interfaces; they do not establish
Linux isolation. Preserve unsupported outcomes instead of weakening grants.

An agent session is complete only when it leaves a reviewable artifact in its
declared write roots, includes the run and role identity, and reports the next
allowed transition. The coordinator should reject missing, conflicting, or
out-of-scope artifacts and should never infer success from pane output alone.
