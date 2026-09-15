# Document-pipeline defect variants

These subjects are controlled, named behavioral changes used to test whether
Sorna's contract and oracle are sensitive to a specific wrong behavior.

The variants are [`status-200-create/`](status-200-create/),
[`remove-name-create/`](remove-name-create/),
[`unsupported-type-500/`](unsupported-type-500/), and
[`process-stays-queued/`](process-stays-queued/),
[`persistence-wrong-key/`](persistence-wrong-key/), and
[`accepts-png/`](accepts-png/). They change a successful
`POST /documents` response from the required `202 Accepted` to `200 OK`,
remove its required `name` field, change the unsupported-document response
from `400 Bad Request` to `500 Internal Server Error`, or leave the public
process result queued instead of completed, while delegating all other
behavior to the clean subject. The persistence variant returns an ID that
cannot be read back through the public document endpoint. The input-validation
variant accepts a PNG document that the contract requires the subject to
reject.

This is a defect fixture, not yet a source-level mutation engine. The variant
is intentionally explicit so its expected observability can be reviewed before
we generalize mutation generation.
