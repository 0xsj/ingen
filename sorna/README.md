# Sorna

Sorna is InGen's standalone verification laboratory. It is the part that
turns a sealed behavioral contract into a frozen oracle, evaluates a system
through a public boundary, runs mutation campaigns, and produces evidence.

The current files in this directory are design specifications plus the first Go
implementation slices. The contract package and CLI can load, validate,
canonicalize, and seal a contract locally or in CI without Herdr. Sorna can
also freeze a deterministic oracle in a sandbox and checksum its evidence.

The current document-pipeline alpha stopping point is recorded in
[`ALPHA-READINESS.md`](ALPHA-READINESS.md). Run the complete Sorna readiness
check with:

```sh
make sorna-alpha-check
```

The cross-language artifact boundary includes the published
[`ingen.run/v1`](spec/ingen.run-v1.schema.json) execution record and
[`sorna.evidence/v1`](spec/sorna.evidence-v1.schema.json) evidence manifest.
The run records what the frozen oracle observed; the manifest binds that run to
policies and checksummed files.

Mutation campaigns publish their aggregate through
[`ingen.mutation-campaign-result/v1`](spec/ingen.mutation-campaign-result-v1.schema.json).
The result is loaded canonically and keeps each mutation's evidence hashes and
diagnosis attached to the aggregate outcome.

Malcolm can hand its JSON IR slice to Sorna through the small `sorna-malcolm`
adapter. The adapter intentionally accepts only meaning it can lower without
loss: typed request bodies with recursive objects and arrays, stateful setup requests with positive
and negative status/body assertions, top-level response field
equality/presence, captures of top-level response fields, and explicit
`X-InGen-Event` event signals. The repeatable repository example is:

~~~sh
make malcolm-sorna-contract
~~~

This compiles malcolm/examples/healthz.malcolm, translates the resulting
malcolm.ir/v1 artifact into a draft ingen.contract/v1 document, and asks
Sorna's own contract validator to accept the result. It does not execute the
subject yet.

The request-body and stateful lowering proof is:

~~~sh
make malcolm-sorna-flow-contract
~~~

It compiles malcolm/examples/document_flow.malcolm and validates the generated
contract, including executable `given.body`, `given.setup`, `given.state`, and
capture data.

The flow also includes `must emit "document.accepted"`. The adapter lowers
this to `expect.events.required` and `expect.events.ordered`, and the HTTP/JSON
runner observes event names from the subject's `X-InGen-Event` response header.
The ordered form checks relative order within one response; both are public
subject signals, separate from Sorna lifecycle and host-access telemetry.

The full stateful and event behavioral proof is:

~~~sh
make malcolm-sorna-flow-run
~~~

This seals the generated contract, freezes its oracle under a dedicated
oracle policy, runs the managed document subject under a separate subject
policy, and verifies the resulting evidence bundle under
.artifacts/malcolm-flow-run.

The full first behavioral proof is available with:

~~~sh
make malcolm-sorna-run
~~~

That workflow seals the translated contract, freezes an oracle under the
separate Malcolm oracle policy, builds the clean document subject, and runs the
frozen oracle under a separate subject policy, then verifies the resulting
evidence bundle. Its generated run bundle is written under
.artifacts/malcolm-healthz-run.

Package areas:

- `internal/contract`: validation, canonicalization, sealing, and lineage;
- `internal/campaign`: deterministic mutation campaign planning;
- `internal/oracle`: deterministic case generation and freezing;
- `internal/runner`: HTTP/JSON public-boundary execution;
- `internal/mutation`: mutation providers, campaigns, and classification;
- `providers/golang`: source-copy and build mechanics for Go mutation variants;
- `internal/evidence`: manifests, lifecycle JSONL, checksums, and verification;
- `internal/policy`: capability policy validation, sealing, and references;
- `internal/sandbox`: process, filesystem, network, and resource policy;
- `cmd/sorna`: the headless CLI.

Provider preparation can be exposed before mutation execution with
`sorna mutation provider preparation <summary> --provider <manifest>
--format ci-result`. The resulting `mutation-preparation` envelope binds the
summary to the exact provider and plan inputs for a coordinator such as
Nublar.

The oracle access boundary is a real security boundary. The rest of the
directory layout is a maintainable starting point, not a demand for separate
services.

`sorna run` can either observe an already-running `--base-url` or manage a
subject process with `--subject-command` and repeated `--subject-arg` flags.
Managed runs wait for a declared successful HTTP readiness path, record the
process lifecycle, and tear down the process group after verification. Passing
`--subject-policy` additionally launches the managed process under the host
enforcement backend and records its policy and access stream separately from
the oracle bundle. The run remains assurance level 0 until the access boundary
has a stronger attestation.

The verified run path uses `sorna run --oracle <path>` and consumes the
canonical frozen artifact directly. The older `--contract <path>` form remains
as a compatibility path for callers that have not migrated yet.

Frozen oracle loading is intentionally strict: an oracle must contain at least
one case, and case IDs and rule IDs must each be unique. This prevents an empty
or ambiguous artifact from producing misleading execution summaries.

