# Replay must preserve the frozen boundary

## Finding

Stored Sorna evidence needs two different follow-up operations:

1. verify the evidence bundle's checksums and structural semantics; and
2. re-execute the frozen oracle against an explicitly supplied equivalent
   subject.

Those operations must not be conflated. Verification is read-only and does
not contact a subject. Replay is a new behavioral experiment; it contacts the
URL supplied by the caller, but it does not launch a process, reopen contract
source, or rewrite the original evidence directory.

## Implementation

`evidence.Replay` verifies the bundle, loads its recorded run identity, hashes
the caller-supplied canonical oracle, and requires the oracle and contract
references to match the recorded run. It then delegates execution to
`runner.ExecuteOracle`, which accepts the frozen artifact directly.

The command is:

```sh
sorna evidence replay --oracle <frozen-oracle.json> --base-url <url> <evidence-directory>
```

The report uses `sorna.replay/v1`. It compares rule IDs, statuses, setup
outcome shapes, assertion outcome shapes, and the contract verdict. It also
fingerprints each rule's ordered public request intent: setup and target
method/path/query/body are included, while host and port are ignored. This
keeps equivalent local subjects comparable without allowing a changed request
shape to hide behind the same response. A contract-visible or request-intent
difference is `drifted`. Observation hashes are compared for diagnosis but are
tracked separately: an observation can change while the contract-visible
outcome remains `matched`.

The first host-enabled fresh replay exposed a producer determinism bug: Go map
iteration changed the order of equivalent assertion records between the
baseline and replay. Sorna now sorts contract property keys when producing
assertions, and replay canonicalizes assertion path/status pairs defensively
when comparing existing evidence. The initial red was therefore a useful
false-positive finding, not subject behavior drift.

If the supplied subject cannot be evaluated, replay reports `error` (or
`inconclusive` when evaluation is incomplete) rather than calling that a
behavioral drift. An unavailable observation is not compared as a changed
observation.

The CI adapter preserves the complete replay report inside
`ingen.ci-result/v1`, binds the oracle plus the evidence manifest, checksum
file, and verified artifacts as inputs, and maps the producer states to the
shared envelope: `matched` → `passed`, `drifted` → `failed`, and
`error`/`inconclusive` → `error`.

## Why the URL is explicit

Replaying against the original recorded URL by default could send requests to
an unintended live system. Requiring `--base-url` makes the new experiment's
target an explicit reviewable input. Replay does not claim that the target is
equivalent; it reports what happened when the frozen oracle was run there.

## Limits

The first replay slice is an HTTP/JSON execution path. Requests may have side
effects on the supplied subject, so “read-only” applies to the stored
evidence bundle, not to the external subject. A future adapter can provide a
resettable fixture or an isolated replay environment when the protocol needs
stronger repeatability.

The document-pipeline lab now provides that local environment through a
separate `replay-fixture` harness. It creates no Sorna evidence itself: it
starts a fresh subject, waits for readiness, invokes the replay CLI, and
tears the subject down. Keeping that lifecycle outside Sorna prevents a
convenience target from changing the verifier's explicit URL-only boundary.

The negative fixture uses the controlled `unsupported-type-500` subject against
a clean baseline. Its Make target treats replay exit code 1 as the expected
result, then leaves the failed shared CI artifact available for inspection.
This is a useful distinction for CI: a known behavioral red is successful
regression coverage, while an unevaluable replay remains an infrastructure
error.

The `status-200-create` defect also exposed a separate edge case: one visible
failure can prevent later stateful cases from becoming evaluable. Replay keeps
that run `inconclusive` rather than overstating it as drift, while retaining the
failed replay verdict and the visible rule difference in the report.

The replay regression target now exercises both classifications together:
`unsupported-type-500` is a complete behavioral red (`failed` / `drifted`),
while `status-200-create` is an incomplete stateful experiment (`error` /
`inconclusive`) with its observed failed verdict preserved inside the report.
The `process-stays-queued` and `persistence-wrong-key` defects follow the same
incomplete-state classification, covering a failed process transition and a
broken captured document identity respectively.

The matrix also covers the remaining complete defects: `remove-name-create`
checks response-shape enforcement, and `accepts-png` checks input-validation
enforcement. Both should remain fully evaluable and produce `failed` /
`drifted` CI results.

The replay-matrix adapter is the next aggregation boundary. It consumes the
six producer-owned `behavioral-replay` envelopes, validates each nested replay
report and the outer CI status mapping, hashes every member envelope, and
preserves those references as `replay:<id>` inputs. The matrix has its own
explicit expectations, so intentional reds are counted as matched regression
cases rather than being confused with an infrastructure error. The aggregate
only passes when all expected classifications match.

The document-pipeline expectations now live in the versioned
`sorna.replay-matrix-manifest/v1` YAML file under the example. The manifest is
hashed into the aggregate as `matrix_manifest`, which keeps the reviewed
expectations bound to the generated report instead of hiding them in a shell
recipe. JSON manifests are accepted as the same language-neutral shape.

The saved aggregate has a separate verification path:
`sorna evidence replay matrix verify`. This is deliberately independent of
creation. It re-hashes the member envelopes, revalidates each nested replay
report, and checks the compact explanation plus contract/run lineage against
the current bytes. An error matrix can still be verified as an error outcome,
but any member bytes that were available at creation must remain unchanged.
