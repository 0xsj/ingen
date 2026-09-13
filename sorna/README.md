# Sorna

Sorna is InGen's standalone verification laboratory. It is the part that
turns a sealed behavioral contract into a frozen oracle, evaluates a system
through a public boundary, runs mutation campaigns, and produces evidence.

The current files in this directory are design specifications plus the first Go
implementation slice. The contract package and CLI can load, validate,
canonicalize, and seal a contract locally or in CI without Herdr.

Planned package areas:

- `internal/contract`: validation, canonicalization, sealing, and lineage;
- `internal/oracle`: generation, freezing, and evaluation;
- `internal/adapter`: HTTP/JSON and later public-boundary adapters;
- `internal/verification`: baseline and rule execution;
- `internal/mutation`: mutation providers, campaigns, and classification;
- `internal/evidence`: manifests, JSONL results, checksums, and replay;
- `internal/sandbox`: process, filesystem, network, and resource policy;
- `cmd/sorna`: the headless CLI.

The oracle access boundary is a real security boundary. The rest of the
directory layout is a maintainable starting point, not a demand for separate
services.
