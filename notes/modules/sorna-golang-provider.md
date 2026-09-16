# A Go provider must mutate a copy before building

The first language-specific provider is intentionally a thin Go source
preparation layer. It consumes the already-reviewed canonical
`ingen.mutation-plan/v1` in memory, copies the source root once per mutation,
applies a lab-specific AST transformation to that copy, builds a fresh binary,
and emits the existing `ingen.mutation-provider/v1` manifest. The generated
manifest also carries `plan_sha256` and exact capabilities for the mutation
shapes it prepared; Sorna rejects it when the manifest was prepared from a
different plan or when a plan asks for an undeclared capability.

The boundary matters:

- the campaign still owns the contract, frozen oracle, baseline, expected rules,
  and classification;
- the Go provider owns source copying, Go parsing/building, and prepared binary
  paths;
- Sorna still owns the managed process, public-boundary execution, evidence,
  and mutation outcome.

For the document lab, the operators are deliberately narrow:

- `response.status.replace` at `POST /documents`, from status 202 to 200. The
  callback finds the `createDocument` function and changes the
  `http.StatusAccepted` selector to `http.StatusOK` using `go/ast`.
- `response.field.remove` at `POST /documents`, removing the `name` key from
  the successful response map. The callback resolves the exact `writeJSON` or
  `writeJSONWithEvents` response literal and removes one matching key.

Both operators require exactly one matching AST target. An ambiguous or
missing source shape is a preparation error instead of silently producing an
unknown mutation. Successful entries now record `target_resolution` alongside
the edit provenance: the selector, number of candidates, and number applied.
The provider returns `ingen.mutation-target-resolution-error/v1` when the
candidate count is zero or greater than one, with `applied_count: 0`; the
source copy is not written in either case.

The provider keeps copied source variants under its output directory for
review, while placing only built binaries under the managed subject's allowed
read root. The original source tree is never an output target, and existing
provider output directories are rejected to prevent accidental reuse. Source
and binary outputs are staged and published only after every planned variant
has been prepared, so an AST or build failure does not publish a partial
provider.
Generated dependency/cache trees such as `node_modules`, `.artifacts`, and
`.cache` are excluded; symlinks in the remaining source tree are rejected so
the copied build input cannot silently escape the source root.

Each generated entry records semantic edit provenance (`location`, `before`,
and `after`) plus target resolution, the relative copied source directory, a
deterministic hash of that source tree, and the built binary hash. The campaign
executor verifies the two byte identities immediately before launch. This closes the practical
time-of-check/time-of-use gap between provider preparation and subject start;
it does not claim that the provider itself is independently trusted.

The command also writes `preparation.json` using the
`ingen.mutation-preparation/v1` schema. It records the plan hash, changed source
files, source/binary identities, and the retained provenance for every variant.
The provider rejects a no-op mutation before building or publishing it, so a
successful summary represents an actual source change.

## Command

```sh
make mutation-go-provider-build
make mutation-go-campaign-run
make mutation-go-campaign-verify
```

Use `--summary-output` to choose a different preparation-summary path. The
default is `<output-dir>/preparation.json`.

The generated provider manifest remains the normal Sorna handoff. Its exact
bytes are copied into each mutation evidence bundle, while the managed-subject
evidence records the executable identity that actually ran.

## Limits

This is not a general Go mutation engine. It proves the source-copy, AST
transformation, isolated build, provenance, and Sorna handoff mechanics for
two controlled response operators. Future operators need their own
target-resolution and ambiguity rules, plus provider-level build diagnostics
and source identity. The target-resolution error is deliberately
language-neutral; its selector remains provider-owned.

## Used in

- [`sorna/providers/golang`](../../sorna/providers/golang/)
- [`sorna-go-provider`](../../sorna/cmd/sorna-go-provider/)
- [`document-pipeline catalogue`](../../examples/document-pipeline-lab/mutations/catalogue.yaml)
