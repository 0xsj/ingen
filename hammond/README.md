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
│   │   ├── authority_signature.go # trusted Ed25519 authority signatures
│   │   ├── authority_root.go # caller-delivered root-key snapshots and rotation
│   │   ├── authority_trust.go # active/revoked key snapshots and trust roots
│   │   ├── authority_membership.go # effective-dated membership adapter
│   │   ├── membership.go      # authenticated provider membership snapshots
│   │   ├── membership_provider.go # bounded HTTP transport and auth hook
│   │   ├── codec.go          # strict decoding and contract/policy hashing
│   │   └── governance_test.go
│   │
│   └── store/
│       ├── store.go          # persistence and conditional revision interface
│       ├── artifacts.go      # local content-addressed artifact blobs
│       ├── filesystem.go     # atomic local writes and process-shared locking
│       ├── artifacts_test.go
│       └── filesystem_test.go
│
├── spec/
│   ├── ingen.hammond-authority-v1.schema.json
│   ├── ingen.hammond-authority-root-v1.schema.json
│   ├── ingen.hammond-authority-trust-v1.schema.json
│   ├── ingen.hammond-governance-v1.schema.json
│   ├── ingen.hammond-membership-v1.schema.json
│   └── ingen.hammond-review-policy-v1.schema.json
│
├── examples/
│   ├── review-authority-v1.json
│   ├── review-policy-v1.json
│   └── document-pipeline/
│       ├── contract-v2.canonical.json
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
records, list the registry, validate lineage, and expose revision tokens for
conditional mutations:

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

Use `hammond revision --store "$STORE" --record <path>` to obtain the current
record revision. Pass it as `--if-revision <revision>` to `append-event`,
`amend`, or `supersede`; a stale token fails with a conflict and should be
replaced after rereading the record.

The store and CLI use the explicit v1 default policy: one approval from one
distinct actor in the active review cycle. Library callers can also require
named approval roles, a higher overall threshold, or distinct-actor
thresholds for specific roles. A policy may reference a separate, digest-bound
local authority snapshot for actor-to-role grants;
organization-level identity and policy rules are not modeled yet.

At runtime, callers can supply an `AuthorityVerifier` implementation. Its
check receives the approval event's timestamp, so providers can apply
effective-date and revocation rules. The bundled local authority snapshot is
one adapter; a hosted registry can later inject verified organization
membership without changing record or lifecycle validation. Callers that need
issuer attribution can additionally load signed
authority artifacts with a trusted `AuthoritySignatureVerifier` key set.
`AuthorityTrustStore` provides a versioned active/revoked key-set adapter for
rotation; its successor helper preserves the trust ID and rejects equal or
older versions. `AuthorityRootStore` provides the caller-delivered root-key
layer: a
replacement root snapshot can be signed by an active predecessor, while
revoked roots are excluded from the next verifier. The initial bootstrap and
approval of root keys remain outside Hammond. The initial bootstrap pins the
expected root ID, version, and public keys. A trust snapshot can be root-signed
and verified before its active keys are used.
`TimeScopedAuthority` is available for normalized membership data with
effective and expiry timestamps. `MembershipSnapshot` adds a digest-bound,
optionally signed provider-response envelope around those grants; callers can
use `VerifierAt` to enforce snapshot freshness before evaluation.
`HTTPMembershipProvider` is a narrow transport adapter for fetching one such
snapshot. It requires a caller-owned signature verifier, bounds the response,
and exposes authentication, endpoint-resolution, and endpoint-policy hooks;
provider credentials, TLS policy, discovery policy, and response mapping
remain outside Hammond. `FetchNormalized` hands provider-native bytes to a
caller-owned normalizer, which may reject an incomplete view before Hammond
verifies the resulting envelope. `BearerTokenAuthenticator` is available as a
small token-source adapter; the source remains responsible for token storage,
refresh, and rotation. Decision events may carry the membership reference used
by the caller's injected verifier for audit provenance. Callers can use
`VerifierAtWithProvenance` or `FetchVerifierAtWithProvenance` to make Hammond
enforce that binding. `MembershipEndpointAllowlist` is available as a strict
exact host/port endpoint policy; it does not replace caller-owned TLS or
network enforcement. When configured, the HTTPS requirement and endpoint
policy are also applied to redirect targets before the client follows them.
