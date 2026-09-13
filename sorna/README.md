# Sorna

Sorna is InGen's standalone verification laboratory. It is the part that
turns a sealed behavioral contract into a frozen oracle, evaluates a system
through a public boundary, runs mutation campaigns, and produces evidence.

The current files in this directory are design specifications plus the first Go
implementation slice. The contract package and CLI can load, validate,
canonicalize, and seal a contract locally or in CI without Herdr. Sorna can
also write and checksum a small evidence bundle for a run.

Planned package areas:

- `internal/contract`: validation, canonicalization, sealing, and lineage;
- `internal/oracle`: generation, freezing, and evaluation;
- `internal/adapter`: HTTP/JSON and later public-boundary adapters;
- `internal/verification`: baseline and rule execution;
- `internal/mutation`: mutation providers, campaigns, and classification;
- `internal/evidence`: manifests, lifecycle JSONL, checksums, and verification;
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

After a run, `sorna evidence verify <directory>` checks the bundle's recorded
SHA-256 values and reports the first mismatch.
