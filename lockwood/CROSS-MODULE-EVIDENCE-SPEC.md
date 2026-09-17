# Lockwood cross-module evidence handoff

Status: workflow contract with an implemented offline fixture path. It defines
how existing module artifacts may be preserved and related without introducing
a combined cross-module envelope.

The deterministic fixture path is implemented by
[`cross_module_evidence_test.go`](cross_module_evidence_test.go) and the
producer-shaped fixtures under [`testdata/`](testdata/). It uses the existing
Sorna and CI-result adapters plus generic custody intake for Hammond and
Nublar artifacts; it does not call live module or delivery services.

## Ownership map

| Module | Produces or owns | Lockwood responsibility |
| --- | --- | --- |
| Sorna | Evidence bundles, run manifests, behavioral results, assurance claims | Preserve exact bundle bytes and producer-owned metadata; do not recalculate Sorna semantics. |
| Hammond | Contract references, review decisions, governance lineage | Preserve referenced bytes or decision artifacts when supplied; do not approve or reinterpret them. |
| Lockwood | Local artifact bytes, custody records, lineage, handling history, detached evidence | Verify local byte integrity and preserve append-only custody history. |
| Nublar | CI collection runs, coordinator decisions, delivery receipts | Preserve exact producer artifacts when requested; do not replace Nublar run or delivery ownership. |

The handoff is intentionally made of existing versioned artifacts and exact
digests. A module's status field, decision, approval, or assurance level stays
owned by the module that produced it.

## Canonical evidence path

The smallest end-to-end path is:

```text
Hammond contract reference/decision
              │ exact artifact digest
              ▼
Sorna evidence bundle ──► Sorna or CI result envelope
              │                       │ exact bytes and digest
              └──────────────► Lockwood custody and lineage
                                      │ caller-supplied artifact reference
                                      ▼
                              Nublar run/decision
                                      │ separate delivery attempt
                                      ▼
                              Nublar delivery receipt
```

Lockwood may preserve each node and the explicit digest relationships between
nodes. It must not turn the path into a single semantic verdict or claim that
custody proves governance approval, behavioral correctness, CI success, or
delivery acceptance.

## Handoff rules

1. Every preserved artifact is identified by its exact local content digest.
   Paths, run IDs, workflow IDs, contract IDs, and external correlation IDs
   remain descriptive metadata.
2. Producer bytes are stored unchanged unless a producer-owned adapter defines
   a deterministic representation. The Sorna adapter's deterministic tar is
   such a representation; the CI-result adapter preserves the original JSON
   bytes.
3. A custody record may carry producer and source metadata, but it must not
   copy producer-specific semantic fields into a Lockwood-owned verdict.
4. Cross-module relationships use explicit local artifact digests or custody
   lineage. A URI or ID without a digest is not an integrity relationship.
5. A missing, damaged, mismatched, or noncanonical referenced artifact fails
   the relevant read-only verification path. A failed producer result may be
   valid evidence and must remain distinguishable from a malformed handoff.
6. Each module's receipt remains a separate artifact. Nublar's delivery
   failure does not mutate its stored run decision or the Lockwood custody
   record that preserves the producer artifact.

## Existing adapter mappings

### Sorna evidence

The Lockwood Sorna adapter validates the Sorna manifest and checks listed file
digests before creating one deterministic archive artifact. The resulting
custody record should retain:

- producer tool `sorna` and the manifest kind;
- the Sorna `run_id` as descriptive source metadata;
- the deterministic archive's local artifact digest;
- any caller-supplied parent artifact digests; and
- the original Sorna assurance semantics as bytes, not as a Lockwood verdict.

The archive digest is the identity of the bytes Lockwood stores. It is not a
replacement for the Sorna manifest's own referenced-artifact checks.

### CI result

The CI-result adapter validates `ingen.ci-result/v1`, preserves the original
JSON bytes, and records the producer tool and kind. A `passed`, `failed`, or
`error` result can all be accepted as intact evidence; malformed JSON,
schema-invalid content, or a digest mismatch is an intake failure.

Lockwood must not turn a failed producer result into a rejected custody record
or turn a valid result into a passed Lockwood decision. Nublar remains the owner
of collection status and coordinator exit codes.

### Hammond governance artifacts

Hammond's contract reference identifies governed contract bytes by project,
contract ID, version, and artifact digest. When a caller supplies those bytes
to Lockwood, it may store them as a normal artifact and preserve the complete
Hammond reference or decision as a separate artifact. The custody relationship
may point to the contract artifact digest, but Lockwood must not infer that an
approved Hammond record makes a Sorna run correct or authorized.

Hammond's local authority, membership, policy, and review semantics remain
Hammond-owned. An external authorization handoff may reference their exact
digests, but Lockwood does not load a Hammond directory or recreate its review
evaluation.

### Nublar run and delivery receipt

Nublar consumes complete producer result files and records its own immutable
run ID, decision, and optional external correlation tuple. When a caller asks
Lockwood to preserve Nublar output, each run or delivery receipt is stored as
its own artifact with its own digest and producer metadata.

Nublar's delivery receipt is evidence of one delivery attempt. It is not a
mutation to the stored Nublar decision and not proof that a destination
accepted or acted on the producer evidence beyond the receipt's stated
observation.

## End-to-end fixture matrix

The first cross-module fixture path should use existing schemas and adapters:

| Fixture | Expected Lockwood behavior |
| --- | --- |
| valid Hammond contract bytes plus exact reference | store bytes and reference separately; preserve digest binding |
| deterministic Sorna evidence bundle | import one stable archive digest and retain Sorna run metadata |
| valid failed `ingen.ci-result/v1` | accept intact bytes while preserving `failed` producer status |
| malformed CI result | reject before custody publication |
| custody record with explicit Sorna/CI parent digests | preserve lineage and verify reachable local artifacts |
| Nublar run with a failed decision | preserve the run/producer evidence without changing its status |
| Nublar failed delivery receipt | preserve a separate receipt; do not mutate the run or producer artifact |
| changed bytes under an existing expected digest | fail closed and preserve the original published artifact |
| missing or damaged lineage parent | retain custody if already accepted, but fail record-level verification |

The fixture should assert exact digest relationships and byte preservation,
not only successful command exit codes. It should be runnable without live
Hammond, Nublar, Sorna, or delivery services.

## Relation vocabulary

Until a concrete consumer requires a shared relation schema, use existing
Lockwood lineage relations such as `references` and producer-specific
metadata. Do not introduce a universal relation that implies semantic
ownership of another module's decision. If a first-class cross-module catalog
relation becomes necessary, it must define source/target artifact digests,
relation kind, producer namespace, and verification behavior as a versioned
closed contract.

## Explicit non-goals

This contract does not define:

- a universal workflow or run identifier;
- a combined Sorna/Hammond/Lockwood/Nublar envelope;
- automatic Hammond approval from Sorna evidence;
- automatic Nublar delivery from Lockwood custody;
- reinterpretation of producer status, assurance, or review decisions;
- live cross-module service calls or remote artifact retrieval; or
- a hosted cross-module catalog.

## Open decisions

- Which concrete consumer needs a first-class cross-module relation beyond
  digest-bearing custody lineage?
- Should Hammond decision artifacts be preserved by a dedicated adapter or by
  generic `put` plus caller-supplied lineage?
- Should Nublar run and delivery artifacts receive dedicated Lockwood media
  types, or remain generic producer-owned JSON artifacts?
- What retention and authorization policy applies when one artifact is shared
  by multiple module workflows?
