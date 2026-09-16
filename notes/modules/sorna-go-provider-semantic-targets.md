# A source provider should resolve the contract target, not a stale helper name

The document-pipeline campaign exposed a useful distinction in source-level
mutation work. The reviewed mutation targets `POST /documents` and changes the
successful response status. The Go subject had refactored its implementation
from `writeJSON` to `writeJSONWithEvents`, but the provider still searched only
for the old helper name. Preparation then reported zero candidates even though
the public behavior still had exactly one matching response.

The provider now accepts both response helpers for the document subject, still
requires exactly one matching AST call, and records the helper actually found
in provenance. This keeps the mutation vocabulary language- and
implementation-detail-aware at the right level: the catalogue names the public
target and semantic change, while the provider owns how that target is located
in one SDK or language.

The lesson is not to make source matching permissive. Accepting several known
implementation shapes is safe only when the semantic filters and exact-one
candidate rule remain strict. Unknown shapes should still stop preparation;
otherwise a campaign could produce a binary whose edit is not the reviewed
mutation.

The provider's normal output directories remain non-reusable by design. The
root Makefile therefore exposes `mutation-go-campaign-ci-result-fresh`, which
copies the repository into a temporary workspace before generating artifacts.
That makes repeated local and CI proofs reproducible without making ordinary
commands destructive.

## Verification

The focused Go provider tests pass. A fresh six-mutation document-pipeline
campaign prepared and built every variant, killed all six mutations, produced
zero survivors/inconclusive/errors, and emitted a passing
`ingen.ci-result/v1` envelope.

## Used in

- [`sorna-go-provider`](../../sorna/cmd/sorna-go-provider/)
- [`Makefile`](../../Makefile)
- [`document-pipeline mutation catalogue`](../../examples/document-pipeline-lab/mutations/catalogue.yaml)
