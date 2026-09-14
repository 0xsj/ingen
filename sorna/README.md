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
- `internal/oracle`: deterministic case generation and freezing;
- `internal/adapter`: HTTP/JSON and later public-boundary adapters;
- `internal/verification`: baseline and rule execution;
- `internal/mutation`: mutation providers, campaigns, and classification;
- `internal/evidence`: manifests, lifecycle JSONL, checksums, and verification;
- `internal/policy`: capability policy validation, sealing, and references;
- `internal/sandbox`: process, filesystem, network, and resource policy;
- `cmd/sorna`: the headless CLI.

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
