# Nublar architecture map

This is the implemented near-term shape for Nublar as it grows beyond the
initial aggregation prototype. The later directories are expansion points,
not an implementation checklist.

The product ownership and first-slice scope are defined in
[`PRODUCT-BOUNDARY.md`](PRODUCT-BOUNDARY.md).
The producer handoff and execution ownership are defined in
[`EXECUTION-BOUNDARY.md`](EXECUTION-BOUNDARY.md).
The CI-facing output and exit-code contract are defined in
[`OUTPUT-BOUNDARY.md`](OUTPUT-BOUNDARY.md).
The provider-neutral delivery and optional receipt contracts are defined in
[`DELIVERY-BOUNDARY.md`](DELIVERY-BOUNDARY.md).
The optional local receipt persistence boundary is defined in
[`RECEIPT-STORAGE.md`](RECEIPT-STORAGE.md).

## Near-term tree

```text
nublar/
├── README.md
├── ARCHITECTURE.md
├── CONTRACT-CHECKPOINT.md
├── CONSUMER-REQUEST-TEMPLATE.md
├── FREEZE-RECORD.md
├── examples/
│   └── consumer/
│       ├── README.md
│       ├── check.sh
│       └── nublar-ci-gate.sh
├── cmd/
│   └── nublar/
│       ├── main.go
│       └── main_test.go
├── spec/
│   ├── decision-v1.schema.json
│   ├── receipt-v1.schema.json
│   ├── workflow-v1.schema.json
│   └── run-v1.schema.json
├── internal/
│   ├── aggregate/
│   │   └── aggregate.go
│   ├── artifact/
│   │   └── artifact.go
│   ├── workflow/
│   │   └── workflow.go
│   ├── run/
│   │   ├── run.go
│   │   └── collect.go
│   ├── storage/
│   │   ├── store.go
│   │   └── filesystem/
│   │       ├── filesystem.go
│   │       └── receipts.go
│   ├── delivery/
│   │   ├── projection.go
│   │   ├── publisher.go
│   │   └── webhook/
│   │       └── webhook.go
│   └── output/
│       └── output.go
├── workflows/
│   ├── document-pipeline.yaml
│   ├── sentinel-webhook.yaml
│   └── webhook-validation.yaml
└── testdata/
    ├── workflows/
    │   ├── malformed-producer.yaml
    │   ├── missing-producer.yaml
    │   ├── mixed-producers.yaml
    │   └── passed-producer.yaml
    └── ci-results/
        └── malformed.json
```

The compatibility `aggregate` package and the implemented run path share the
same workflow and artifact boundaries. Tests live beside each package, with a
mixed-producer integration fixture under `testdata/`.

The first product-shaped flow is:

```text
workflow declaration
    → collect producer envelopes
    → validate and hash artifacts
    → create run record
    → compute aggregate decision
    → persist result
    → optionally deliver projection and export receipt
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

The current `internal/aggregate` package is the compatibility predecessor of
the implemented `internal/run` boundary, while `internal/workflow` represents
the workflow boundary.

The mixed-producer fixture under `testdata/` demonstrates that the run path
can compose Sorna and Paddock envelopes without importing either producer's
implementation.
