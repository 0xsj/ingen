# A checksum-verified bundle proves artifact integrity, not isolation or correctness

The first Sorna evidence bundle makes the relationship between a run record,
lifecycle events, and their stored bytes inspectable. Its hashes detect later
tampering, but they do not prove that the contract was good, the subject was
isolated, or the original bytes were truthful.

## Origin

Managed lifecycle evidence initially lived only inside `run.json`. The next
slice added a manifest, append-only lifecycle JSONL, and a checksum file so a
consumer can verify the stored artifacts independently of the runner.

## What

`evidence.WriteBundle` writes `run.json`, `events/lifecycle.jsonl`,
`manifest.json`, and `checksums.sha256`. The lifecycle JSONL adds run identity
and actor fields to each process event. `evidence.Verify` reads the checksum
file, rejects unsafe or duplicate paths, hashes each listed artifact, and
stops at the first mismatch.

## Why

A run result is easier to consume when there is one entrypoint and a small,
machine-checkable integrity surface. Keeping lifecycle events as JSONL also
leaves room for future append-only access and policy events without making the
run record the only history format.

## Example

```sh
make sorna-run
make evidence-verify
```

The current bundle contains run and lifecycle artifacts plus the manifest. It
does not yet contain oracle source, frozen cases, implementation revision, or
capability-policy evidence.

## Gotchas

- A valid checksum only says the current bytes match the checksum file.
- The checksum file is not self-hashed; a trusted copy or higher-level custody
  mechanism is needed to protect the checksum root itself.
- Hashes establish identity and integrity, not that the contract is correct or
  that the subject never accessed forbidden data.
- The bundle currently records lifecycle events after the run; a future
  streaming writer should preserve event ordering when runs are long-lived.

## Used in

- [`sorna/internal/evidence`](../../sorna/internal/evidence/)
- [`sorna evidence verify`](../../sorna/cmd/sorna/)
- [`run artifacts`](../../.artifacts/)
- [`Sorna evidence specification`](../../sorna/EVIDENCE-SPEC.md)

## Related

- [`A managed subject lifecycle improves reproducibility without proving isolation`](sorna-managed-subject-lifecycle.md)
- [`A subject URL is not an isolation boundary`](../concepts/a-url-is-not-an-isolation-boundary.md)
- [`A fair mutation comparison requires the same sealed contract`](../concepts/fair-mutation-comparison-requires-the-same-contract.md)
