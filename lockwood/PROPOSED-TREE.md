# Proposed Lockwood tree

Status: proposal. This is a planning artifact, not yet an implementation
commitment.

The first Lockwood slice should stay narrow and center on three
responsibilities: artifact identity, custody metadata, and storage.

The original proposal remains preserved below. The implemented slice has
grown to include local recovery, reconciliation, versioned source metadata,
and producer adapters; the inventory below keeps the planning tree from being
mistaken for an exact filesystem listing.

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
│   │   ├── handling_event.go
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
│   ├── attestation/              # Detached signing, encoding, and publication
│   └── adapters/
│       ├── sorna/                # Import Sorna evidence bundles
│       └── ciresult/             # Import ingen.ci-result/v1
├── spec/
│   ├── lockwood.artifact-v1.schema.json
│   ├── lockwood.custody-v1.schema.json
│   ├── lockwood.custody-v2.schema.json
│   ├── lockwood.attestation-v1.schema.json
│   ├── lockwood.handling-event-attestation-v1.schema.json
│   ├── lockwood.handling-event-policy-v1.schema.json
│   └── lockwood.handling-event-v1.schema.json
├── examples/
│   └── custody-record.json
└── testdata/
```

## Current implemented slice

```text
lockwood/
├── cmd/lockwood/                 # put, imports, signed handling events, get, inspect, lineage-status, handling events/status/guard, redaction registration/promotion, verify, find, recover, reconcile
├── internal/
│   ├── artifact/                 # SHA-256 references
│   ├── store/                    # filesystem and in-memory blobs, inventories, reference manifests
│   ├── custody/                  # filesystem and in-memory records, handling events, lineage, recovery, verification
│   ├── catalog/                  # deterministic metadata queries
│   ├── integrity/                # shared streaming hash and size verification
│   ├── attestation/              # detached record/event sign/verify, encoding, and publication
│   └── adapters/
│       ├── ciresult/             # validated ingen.ci-result/v1 intake
│       └── sorna/                # deterministic verified Sorna bundle intake
├── spec/                         # artifact-v1, custody-v1/v2, attestation-v1/trust-v1, handling-event-v1, event-attestation-v1, event-policy-v1
└── testdata/                     # valid and invalid contract fixtures
```

The proposed remote/object-store area and broader action-authorization implementation
remain future work rather than missing implementation files. The detached
attestation and handling-event-signature contracts, local Ed25519 helper, and
explicit canonical trust registry now exist. The registry makes only key-status
and validity decisions;
it does not provide human identity or access control. The in-memory stores,
shared integrity package, and example record are included as lightweight
development surfaces.

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
├── events/
│   └── <custody-id>/
│       └── <event-id>.json
├── references/
│   └── sha256/
│       └── ab/
│           └── cd/
│               └── <reference-metadata-digest>.json
└── catalog/
    └── index.json
```

Blobs are addressed by content digest. Custody records describe where an
artifact came from and how it relates to other artifacts.

Reference manifests preserve descriptive media-type and logical-name variants
without making that metadata part of blob identity.

## Deferred areas

The initial slice does not need to include remote/object-storage backends,
human signer identity and access authorization, retention deletion and legal holds,
redaction execution, authentication, or a web UI. Reconciliation may classify
age-based orphan cleanup candidates, but deletion and race-safe cleanup
coordination remain deferred. Recognized detached artifacts are protected by
their verified reference metadata rather than being treated as custody
records. Handling events are append-only descriptive records; authenticated
actors, policy enforcement, and payload-changing redaction remain deferred.
The first redaction data boundary may register a caller-produced derivative
after verifying both source and result artifacts and the legal-hold guard;
that registration preserves the original and custody record. Built-in local
stores serialize handling-event mutations per custody ID; flock-capable
filesystem roots coordinate separate local processes, while distributed
coordination remains deferred. Promotion can then anchor the registered result
in a separate custody record with explicit `derived-from` lineage.
