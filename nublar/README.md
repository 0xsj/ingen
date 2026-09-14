# Nublar

Nublar is a future placeholder for InGen's CI and delivery surface. It may
eventually provide pull-request gates, scheduled mutation campaigns, hosted
reports, retention, and delivery-system integrations.

Nublar should consume Sorna's CLI, run specifications, evidence bundles, and
the shared [`ingen.ci-result/v1`](../core/CI-RESULT-SPEC.md) envelope produced
by tools such as Paddock. It should not reimplement architecture evaluation,
contract evaluation, or mutation semantics.

For Paddock, Nublar should store and surface the result status, exit code,
input hashes, deterministic report, and explanation. The nested Paddock report
remains the source of architectural findings; Nublar is a CI consumer, not a
second architecture engine.
