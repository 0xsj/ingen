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

### Signed offline callback batches

The native Herdr host currently does not create signed callback envelopes.
`sentinel adapter herdr-event` and `herdr-events` remain unsigned legacy
ingress; neither command authenticates the file or its source. The opt-in
`herdr-sign-events` command signs a local JSONL event batch after validating it
against the current receipt. It is an operator signing utility, not a Herdr
host signer. If an operator signs bytes received from a host, the signature
proves only that someone with the shared key signed those bytes; it does not
prove the host produced them.

Provision a private 32-byte raw key outside the project root, owned by the
current user with mode `0600` or `0400`. Keep the same key and configured sender
name for signing and ingestion. Given a receipt and validated JSONL file:

```sh
"$SENTINEL" adapter herdr-sign-events \
  --root "$PROJECT_ROOT" \
  --receipt .ingen/artifacts/sentinel-run.json \
  --events .ingen/artifacts/herdr-events.jsonl \
  --key /absolute/private/callback.key --sender local-adapter \
  --output .ingen/artifacts/herdr-events.signed.json \
  --lifetime-seconds 300

"$SENTINEL" adapter herdr-auth-events \
  --root "$PROJECT_ROOT" \
  --receipt .ingen/artifacts/sentinel-run.json \
  --envelope .ingen/artifacts/herdr-events.signed.json \
  --key /absolute/private/callback.key --expected-sender local-adapter
```

The signer creates a new envelope exclusively. Ingestion validates the complete
envelope and event batch before atomically applying it to the receipt. The
closed envelope follows
[`ingen.herdr-signed-event-batch/v1`](spec/herdr-signed-event-batch-v1.schema.json);
each payload line follows
[`ingen.herdr-event/v1`](spec/herdr-event-v1.schema.json). Unknown fields,
duplicate keys, trailing JSON, mismatched receipt identity, and expired
batches are rejected.

The envelope's exact fields are `schema`, `sender`, `key_id`, `run_id`,
`workspace_id`, `workspace_version`, `issued_at`, `expires_at`,
`payload_sha256`, `payload_base64`, and `hmac_sha256`. Each event object is also
closed; its allowed fields are `schema`, `event_id`, `run_id`, `workspace_id`,
`workspace_version`, `type`, `at`, `role`, `workspace`, `session_id`,
`receipt_status`, `artifact_ids`, `outcome`, and `reason`.

