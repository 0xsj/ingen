# A provider capability review should happen before execution

The mutation provider is now reviewable as its own no-execution artifact. The
`sorna mutation provider inspect` command loads the canonical campaign plan and
provider manifest, hashes the exact bytes, compares every plan mutation with a
prepared entry and a declared plane/operator/target capability, and reports
whether the provider is ready.

The report uses `ingen.mutation-provider-review/v1`. It records:

- the plan and provider paths and hashes;
- whether an optional provider `plan_sha256` is matched, mismatched, or
  unbound;
- the complete declared capability list; and
- per-mutation entry and capability status.

This is deliberately separate from execution. A `ready` report means the
handoff is structurally covered; it does not prove the provider's mutation is
semantically correct, and it does not replace Sorna's pre-launch source and
binary hash checks for source-level providers.

The report can be printed for a human or saved as JSON:

```sh
sorna mutation provider inspect .artifacts/document-pipeline-mutation-plan.json \
  --provider examples/document-pipeline-lab/mutations/provider.yaml \
  --format json --output .artifacts/document-pipeline-provider-review.json
```

Existing report paths are refused so a later review cannot silently overwrite
an earlier artifact.

For CI collection, `--format ci-result` wraps the same report as
`ingen.ci-result/v1` with `kind: mutation-provider-review`. A ready report is
`passed`; a blocked report is `failed`; neither path starts a subject. Nublar
can aggregate this envelope with Sorna's behavioral-verification result while
leaving both producer reports intact.

## Used in

- [`sorna/internal/campaign`](../../sorna/internal/campaign/)
- [`sorna mutation provider inspect`](../../sorna/cmd/sorna/)
- [`document-pipeline provider`](../../examples/document-pipeline-lab/mutations/provider.yaml)
