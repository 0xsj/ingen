# Run and evidence schemas keep the execution handoff language-neutral

The frozen oracle is not the final Sorna artifact. A subject run records what
happened when that oracle met an implementation, while the evidence manifest
binds the run to policies and checksummed files. These are related artifacts,
but they have different ownership:

```text
frozen oracle -> subject run -> evidence manifest -> checksummed bundle
```

Sorna now publishes structural JSON Schemas for both `ingen.run/v1` and
`sorna.evidence/v1`. A future SDK can read the run and manifest without linking
against Go or importing Sentinel/Herdr. The run schema includes per-rule
requests, observations, assertion outcomes, and optional lifecycle or
mutation detail. The evidence schema intentionally contains references and
hashes rather than duplicating those observations.

The Go read boundary now mirrors that contract: `runner.LoadFile` and
`evidence.LoadManifestFile` reject unknown fields, trailing JSON values, and
ambiguous identity or lineage fields. Evidence verification uses those loaders
before checking bundle checksums, while the low-level writers remain usable for
focused construction tests.

## Boundary lesson

The schema is a shape contract, not a correctness proof. The evidence verifier
still checks checksums and cross-artifact identity, and the runner remains the
authority for producing a verdict. Keeping those responsibilities separate
avoids making JSON Schema pretend it can prove that a subject behaved correctly
or that host observation was complete.

The package constants are used by producers and consumers so a published
identity cannot silently drift from the runtime comparison:

- `runner.Schema == "ingen.run/v1"`;
- `evidence.Schema == "sorna.evidence/v1"`.

## Used in

- [`ingen.run-v1.schema.json`](../../sorna/spec/ingen.run-v1.schema.json)
- [`sorna.evidence-v1.schema.json`](../../sorna/spec/sorna.evidence-v1.schema.json)
- [`sorna/internal/runner`](../../sorna/internal/runner/)
- [`sorna/internal/evidence`](../../sorna/internal/evidence/)

## Related

- [`A frozen oracle needs a language-neutral handoff shape`](sorna-oracle-schema.md)
- [`A checksum-verified bundle proves artifact integrity, not isolation or correctness`](sorna-evidence-bundle.md)
- [`A subject run consumes the frozen oracle, not the contract source`](sorna-run-consumes-frozen-oracle.md)
