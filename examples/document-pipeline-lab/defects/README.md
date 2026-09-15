# Document-pipeline defect variants

These subjects are controlled, named behavioral changes used to test whether
Sorna's contract and oracle are sensitive to a specific wrong behavior.

The variants are [`status-200-create/`](status-200-create/),
[`remove-name-create/`](remove-name-create/), and
[`unsupported-type-500/`](unsupported-type-500/). They change a successful
`POST /documents` response from the required `202 Accepted` to `200 OK`,
remove its required `name` field, or change the unsupported-document response
from `400 Bad Request` to `500 Internal Server Error`, while delegating all
other behavior to the clean subject.

This is a defect fixture, not yet a source-level mutation engine. The variant
is intentionally explicit so its expected observability can be reviewed before
we generalize mutation generation.
