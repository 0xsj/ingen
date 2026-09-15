# Sorna

Sorna is InGen's standalone verification laboratory. It is the part that
turns a sealed behavioral contract into a frozen oracle, evaluates a system
through a public boundary, runs mutation campaigns, and produces evidence.

The current files in this directory are design specifications plus the first Go
implementation slices. The contract package and CLI can load, validate,
canonicalize, and seal a contract locally or in CI without Herdr. Sorna can
also freeze a deterministic oracle in a sandbox and checksum its evidence.

Package areas:

- `internal/contract`: validation, canonicalization, sealing, and lineage;
- `internal/campaign`: deterministic mutation campaign planning;
- `internal/oracle`: deterministic case generation and freezing;
- `internal/adapter`: HTTP/JSON and later public-boundary adapters;
- `internal/verification`: baseline and rule execution;
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

After a run, `sorna evidence verify <directory>` checks the bundle's recorded
SHA-256 values and reports the first mismatch.

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
