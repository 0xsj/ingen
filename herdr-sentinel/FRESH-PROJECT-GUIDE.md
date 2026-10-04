# Fresh-project integration guide

This is the first practical InGen workflow for a brand-new project. It combines
manual artifact review with Sentinel's local and native session providers.

Start with a small HTTP/JSON subject. Keep the public surface narrow enough
that a verifier can exercise it entirely from the network boundary. The first
trial should prove the handoff and isolation rules, not maximize product scope.

## 1. Prepare the project

From the InGen checkout, set `PROJECT_ROOT` to the fresh project's directory
and ask Sentinel to create `.ingen/` using the standard layout in the root
[`AGENT-GUIDE.md`](../AGENT-GUIDE.md):

```sh
PROJECT_ROOT=/path/to/project
go run ./herdr-sentinel/cmd/sentinel project init \
  --root "$PROJECT_ROOT" --id my-project
```

This creates `.ingen/workspace.yaml`, role directories, policy placeholders,
and the initial workflow declaration. The manifest is a declaration. The host
or session provider must enforce its capabilities; Sentinel's current
capability plan is explicitly `declaration-only`. The checked-in
[`workspaces/fresh-project.yaml`](workspaces/fresh-project.yaml) is the
portable reference when a project needs to customize the scaffold manually.

Run the preflight before opening agent sessions:

```sh
go run ./herdr-sentinel/cmd/sentinel project check \
  --root "$PROJECT_ROOT" --format json
```

The initial result is expected to be `incomplete` because the contract has
not been produced and local session capability enforcement is still
declaration-only. It should not be `blocked`.

## 2. Create, review, and seal the contract

The contract-author session writes `.ingen/contract/spec.malc` from the
brief and public requirements. It must not read the subject implementation.

Compile the specification, lower it to a Sorna contract, validate it, and seal
the exact canonical bytes:

```sh
go run ./herdr-sentinel/cmd/sentinel contract create \
  --root "$PROJECT_ROOT" --ingen-root "$PWD"
go run ./herdr-sentinel/cmd/sentinel contract validate \
  .ingen/contract/contract.json --root "$PROJECT_ROOT"
go run ./herdr-sentinel/cmd/sentinel contract seal \
  .ingen/contract/contract.json --root "$PROJECT_ROOT" \
  --output-dir .ingen/contract
```

The governance reviewer then approves the sealed contract by hash. Hammond
should store the contract reference and exact SHA-256 identity; it does not
reinterpret Malcolm rules or generate Sorna oracles.

## 3. Freeze the independent oracle

The oracle-writer session receives the approved contract and public fixtures,
but not the implementation root. Use the declared Sorna oracle policy and
freeze the resulting oracle under `.ingen/artifacts/oracle/`.

From the InGen checkout, the project-level command delegates to Sorna and
keeps the output under the project namespace:

```sh
go run ./herdr-sentinel/cmd/sentinel oracle freeze \
  --root "$PROJECT_ROOT" --ingen-root "$PWD"
```

Only after the oracle is frozen should the implementation session receive the
contract. If the boundary was accidentally crossed, mark the run
`independence-compromised` and regenerate the oracle.

## 4. Implement and verify

The implementation session works only in the declared implementation roots.
The verifier runs Sorna against the subject's public boundary with the frozen
oracle and subject policy. For a subject executable that listens on the
generated policy's default `127.0.0.1:8080` and exposes `/healthz`, run:

```sh
go run ./herdr-sentinel/cmd/sentinel verify \
  --root "$PROJECT_ROOT" --ingen-root "$PWD" \
  --subject-command /path/to/project/bin/subject \
  --subject-arg=-addr --subject-arg 127.0.0.1:8080
```

Preserve the evidence bundle under `.ingen/artifacts/evidence/`, then verify
and gate it:

```sh
go run ./herdr-sentinel/cmd/sentinel evidence verify \
  --root "$PROJECT_ROOT" --ingen-root "$PWD" .ingen/artifacts/evidence
go run ./herdr-sentinel/cmd/sentinel evidence gate \
  --root "$PROJECT_ROOT" --ingen-root "$PWD" \
  --output .ingen/artifacts/evidence-ci-result.json \
  .ingen/artifacts/evidence
go run ./nublar/cmd/nublar aggregate \
  --workflow "$PROJECT_ROOT/.ingen/nublar/workflow.yaml" \
  --root "$PROJECT_ROOT" \
  --output "$PROJECT_ROOT/.ingen/artifacts/nublar-result.json"
```

The mutation session then introduces controlled wrong behavior or uses the
declared mutation campaign. Record killed and surviving mutations under
`.ingen/artifacts/mutations/`.

