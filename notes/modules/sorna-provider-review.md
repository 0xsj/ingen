# A provider capability review should happen before execution

The mutation provider is now reviewable as its own no-execution artifact. The
`sorna mutation provider inspect` command loads the canonical campaign plan and
provider manifest, hashes the exact bytes, compares every plan mutation with a
prepared entry and a declared plane/operator/target capability, and reports
whether the provider is ready.

The report uses `ingen.mutation-provider-review/v1`. It records:

- the plan and provider paths and exact hashes;
- the stable semantic plan identity and, when declared, the provider's
  semantic identity binding;
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

Unbound providers remain available for local fixture demonstrations, where the
provider is a reviewed static handoff. A stricter caller can require the
provider to declare the exact plan hash:

```sh
sorna mutation provider inspect plan.json \
  --provider provider.yaml \
  --require-plan-binding
```

In this mode, an unbound provider is `blocked`, just like a mismatched provider.
The same flag is available on `sorna mutation run`, so a campaign executor can
enforce the policy even if a separate preflight was skipped. The generated Go
provider is the first example that produces a matched exact plan binding. The
semantic identity is reported alongside it but is not required by this flag.

For CI collection, `--format ci-result` wraps the same report as
`ingen.ci-result/v1` with `kind: mutation-provider-review`. A ready report is
`passed`; a blocked report is `failed`; neither path starts a subject. Nublar
can aggregate this envelope with Sorna's behavioral-verification result while
leaving both producer reports intact.

The default document-pipeline Nublar workflow uses the generated Go provider
and passes `--require-plan-binding`. The static fixture provider remains a
useful local demonstration, but it is intentionally not the production-style
workflow input.

## Conformance fixtures

The checked-in provider conformance corpus exercises the handoff without
starting a subject. It includes a valid unbound fixture, exact plan-byte drift,
semantic-plan drift, an unsupported capability, partial preparation, and a
malformed entry. The cases make the distinction visible between a provider
that can be parsed, one that covers the plan, and one that is allowed to run.

Run the focused acceptance command with:

```sh
make mutation-provider-conformance
```

## Used in

- [`sorna/internal/campaign`](../../sorna/internal/campaign/)
- [`sorna mutation provider inspect`](../../sorna/cmd/sorna/)
- [`document-pipeline provider`](../../examples/document-pipeline-lab/mutations/provider.yaml)
