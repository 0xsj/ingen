# Hammond

Hammond is a future placeholder for cross-project contract intent and
governance: contract registries, approvals, review history, amendments, and
lineage.

It is not a separate service yet. Sentinel should continue to provide the
local contract workspace, and Hammond should only grow when projects need
shared governance beyond a single Herdr workspace.

The working v1 governance model is documented in
[GOVERNANCE-SPEC.md](GOVERNANCE-SPEC.md).

## Proposed v1 tree

The first Hammond slice should remain a small, local governance module around
existing Sorna contract artifacts:

```text
hammond/
├── README.md
├── GOVERNANCE-SPEC.md
├── status.md
├── Makefile
│
├── cmd/
│   └── hammond/
│       └── main.go
│
├── internal/
│   ├── governance/
│   │   ├── types.go          # contract refs, reviews, approvals, amendments
│   │   ├── policy.go         # explicit approval policy and quorum counting
│   │   ├── validate.go       # structural and governance invariants
│   │   ├── lifecycle.go      # draft/review/approved/superseded transitions
│   │   ├── lineage.go        # parent, successor, and amendment relationships
│   │   ├── amendment.go      # policy-aware amendment and supersession events
│   │   ├── codec.go          # strict record/event/policy decoding and hashing
│   │   └── governance_test.go
│   │
│   └── store/
│       ├── store.go          # persistence interface
│       ├── filesystem.go     # initial local implementation
│       └── filesystem_test.go
│
├── spec/
│   ├── ingen.hammond-governance-v1.schema.json
│   └── ingen.hammond-review-policy-v1.schema.json
│
├── examples/
│   ├── review-policy-v1.json
│   └── document-pipeline/
│       ├── record-v2.json
│       ├── event-review-opened.json
│       └── event-approved.json
│
├── contracts/
│   └── .gitkeep
│
└── governance/
    └── .gitkeep
```

`spec/` will define Hammond's governance artifact, while Sorna remains the
authority for contract validation, sealing, oracle generation, verification,
and evidence. The `internal/governance/` package will own approvals,
amendments, lifecycle, and lineage; `internal/store/` will hide the initial
file-backed persistence choice.

The first milestone is intentionally narrow: register one contract, approve
its exact artifact hash, create a linked amendment, and print the resulting
lineage. An HTTP API, UI, database migrations, and external integrations are
deferred until this model is stable.

## Local CLI slice

The current CLI can register a record, append validated events, create a linked
amendment, supersede a predecessor after successor approval, inspect stored
records, list the registry, and validate lineage:

```sh
STORE=/tmp/hammond-records

go run ./hammond/cmd/hammond register \
  --store "$STORE" \
  --record hammond/examples/document-pipeline/record-v2.json

go run ./hammond/cmd/hammond append-event \
  --store "$STORE" \
  --record hammond/examples/document-pipeline/record-v2.json \
  --event hammond/examples/document-pipeline/event-review-opened.json

go run ./hammond/cmd/hammond append-event \
  --store "$STORE" \
  --record hammond/examples/document-pipeline/record-v2.json \
  --event hammond/examples/document-pipeline/event-approved.json

go run ./hammond/cmd/hammond lineage --store "$STORE"
```

The store and CLI use the explicit v1 default policy: one approval from one
distinct actor in the active review cycle. Library callers can also require
named approval roles or a higher threshold; role authorization outside the
verified policy artifact and organization-level policy rules are not modeled
yet.