Amber provenance and Lockwood custody should be attached to each handoff as
their integrations become active. Until then, retain stable run IDs, role IDs,
artifact paths, and SHA-256 hashes so the later adapters have a durable seam.

## 5. Sentinel commands available now

Validate the workspace and compile its declaration-only capability plan:

```sh
go run ./herdr-sentinel/cmd/sentinel workspace validate \
  "$PROJECT_ROOT/.ingen/workspace.yaml"
go run ./herdr-sentinel/cmd/sentinel workspace capabilities \
  --workspace .ingen/workspace.yaml --root "$PROJECT_ROOT"
```

Bootstrap the lifecycle receipt:

```sh
go run ./herdr-sentinel/cmd/sentinel run bootstrap \
  --workspace .ingen/workspace.yaml --root "$PROJECT_ROOT" \
  --output "$PROJECT_ROOT/.ingen/artifacts/sentinel-run.json"
```

Spawn a local role process with stable identity and captured output:

```sh
go run ./herdr-sentinel/cmd/sentinel session spawn \
  --workspace .ingen/workspace.yaml \
  --receipt .ingen/artifacts/sentinel-run.json \
  --root "$PROJECT_ROOT" --role contract-author -- your-agent-command
```

The local provider records `declaration-only` enforcement; it does not create
a sandbox. The receipt can also receive provider-neutral Herdr events and
registered artifacts through the existing `adapter herdr-event`, `run
artifact`, `run audit`, and `run report` commands.

List and inspect the resulting sessions:

```sh
go run ./herdr-sentinel/cmd/sentinel session list --root "$PROJECT_ROOT"
go run ./herdr-sentinel/cmd/sentinel session status \
  --root "$PROJECT_ROOT" \
  --path .ingen/artifacts/sessions/<session-id>.json

go run ./herdr-sentinel/cmd/sentinel run status \
  --receipt .ingen/artifacts/sentinel-run.json \
  --root "$PROJECT_ROOT" --status completed
```

## Native Herdr session operations

The opt-in native provider targets Herdr 0.9.3, socket protocol 22. Run it
inside a managed Herdr terminal (`HERDR_ENV=1`). Build a persistent Sentinel
binary first; an ephemeral `go run` executable is unsuitable for an asynchronous
wrapper launch:

```sh
mkdir -p .artifacts
go build -o .artifacts/sentinel ./herdr-sentinel/cmd/sentinel
SENTINEL="$PWD/.artifacts/sentinel"
HERDR_SOCKET=/absolute/path/to/herdr.sock
```

Find the active socket with `herdr status --json`. After bootstrapping the
receipt as above, launch a role in a new background workspace:

```sh
"$SENTINEL" session spawn --provider herdr --socket "$HERDR_SOCKET" \
  --workspace .ingen/workspace.yaml \
  --receipt .ingen/artifacts/sentinel-run.json \
  --root "$PROJECT_ROOT" --role contract-author -- your-agent-command
```

Use a noninteractive role command: the wrapper supplies no stdin and captures
stdout/stderr in files. Interactive agent terminal interfaces are not supported
by this capture path yet. The command prints the journal path. Set
`NATIVE_JOURNAL` to that exact path:

```sh
NATIVE_JOURNAL=.ingen/artifacts/native-sessions/<session-id>.json
"$SENTINEL" session native-status \
  --root "$PROJECT_ROOT" --path "$NATIVE_JOURNAL" --format json
"$SENTINEL" session native-collect \
  --root "$PROJECT_ROOT" --path "$NATIVE_JOURNAL" \
  --receipt .ingen/artifacts/sentinel-run.json
```

Collection requires a terminal session. When a child ran, it verifies the
captured output hashes and attaches the exact journal and logs to the lifecycle
receipt. A session stopped before launch attaches its journal without claiming
process evidence. A signal-terminated child has `exit_code: -1`; the reason
records the signal. Process completion is a role outcome; behavioral
verification remains Sorna's responsibility.

Cancellation and recovery operate on the journal's exact host identities and
socket:

```sh
"$SENTINEL" session native-cancel --socket "$HERDR_SOCKET" \
  --root "$PROJECT_ROOT" --path "$NATIVE_JOURNAL"
"$SENTINEL" session native-recover --socket "$HERDR_SOCKET" \
  --root "$PROJECT_ROOT" --path "$NATIVE_JOURNAL"
```

Cancellation is a request until the wrapper records its outcome. Recovery
reconciles existing host identity and never resends a command or interprets a
pane's UI status as proof of completion. An uncertain launch remains explicit.
After a terminal outcome, the operator can close the recorded disposable Herdr
workspace by its exact ID.

