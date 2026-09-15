# Nublar architecture map

This is the proposed shape for Nublar as it grows beyond the current
aggregation prototype. The near-term tree is intentionally small; the later
directories are expansion points, not an implementation checklist.

The product ownership and first-slice scope are defined in
[`PRODUCT-BOUNDARY.md`](PRODUCT-BOUNDARY.md).
The producer handoff and execution ownership are defined in
[`EXECUTION-BOUNDARY.md`](EXECUTION-BOUNDARY.md).
The CI-facing output and exit-code contract are defined in
[`OUTPUT-BOUNDARY.md`](OUTPUT-BOUNDARY.md).

## Near-term tree

```text
nublar/
├── README.md
├── ARCHITECTURE.md
├── status.md
├── cmd/
│   └── nublar/
│       └── main.go
├── spec/
│   ├── decision-v1.schema.json
│   ├── workflow-v1.schema.json
│   └── run-v1.schema.json
├── internal/
│   ├── workflow/
│   │   ├── document.go
│   │   ├── loader.go
│   │   └── validate.go
│   ├── run/
│   │   ├── model.go
│   │   ├── collect.go
│   │   └── decision.go
│   ├── artifact/
│   │   ├── reader.go
│   │   ├── hasher.go
│   │   └── provenance.go
│   ├── storage/
│   │   ├── store.go
│   │   └── filesystem/
│   ├── delivery/
│   │   ├── projection.go
│   │   ├── publisher.go
│   │   └── webhook/
│   └── output/
│       └── output.go
├── workflows/
│   └── document-pipeline.yaml
└── testdata/
    ├── workflows/
    └── ci-results/
```

The first product-shaped flow is:

```text
workflow declaration
    → collect producer envelopes
    → validate and hash artifacts
    → create run record
    → compute aggregate decision
    → persist result
```

## Eventual expansion points

```text
nublar/
├── cmd/
│   ├── nublar/
│   └── nublar-server/
├── api/
│   └── http/
├── internal/
│   ├── workflow/
│   ├── run/
│   ├── artifact/
│   ├── storage/
│   │   ├── filesystem/
│   │   └── postgres/
│   ├── execution/
│   │   ├── producer.go
│   │   └── sorna.go
│   ├── delivery/
│   │   ├── pullrequest/
│   │   └── webhook/
│   ├── retention/
│   └── scheduler/
├── spec/
├── migrations/
├── workflows/
└── testdata/
```

`execution`, `scheduler`, `delivery`, and database-backed storage should be
added only when a concrete workflow requires them. Sorna remains the owner of
verification and mutation semantics; Nublar owns collection, run lifecycle,
workflow policy, and delivery decisions.

The current `internal/aggregate` package is the prototype predecessor of the
future `internal/run` boundary, while `internal/workflow` already represents
the intended workflow boundary.

The mixed-producer fixture under `testdata/` demonstrates that the run path
can compose Sorna and Paddock envelopes without importing either producer's
implementation.
