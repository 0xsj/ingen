# Hammond

Hammond is a future placeholder for cross-project contract intent and
governance: contract registries, approvals, review history, amendments, and
lineage.

It is not a separate service yet. Sentinel should continue to provide the
local contract workspace, and Hammond should only grow when projects need
shared governance beyond a single Herdr workspace.

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
│   │   ├── validate.go       # structural and governance invariants
│   │   ├── lifecycle.go      # draft/review/approved/superseded transitions
│   │   ├── lineage.go        # parent, successor, and amendment relationships
│   │   └── governance_test.go
│   │
│   └── store/
│       ├── store.go          # persistence interface
│       ├── filesystem.go     # initial local implementation
│       └── filesystem_test.go
│
├── spec/
│   └── ingen.hammond-governance-v1.schema.json
│
├── examples/
│   └── document-pipeline/
│       ├── contract-reference.json
│       ├── review-record.json
│       └── amendment-record.json
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