Native journals retain `declaration-only` enforcement and `unverified`
assurance. Their event IDs and times belong to Sentinel's durable journal;
they are not host callback identities. Child access enforcement is selected
separately with `--isolate`, as described below.

## Contained and governed role commands

On macOS, add `--isolate` to run a noninteractive child through Sorna's Seatbelt
boundary. This works with either session provider. For example:

```sh
"$SENTINEL" session spawn --provider herdr --socket "$HERDR_SOCKET" \
  --workspace .ingen/workspace.yaml \
  --receipt .ingen/artifacts/sentinel-run.json \
  --root "$PROJECT_ROOT" --role implementation --isolate -- /bin/echo ready
```

The child receives the selected role's declared read, write, and deny roots,
plus fresh private scratch space. A write grant does not imply a read grant:
the default implementation role can write `src`, but reading it requires an
explicit manifest change. The command starts in private scratch; use absolute
project paths in its arguments. Its environment has a fixed system `PATH` and
private `HOME`, temporary directory, and cache. Explicit child tools are
selected with repeatable `--allow-tool /absolute/path` flags and bound by hash.
Network access is disabled. Unsupported enforcement fails before child launch.

Canonical role policies, captures, and `ingen.sentinel-role-execution/v1`
reports are stored under `.ingen/artifacts/role-executions/`. The immutable
session command invokes `sentinel role execute` with expected manifest, policy,
executable, and tool digests; that wrapper checks them again before execution.
Reports retain actual child outcomes and describe the enforcement scope.
For contained sessions, the native journal records the Sentinel role wrapper's
exit; the separate role report records the requested command's exit. Collection
of the native journal does not by itself register the role policy and report
with custody or make them required Nublar producers.
They remain `unverified`: runtime bootstrap reads and ancestor metadata are
available, the process namespace is shared, prior context is outside this
boundary, and no independent host attestation is supplied.
Executable and tool digests are checks of bytes before launch; they do not
attest the running image or every later tool invocation. Tool runtime data
also needs explicit capabilities; selecting a tool does not grant reads of
neighboring files. On current macOS, `/bin/sh` invokes `/bin/bash` internally;
select `/bin/bash` directly or explicitly authorize that additional executable.

For governed stages, select an existing Hammond approval record and the active
review policy explicitly. The operator must have reviewed the exact sealed
contract bytes; these commands do not create an approval:

Use the [Hammond operator guide](../hammond/usage.md) to establish the review
policy, authority snapshot, and approval lifecycle. Keep its artifact URIs
relative to this project root, including references nested in the policy.

```sh
"$SENTINEL" oracle freeze --root "$PROJECT_ROOT" --ingen-root "$PWD" \
  --governed --approval .ingen/governance/approval.json \
  --review-policy .ingen/governance/review-policy.json

"$SENTINEL" session spawn --provider herdr --socket "$HERDR_SOCKET" \
  --workspace .ingen/workspace.yaml \
  --receipt .ingen/artifacts/sentinel-run.json \
  --root "$PROJECT_ROOT" --role implementation --isolate --governed \
  --approval .ingen/governance/approval.json \
  --review-policy .ingen/governance/review-policy.json -- your-command
```

Approval, policy, and authority references must be normalized paths beneath
the project root. Hammond replays the approval lifecycle and checks exact
contract identity and active policy bytes. Oracle generation requires that
approval; implementation, verification, and mutation roles also require a
frozen oracle matching the approved contract and current oracle policy. The
role wrapper rechecks the selected digests before launching the child.
Contract and governance authoring are bootstrap work before these transitions.
Without `--governed`, existing development commands do not require approval.
The current governance gate uses a local authority snapshot; it does not
authenticate an organization or verify authority signatures against a trust store.

## Current core workflow

The manual sequence should eventually become:

```text
sentinel project init
sentinel contract create
sentinel contract validate
sentinel contract seal
sentinel oracle freeze
# implementation work happens here
sentinel verify --subject-command <subject>
sorna evidence verify <evidence-directory>
sorna gate <evidence-directory>
```

The session-spawn and receipt commands can wrap the role handoffs, but they are
not required for the core proof. The native Herdr provider wraps those handoffs
without changing the Malcolm, Sorna, or evidence artifacts.

## Acceptance gate for the first fresh project

The integration is ready for a first serious trial when it can demonstrate:

- one exact contract hash approved before implementation;
- an oracle created without implementation-root access;
- an implementation evaluated only through its public boundary;
- a failing controlled defect and a killed mutation;
- a Sentinel receipt with stable role, session, artifact, and event identity;
- an audit that rejects missing or tampered artifacts; and
- a delivery result that Nublar can consume without rewriting Sorna's verdict.