The HMAC-SHA-256 input starts with the domain string
`ingen.sentinel.herdr-event-batch.hmac-sha256.v1` plus a NUL byte. It then
frames, in order, the schema, sender, key ID, run ID, workspace ID, decimal
workspace version, issue time, expiry time, payload SHA-256, and the exact
decoded JSONL payload. Each frame is an unsigned 8-byte big-endian byte length
followed by that field's bytes. The key ID is SHA-256 of the shared key; the
payload also has its own SHA-256. This domain-separated format uses HMAC as
specified by [RFC 2104](https://datatracker.ietf.org/doc/html/rfc2104).

The event batch is limited to 4 MiB and 1000 events; the encoded envelope is
limited to 6 MiB. CLI lifetimes range from 1 to 300 seconds. Verification
allows at most 30 seconds of clock skew into the future and rejects expiry,
nonpositive windows, or windows longer than five minutes. Share the key only
with intended signers: a holder can create valid envelopes, so this is shared
key possession authentication, not a host identity or independently verifiable
signature.

## Amber coordinator provenance

Build a persistent Sentinel executable, then create a raw Amber root and a child
around a noninteractive command:

```sh
mkdir -p .artifacts
go build -o .artifacts/sentinel ./herdr-sentinel/cmd/sentinel
SENTINEL="$PWD/.artifacts/sentinel"
"$SENTINEL" provenance start --root "$PROJECT_ROOT" \
  --output .ingen/provenance/root.json
"$SENTINEL" provenance execute --root "$PROJECT_ROOT" \
  --parent .ingen/provenance/root.json \
  --output .ingen/provenance/check-child.json \
  --receipt .ingen/provenance/check-execution.json \
  --operation local-check -- /usr/bin/true
```

The optional `--expected-parent-sha256` pins the selected root's exact bytes.
Each child has a new work and execution ID, shares the parent's correlation ID,
and records its cause through Amber's public SDK. Raw contexts retain Amber's
version 1 format. The separate execution receipt records actual process exit,
timestamps, executable and context hashes, and stdout/stderr capture hashes.
Outputs are exclusive. Wrapper decision codes are 0 for completed, 1 for failed,
and 2 for canceled or indeterminate execution; the receipt retains the actual
child exit independently.

This command inherits Sentinel's environment and project access. Its receipt
records `not-isolated` enforcement and `unverified` assurance. Coordinator
lineage does not establish independent agent context, running-image attestation,
or HTTP propagation into the subject service. The fresh
[`make ecosystem-http-check`](../acceptance/ecosystem-http/README.md) workflow
uses these contexts around actual Sorna managed HTTP verification.

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

Find the active socket with `herdr status --json` and resolve any symlink aliases
before selecting it (on macOS, `/tmp` resolves to `/private/tmp`). Sentinel
requires a canonical absolute UNIX socket owned by the caller, and checks the
connected peer's effective UID before sending request bytes. This authenticates
a local user boundary; it does not authenticate the Herdr application or host
callbacks. After bootstrapping the
receipt as above, launch a role in a new background workspace:

```sh
"$SENTINEL" session spawn --provider herdr --socket "$HERDR_SOCKET" \
  --workspace .ingen/workspace.yaml \
  --receipt .ingen/artifacts/sentinel-run.json \
  --root "$PROJECT_ROOT" --role contract-author -- your-agent-command
```

Use a noninteractive role command: the arbitrary-command wrapper supplies no stdin and captures
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
New journals include an execution lease held by the wrapper. Recovery leaves an
active wrapper's journal unchanged and returns a diagnostic. A claimed wrapper
whose lease is free becomes `indeterminate`, with no inferred exit or capture
hashes. A free lease does not prove the child has stopped. Legacy journals
without a lease retain their earlier host reconciliation behavior.

Before choosing a recovery action, inspect the journal and available evidence
without contacting Herdr or changing project files:

```sh
"$SENTINEL" session native-assess \
  --root "$PROJECT_ROOT" --path "$NATIVE_JOURNAL" --format json
```

The report binds the journal snapshot by SHA-256 and records the lease
observation, receipt status, and available terminal stdout/stderr hashes. The
assessment is read-only; related files are not one atomic filesystem snapshot,
and a free lease does not prove that a child stopped or never started. Follow
`recommended_action`: `wait-and-reassess` means an existing terminal journal's
lease was observed held; `collect-or-close` is for a terminal journal with a
free lease and matching available output evidence. Nonterminal journals report
an existing lease as `not-probed` and require `manual-review`: acquiring even a
temporary lock could interfere with a wrapper about to claim execution.
An indeterminate journal also requires manual review, without inferring an exit.
These lease observations are transient. The action
`reassess` means the journal changed during inspection; and `manual-review`
means evidence or lease status is missing, uncertain, or mismatched. Exit 0 is
an `assessed` report, exit 1 an `uncertain` report, and exit 2 an invalid
request or read failure. Assessment itself never relaunches or recovers a
session.

Inspect an owned pane without changing its journal:

```sh
"$SENTINEL" session native-process-info --socket "$HERDR_SOCKET" \
  --root "$PROJECT_ROOT" --path "$NATIVE_JOURNAL"
```

This validates the stored workspace, pane, terminal, and working directory
before reading process information. Its JSON is a local unverified host
observation; it is not execution evidence.
After a terminal outcome, the operator can close the recorded disposable Herdr
workspace by its exact ID.

For a journal with a known process outcome or proven cancellation before
start, Sentinel can verify and close that exact owned workspace:

```sh
"$SENTINEL" session native-close --socket "$HERDR_SOCKET" \
  --root "$PROJECT_ROOT" --path "$NATIVE_JOURNAL"
```

It rejects unsettled journals, including indeterminate outcomes. The explicit
socket must match the journal. `session native-snapshot --socket "$HERDR_SOCKET"`
provides a read-only host snapshot for preservation checks; it does not supply
durable event history.

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

### Fresh Codex CLI launch

The typed Codex profile supports either provider on macOS. Place a UTF-8 prompt
in an explicit role read root outside all write and deny roots. Its size must be
1–65536 bytes. Select an absolute executable; Sentinel pins its bytes before
launch. For example, after placing the implementation prompt under an allowed
contract read root:

```sh
"$SENTINEL" session spawn --provider herdr --socket "$HERDR_SOCKET" \
  --workspace .ingen/workspace.yaml \
  --receipt .ingen/artifacts/sentinel-run.json \
  --root "$PROJECT_ROOT" --role implementation --isolate \
  --agent codex --agent-executable /absolute/path/to/codex \
  --prompt-file .ingen/contract/implementation-prompt.txt
```

An optional `--agent-model NAME` selects a model token. The profile generates a
fixed noninteractive, ephemeral invocation; arbitrary command arguments and
resume/fork commands are rejected. It supplies exact prompt bytes through
stdin, rechecks the source hash before launch, and retains a separate prompt
snapshot for verification and custody. Each execution starts in unique private
scratch outside role workspace grants, with private `HOME`, `CODEX_HOME`, and
XDG directories. Other typed executions' private state remains outside the
allowed role roots. Files explicitly included in role read grants remain inputs
to that role; the launcher cannot establish the provenance of their contents.

The default profile inherits no provider credentials and disables networking.
This is a local launch boundary, with `unverified` assurance; provider context,
retention, managed Codex configuration, and running-image identity are not
attested.

Run `make sentinel-native-context-check` for the macOS offline acceptance gate.
Its executable fixture implements the selected CLI argument/stdin interface;
it proves launch boundaries and evidence handling, not model behavior.

### Scoped API-key broker for Codex

The broker and evidence paths have passed synthetic containment tests. The
installed Codex 0.160.0 currently fails contained startup at macOS managed
preferences synchronization, before a model turn. Real contained Codex work
requires a compatible runtime or a reviewed mechanism for those preferences.
The wrapper preserves this failure and grants no access to the host preference
daemon. To opt into the experimental broker profile, add these flags:

```sh
--agent-provider openai-broker --agent-model <explicit-model> \
  --agent-credential-env OPENAI_API_KEY \
  --agent-max-requests 16 --agent-max-output-tokens 4096 \
  --agent-timeout-seconds 300
```

The selected environment variable must already exist in the trusted Sentinel
process. For a native launch it must also exist in the trusted Herdr terminal
that executes the wrapper. A variable set only in the requesting CLI process
is not transported to that terminal. Missing credentials stop the launch;
credentials are never carried in wrapper arguments or evidence files. Do not
place keys in prompts, project files, or command arguments.

Sentinel holds the API key and grants the contained child outbound TCP access
at one pinned port using Seatbelt's `localhost` selector. That permission
includes local host addresses; it is not restricted to IPv4 loopback. The
broker listens only on `127.0.0.1`, and its exact HTTP endpoint and random
execution token are supplied to the child. The broker forwards only Responses requests to
`https://api.openai.com/v1/responses`, enforces the explicit model, requests
`store:false`, and rejects hosted tools, stored conversation references,
redirects, and other routes. Local function/custom tools remain under the
outer Sorna policy. Requests are serialized and bounded by count, output
tokens, body sizes, and a total deadline; these are operational limits, not a
currency budget. The default limits are shown above; maxima are 64 requests,
8192 output tokens per request, and 1800 seconds.

The immutable native wrapper pins a reserved loopback port. If another
process claims that port before execution, setup fails. The wrapper selects
no replacement endpoint. A broker report records configuration and lifecycle
counters, with `unavailable`, `closed`, or `indeterminate` status. A completed
child or a forwarded request does not establish successful model inference,
provider retention, or an independent behavioral verdict.

Run `make sentinel-codex-broker-check` for the synthetic macOS containment and
custody gate. The installed CLI protocol check is opt-in:

```sh
INGEN_CODEX_INTEGRATION_BINARY=/absolute/path/to/codex \
  go test ./herdr-sentinel/internal/codexbroker \
  -run TestInstalledCodexBrokerProtocol -count=1 -v
```

This wire check runs the CLI outside Sorna, substitutes a trusted in-process
transport for the upstream connection, and returns a synthetic Responses
stream. It establishes protocol compatibility only. The separate contained
runtime gate is:

```sh
INGEN_CODEX_CONTAINED_INTEGRATION_BINARY=/absolute/path/to/codex \
  go test ./herdr-sentinel/internal/codexbroker \
  -run TestInstalledCodexContainedBrokerProtocol -count=1 -v
```

It currently fails for the installed 0.160.0 build. Both probes use synthetic
credentials and private local state. The owner deferred the real-provider
smoke test. The provider configuration follows the
[official Codex configuration reference](https://learn.chatgpt.com/docs/config-file/config-reference).

### Verify and preserve execution evidence

After execution, select the exact report path and retain its SHA-256 from the
collection/review record. Verify the report and its referenced files, then emit
a producer CI envelope:

```sh
ROLE_REPORT=.ingen/artifacts/role-executions/<execution-id>.json
ROLE_SHA256=<reviewed-report-sha256>
"$SENTINEL" role verify --root "$PROJECT_ROOT" --path "$ROLE_REPORT" \
  --expected-sha256 "$ROLE_SHA256" \
  --ci-result .ingen/artifacts/role-ci-result.json
go run ./lockwood/cmd/lockwood import-role-execution \
  --root "$PROJECT_ROOT/.ingen/artifacts/custody" \
  --project-root "$PROJECT_ROOT" --path "$ROLE_REPORT" \
  --expected-digest "$ROLE_SHA256" --id role-<execution-id>
go run ./lockwood/cmd/lockwood verify \
  --root "$PROJECT_ROOT/.ingen/artifacts/custody" --id role-<execution-id>
```

The expected digest binds a previously selected report. Omitting it permits
first local collection of the current files; this establishes a byte snapshot,
not an independently authenticated origin. Verification requires the original
project root and referenced input files. Prelaunch executable/tool hashes
remain historical claims, rather than checks of running images.
Governed v1 role reports record the frozen oracle's hash without its path, so
standalone report verification cannot retrieve that oracle. Native collection
also has the original governed argv and rechecks the selected oracle path.

Use a distinct CI output path: producer reports and their inputs cannot be
overwritten. `role verify` returns the shared CI decision codes: 0 for a
completed command, 1 for a failed command, and 2 for cancellation or an
indeterminate outcome. The actual contained command exit stays in the report.
Failed execution evidence can still enter custody. Lockwood retains exact
report and related file bytes with explicit digest references; custody
integrity does not change the command's result.

Add the envelope as a required check in the project's Nublar workflow:

```yaml
  - id: contained-role
    tool: sentinel
    result: .ingen/artifacts/role-ci-result.json
    required: true
```

This check proves the recorded execution result and evidence integrity. Sorna's
behavioral and mutation checks remain separate required producers. Compare
compatible role envelopes with `sattler compare BEFORE AFTER`; comparison
preserves their original outcomes.

Run `make sentinel-role-evidence-check` from the InGen checkout for a repeatable
macOS acceptance check. It creates fresh temporary projects, runs actual
contained success and failure commands, and exercises CI, custody, comparison,
and rejection of damaged captures. It creates no governance approval.

### Role policy and governance bindings

Canonical role policies, captures, and `ingen.sentinel-role-execution/v1`
reports are stored under `.ingen/artifacts/role-executions/`. The immutable
session command invokes `sentinel role execute` with expected manifest, policy,
executable, and tool digests; that wrapper checks them again before execution.
Reports retain actual child outcomes and describe the enforcement scope.
For contained sessions, the native journal records the Sentinel role wrapper's
exit; the separate role report records the requested command's exit. Native
collection validates and attaches the contained report, policy, and available
captures when the recorded command is a bound Sentinel role wrapper. Custody
intake and required CI checks are explicit subsequent steps below.
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

### Installed tools and runtime diagnostics

The local bundle path is documented in [packaging](../packaging/README.md).
Use `--tool-dir /absolute/prefix/bin` on contract creation, oracle freeze,
subject verification, and evidence verification/gating to run installed tools
from a project outside the checkout. Review declared policy tools before
sealing the policy; installed mode preserves its exact grants.

Check a selected Codex executable without real credentials or provider calls:

```sh
sentinel agent diagnose --agent-executable /absolute/path/to/codex \
  --timeout-seconds 30 > codex-readiness.json
```

The JSON scope is `local-synthetic-codex-startup-and-protocol`. Exit 0 means one
mock Responses round trip and a completed CLI reply under the recorded policy;
exit 1 means an observed incompatible execution; exit 2 means invalid invocation
or indeterminate setup/execution. Private temporary state is discarded, and raw
CLI output is not published. The installed Codex 0.160.0 reports
`managed-preferences-unavailable-under-policy` with zero broker requests.
This diagnostic does not establish real inference or provider retention.

To compare a small set of explicitly selected binaries, run the same synthetic
check sequentially for each one:

```sh
sentinel agent compare \
  --agent-executable /absolute/path/to/codex-current \
  --agent-executable /absolute/path/to/codex-candidate \
  --timeout-seconds 45 > codex-compatibility.json
```

Provide one to four absolute paths. The per-candidate timeout is a whole number
of seconds from 1 through 120; the default is 45 seconds. Paths resolving to
the same canonical binary are rejected before any candidate runs. Exit 0 means
at least one candidate completed the synthetic check; inspect each candidate
because others may be `indeterminate`. Exit 1 means all candidates were
observed unsupported. Exit 2 means invalid input or no supported candidate
with at least one indeterminate result. The recorded hashes cover bounded
executable bytes before and after each probe; they do not attest to a loaded
image. The command does not read provider credentials, contact a provider,
select or replace a runtime, or fall back to another candidate.

The pinned Codex 0.160.0 managed-preferences failure remains unresolved. The
[OpenAI Codex configuration reference](https://learn.chatgpt.com/docs/config-file/config-reference)
describes mandatory administrator constraints; it does not establish a
supported bypass for this failure. Keep managed settings intact and report the
diagnostic result rather than bypassing them.

Native `session spawn --provider herdr --isolate --agent codex` with
`--agent-provider openai-broker` now runs this synthetic check automatically
after read-only role/prompt validation. Unsupported results return exit 1;
indeterminate results return exit 2. Both publish a readiness report on stderr
and stop before policy persistence, session journals, or Herdr dispatch.
Successful readiness is advisory for that executable and mock protocol scope;
actual role execution still uses its own policy and evidence checks.

`make linux-platform-probe` records kernel capability observations. Its checks
do not apply an isolation policy or make Linux enforcement available. Read the
per-feature statuses and scope notes, including enclosing-sandbox uncertainty.

### Workflow criteria

The integration is ready for a first serious trial when it can demonstrate:

- one exact contract hash approved before implementation;
- an oracle created without implementation-root access;
- an implementation evaluated only through its public boundary;
- a failing controlled defect and a killed mutation;
- a Sentinel receipt with stable role, session, artifact, and event identity;
- an audit that rejects missing or tampered artifacts; and
- a delivery result that Nublar can consume without rewriting Sorna's verdict.
