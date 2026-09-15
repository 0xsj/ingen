# Proposed Lockwood tree

Status: proposal. This is a planning artifact, not yet an implementation
commitment.

The first Lockwood slice should stay narrow and center on three
responsibilities: artifact identity, custody metadata, and storage.

```text
lockwood/
├── README.md
├── CUSTODY-SPEC.md              # Lockwood boundary and invariants
├── cmd/
│   └── lockwood/
│       └── main.go
├── internal/
│   ├── artifact/                 # Digests, references, immutable bytes
│   │   ├── artifact.go
│   │   ├── digest.go
│   │   └── reference.go
│   ├── custody/                  # Custody records and lineage
│   │   ├── record.go
│   │   ├── lineage.go
│   │   ├── filesystem.go
│   │   └── ingest.go
│   ├── store/                    # Storage abstraction and local backend
│   │   ├── store.go
│   │   ├── filesystem.go
│   │   └── memory.go
│   ├── catalog/                  # Find artifacts by run, tool, type, etc.
│   │   ├── catalog.go
│   │   └── filesystem.go
│   ├── integrity/                # Hash and manifest verification
│   │   ├── verify.go
│   │   └── verify_test.go
│   └── adapters/
│       ├── sorna/                # Import Sorna evidence bundles
│       └── ciresult/             # Import ingen.ci-result/v1
├── spec/
│   ├── lockwood.artifact-v1.schema.json
│   ├── lockwood.custody-v1.schema.json
│   └── lockwood.custody-v2.schema.json
├── examples/
│   └── custody-record.json
└── testdata/
```

## Proposed custody root

```text
lockwood-data/
├── blobs/
│   └── sha256/
│       └── ab/
│           └── cd/
│               └── <full-digest>
├── records/
│   └── <custody-id>.json
└── catalog/
    └── index.json
```

Blobs are addressed by content digest. Custody records describe where an
artifact came from and how it relates to other artifacts.

## Deferred areas

The initial slice does not need to include remote/object-storage backends,
signatures or external attestation, retention deletion and legal holds,
redaction workflows, authentication, or a web UI.