After a run, `sorna evidence verify <directory>` checks the bundle's recorded
SHA-256 values and reports the first mismatch.

`sorna evidence replay --oracle <path> --base-url <url> <directory>` first
verifies a stored bundle, then re-executes its frozen oracle against an
explicitly supplied equivalent HTTP subject. Replay never reopens the
contract, launches a subject, or overwrites the evidence bundle. A changed
contract-visible outcome is `drifted`; a changed request intent is also
`drifted`, even when the subject returns the same response. Request fingerprints
compare the ordered setup and target method/path/query/body while ignoring
host and port. A changed observation with the same request and outcome is
reported separately. An unreachable or incomplete subject is an execution
error rather than behavioral drift.

Replay can also emit the shared CI envelope:

```sh
sorna evidence replay --format ci-result \
  --oracle <path> --base-url <url> --output replay-ci-result.json <directory>
```

The envelope uses `kind: behavioral-replay`; it maps a matched replay to
`passed`, outcome drift to `failed`, and an unevaluable replay to `error`.

The document-pipeline replay regressions can be collected into one CI result:

```sh
make sorna-replay-matrix-ci-result
```

The Make target reads the reviewable manifest at
`examples/document-pipeline-lab/replay/matrix.yaml`. Other CI systems can use
the same interface directly:

```sh
sorna evidence replay matrix --manifest <path> [--source-root <dir>] [--output <path>]
```

The resulting `kind: behavioral-replay-matrix` artifact binds each member
envelope by path and SHA-256, checks its nested replay report, and passes only
when every expected `passed`, `failed`, or `error` / `matched`, `drifted`, or
`inconclusive` classification is present.

Verify the saved aggregate and its member inputs independently with:

```sh
sorna evidence replay matrix verify [--source-root <dir>] <ci-result>
```

To regenerate the complete six-case regression matrix without using local
artifacts, use:

```sh
make sorna-replay-matrix-ci-result-fresh
```

This copies source-only inputs into a temporary workspace, rebuilds every
member, creates the matrix, and independently verifies it there. The temporary
workspace path is printed when the target exits so its artifacts can be
inspected.

Saved replay reports can be checked independently before they are consumed:

```sh
sorna evidence replay verify replay-report.json
```

For the document-pipeline example, use the existing Makefile target after
starting an equivalent subject:

```sh
REPLAY_BASE_URL=http://127.0.0.1:8080 make sorna-replay
```

For a reproducible local green-path check, `sorna-replay-fresh` first creates
the clean baseline, then a separate fixture harness starts a new clean subject
and invokes replay against it:

```sh
make sorna-replay-fresh
```

The harness owns only the temporary subject lifecycle; the replay command still
does not launch or tear down subjects.

On macOS, the first host-enforcement backend is available through:

```sh
make sandbox-contract-read
```

This runs `/bin/cat` under a Seatbelt profile generated from the policy. It
allows the declared contract root and denies undeclared project paths. The
backend supports `network.mode: disabled` and directional TCP allowlists;
the macOS backend restricts process execution to the requested command and
declared tools. Policies also bind a logical `subject_id` to the sealed
contract ID, resolve the launch command to an absolute path, and compare its
live host-observed executable digest before execution. Oracle generation uses
a startup gate so the workload is observed before it reads the contract.
Oracle freezes record macOS unified-log access events in
`events/access.jsonl`, while managed subject runs use
`events/subject-access.jsonl`. The collector also records observed descendant
PIDs. Each bundle separately records coalesced live executable identities in
`events/executables.jsonl` or `events/subject-executables.jsonl`; a changed
path or digest creates a new observation. These streams remain parent-side
observations and do not automatically raise run assurance. The assurance
record also reports `observation_coverage` as `periodic-best-effort` or
`periodic-best-effort-with-gaps`, making the between-sample blind spot explicit.

To generate the first evidence-bearing oracle:

```sh
make sorna-oracle-freeze
make oracle-evidence-verify
```

To apply the default CI interpretation to a run bundle:

```sh
make sorna-gate
GATE_MIN_OBSERVATION_COVERAGE=periodic-best-effort make sorna-gate
go run ./sorna/cmd/sorna gate --format json .artifacts/document-pipeline-run
go run ./sorna/cmd/sorna gate --format ci-result .artifacts/document-pipeline-run \
  > .artifacts/document-pipeline-ci-result.json
```

The default gate blocks contract failures and surviving or inconclusive
mutations, while observation coverage remains report-only. The optional
minimum makes periodic sampling gaps blocking.

Contract-plane mutations use a separate inspection path. `mutation contract
inspect` applies each declared change to a copied contract, re-seals it,
regenerates the oracle under the original policy identity, and compares cases
by rule ID. It reports `visible`, `equivalent`, or `invalid`; it never launches
a subject and is not a contract-correctness score. Targets use
`rule:<rule-id>`. The document-pipeline example is in
`mutations/contract-catalogue.yaml`.

The follow-up `mutation contract ci-result <report>` command revalidates the
report against the original contract and oracle, then emits the shared
`ingen.ci-result/v1` envelope. Invalid mutation entries become a failed CI
result; unreadable, non-canonical, or mismatched inputs become an error result.

