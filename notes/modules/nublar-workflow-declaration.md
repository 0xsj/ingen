# A CI workflow should declare its expected evidence

The first Nublar aggregator accepted an arbitrary list of result files. That
proved the shared envelope boundary, but it left the caller responsible for
remembering which checks a workflow required.

`ingen.nublar-workflow/v1` makes that expectation explicit. Each check names an
ID, expected producer tool, result path, and optional `required` flag. Checks
are required by default; optionality must be stated with `required: false`.

Nublar resolves the declared paths under a workspace root and validates the
loaded `ingen.ci-result/v1` artifact. A missing required result or producer
identity mismatch is an evaluation error (`exit_code: 2`). A missing optional
result is a warning. Present results are then composed using only their shared
envelope status. The aggregate records the workflow path and SHA-256 so the
collection policy is itself part of the provenance. Each result file is also
hashed at the point Nublar reads it, binding the recorded path to the exact
artifact that was consumed.

## Why this belongs in Nublar

Sorna owns whether behavioral verification passed and Paddock owns whether an
architecture policy passed. Nublar owns the workflow question: which producer
results must exist for this CI run to be meaningful? This is a real boundary,
not an artificial split, because it depends on the delivery workflow rather
than on either verifier's domain semantics.

## Limits

The workflow is currently a local YAML/JSON declaration. It does not yet
contain semantic sealing, branch rules, approvals, schedules, retries, artifact
retention, or hosted CI integration. Its current SHA-256 is exact file
provenance, not a claim that semantically equivalent YAML files are identical.
Those capabilities should be added only when a concrete workflow requires
them.
