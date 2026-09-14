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
process lifecycle, and tear down the process group after verification. This is
stronger lifecycle evidence than a manually started process, but it is still
assurance level 0 until capability isolation is enforced.

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
backend currently supports `network.mode: disabled`; process/tool declarations
and kernel access-event capture are not yet wired into the evidence bundle.

To generate the first evidence-bearing oracle:

```sh
make sorna-oracle-freeze
make oracle-evidence-verify
```

The parent Sorna process seals and verifies the contract and policy, while a
separate child process reads the contract and writes the frozen `oracle.json`
under the host-enforced profile.