When the evidence bundle cannot be verified, `--format ci-result` emits an
`error` envelope with exit code `2` so a CI collector can retain the failure.

The parent Sorna process seals and verifies the contract and policy, while a
separate child process reads the contract and writes the frozen `oracle.json`
under the host-enforced profile.

The managed example run uses the separate subject policy automatically through
`make sorna-run`; use `make subject-policy-validate` to inspect that policy on
its own.

Mutation runs must identify a passing, unmutated evidence bundle with
`--baseline-evidence`. Sorna verifies that the baseline uses the same contract,
oracle, and policy identities before it launches the mutated subject. The
baseline path and run ID are retained in the resulting evidence bundle.

The mutation catalogue is a separate, reviewable declaration of the campaign
inputs. Validate or inspect the first example with:

```sh
make mutation-catalogue-validate
go run ./sorna/cmd/sorna mutation validate examples/document-pipeline-lab/mutations/catalogue.yaml --contract examples/document-pipeline-lab/contract/contract.yaml
go run ./sorna/cmd/sorna mutation list examples/document-pipeline-lab/mutations/catalogue.yaml
go run ./sorna/cmd/sorna mutation contract inspect examples/document-pipeline-lab/mutations/contract-catalogue.yaml \
  --contract examples/document-pipeline-lab/contract/contract.yaml \
  --oracle .artifacts/document-pipeline-oracle/oracle.json \
  --output .artifacts/document-pipeline-contract-mutations.json
go run ./sorna/cmd/sorna mutation contract ci-result \
  .artifacts/document-pipeline-contract-mutations.json \
  --contract examples/document-pipeline-lab/contract/contract.yaml \
  --oracle .artifacts/document-pipeline-oracle/oracle.json \
  --output .artifacts/document-pipeline-contract-mutations-ci.json
make mutation-plan
make mutation-provider-validate
make mutation-provider-inspect
make mutation-campaign-run
make mutation-campaign-verify
make mutation-campaign-ci-result
make mutation-go-provider-build
make mutation-go-provider-ci-result
make mutation-go-campaign-run
make mutation-go-campaign-verify
make mutation-go-campaign-ci-result
```

Catalogue validation checks stable IDs, canonical mutation planes, operators,
targets, reproducible changes, expected contract rules, and lifecycle status.
It does not apply mutations or launch a subject. `make mutation-plan` creates
the reviewable handoff for an execution provider and requires the passing clean
baseline.

The first execution provider is intentionally a prebuilt-variant manifest for
the document lab. `make mutation-campaign-run` consumes the plan, launches one
fresh managed subject for each provider entry, verifies each resulting evidence
bundle, attaches the exact plan/provider inputs to that bundle, binds the
bundle's manifest/checksum hashes into the entry, and writes
`campaign-result.json`. `make mutation-campaign-verify` rechecks each recorded
evidence bundle and compares its current hashes with the aggregate result. A
`killed` mutation is a successful campaign entry; the nested contract run may
still be red by design.

Provider manifests now declare the exact mutation plane, operator, and target
shapes they support. Sorna checks those capabilities against the plan before
launching any subject.

The external manifest contract is documented in
[`spec/ingen.mutation-provider-v1.schema.json`](spec/ingen.mutation-provider-v1.schema.json).
It describes the versioned provider envelope, capability tuples, prepared
entries, and optional source provenance. Runtime Go validation remains the
execution boundary; the schema gives other tools a language-neutral review
surface.

`mutation provider inspect` produces a versioned review report without
launching a subject. It shows the exact plan/provider hashes, plan binding
state, declared capabilities, and per-mutation entry/capability matches.
With `--format ci-result`, the same report becomes a shared CI envelope for
Nublar.

Plan binding is optional for the local fixture provider but can be required by
callers that want every prepared provider tied to the exact plan bytes:

```sh
sorna mutation provider inspect plan.json \
  --provider provider.yaml --require-plan-binding
```

The generated Go provider produces a matched binding. The same
`--require-plan-binding` flag is accepted by `sorna mutation run`, so execution
can enforce the policy independently of preflight.

`sorna mutation verify <campaign-result> --format ci-result` verifies every
recorded evidence bundle and emits a shared `ingen.ci-result/v1` envelope with
the complete campaign result as its report. A passed campaign means every
planned mutation was killed; survivors or inconclusive entries produce a
failed envelope, while evidence or result integrity errors produce an error
envelope.

The first source-level Go provider is deliberately narrow. It copies the clean
Go module once per plan entry, applies the document lab's reviewed
`response.status.replace` or `response.field.remove` change with the Go AST,
builds a fresh binary, records source/edit/binary provenance, and then hands
the normal `ingen.mutation-provider/v1` manifest to Sorna:

```sh
make mutation-go-campaign-run
make mutation-go-campaign-verify
```

The provider retains copied variant sources for review and places runnable
binaries under the existing managed-subject read root. It does not modify the
working tree; Sorna verifies the recorded source and binary hashes before each
variant starts. It does not claim to support arbitrary Go operators yet.
