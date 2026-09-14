# A policy declaration is not an enforcement result

Sorna needs a precise access policy before it can claim that an oracle could
not inspect implementation data. The v1 policy records intended filesystem,
network, process, and tool capabilities, but a `declared-only` policy does not
make those restrictions real.

## Origin

The lifecycle and evidence slices made process control and artifact integrity
visible, but they still left the oracle access boundary unspecified. This
slice introduced a sealed policy artifact and attached it to evidence bundles.

## What

`sorna/internal/policy` validates and seals policies with filesystem `read`,
`write`, and `deny` roots; network mode and allowlist entries; subject-invocation
permission; and optional allowed tools. The policy reference and canonical
policy bytes are copied into the evidence bundle and included in its checksums.

## Why

An enforcement backend cannot safely infer least privilege from a command or a
workspace layout. Explicit reasons make policy reviewable, while the
enforcement field prevents a declaration from being confused with a host
denial.

## Example

```sh
make policy-validate
go run ./sorna/cmd/sorna policy seal \
  examples/document-pipeline-lab/policy/isolation.yaml \
  --output-dir .artifacts/document-pipeline-policy
```

The lab oracle policy uses `enforcement: host-enforced`, disables network
access for oracle generation, and denies the subject, defect, and Git roots.
The separate managed-subject policy uses the same claim while allowing only
its compiled binary root and declared local listener.

## Gotchas

- A denied path in YAML does nothing until a host enforcement backend applies
  it.
- Exact-path overlap is rejected in v1, but parent/child and symlink semantics
  still need an enforcement-specific design.
- A policy hash proves which policy bytes were recorded, not that the policy
  was truthful or applied.
- The oracle and managed-subject policies are separate. The runner remains
  assurance level 0 even when the subject policy is host-enforced because the
  policy and access records are not independently attested.

## Used in

- [`sorna/internal/policy`](../../sorna/internal/policy/)
- [`sorna policy validate`](../../sorna/cmd/sorna/)
- [`evidence manifest policy reference`](../../sorna/internal/evidence/)
- [`document-pipeline isolation policy`](../../examples/document-pipeline-lab/policy/isolation.yaml)

## Related

- [`Sorna policy specification`](../../sorna/POLICY-SPEC.md)
- [`Sorna and Sentinel isolation threat model`](../../ISOLATION-THREAT-MODEL.md)
- [`A checksum-verified bundle proves artifact integrity, not isolation or correctness`](sorna-evidence-bundle.md)
