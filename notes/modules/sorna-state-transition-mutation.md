# Sorna state-transition mutation checkpoint

## What changed

The document-pipeline Go provider now supports a targeted
`state.transition.replace` mutation. The first instance changes
`processDocument` from:

```go
doc.Status = "completed"
```

to:

```go
doc.Status = "queued"
```

The mutation targets `POST /documents/{id}/process`, records the exact source
location and before/after statements, and requires exactly one matching AST
assignment before changing the copied source.

## Why it matters

Response status and field mutations test the shape of a single boundary
response. A state-transition mutation tests whether the operation actually
advances the subject's public lifecycle. The expected rule is
`document.process.valid-completes`; later stateful rules may become
inconclusive if their setup depends on completion, which is why the campaign
diagnosis must distinguish direct failure from setup fallout.

## Provider design lesson

The operator uses a semantic target rather than a raw source line. The Go
provider still owns the language-specific AST matching, while the catalogue
owns the public target, declared change, and expected contract rule. This keeps
the mutation handoff language-neutral even though the first implementation is
Go-specific.

The prebuilt fixture mirrors the observable process defect for local workflow
demonstration; the source-level provider remains the authoritative path for
provenance and exact target resolution.

## Result

The fresh document workflow killed this mutation directly through
`document.process.valid-completes`. The dependent
`document.result.exposes-derived-metadata` rule became inconclusive because its
setup requires a completed document; the other five rules were unaffected.
The full four-mutation campaign passed with 4/4 killed and no execution errors,
and Nublar passed all four required checks.
