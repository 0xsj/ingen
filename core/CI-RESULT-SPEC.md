# InGen CI result artifact

Status: working contract

`ingen.ci-result/v1` is the language-neutral envelope that lets Nublar and
other CI surfaces consume deterministic tool results without reimplementing the
tool's verification semantics.

The producer owns the nested report and explanation schemas. The envelope owns
run identity, status, exit-code semantics, source identity, and input hashes.
The `policy` field is optional for producers that do not use a policy file;
`inputs` provides a common index for other material inputs.

```json
{
  "schema": "ingen.ci-result/v1",
  "tool": "paddock",
  "kind": "architecture",
  "status": "passed",
  "exit_code": 0,
  "created_at": "2026-09-14T12:00:00Z",
  "source": {
    "root": "/workspace/service",
    "module_path": "example.com/service"
  },
  "policy": {
    "path": "paddock.yaml",
    "sha256": "..."
  },
  "policy_lock": {
    "path": "paddock.lock.json",
    "sha256": "..."
  },
  "graph": {
    "path": "paddock-graph.json",
    "sha256": "..."
  },
  "inputs": {
    "manifest": {
      "path": "manifest.json",
      "sha256": "..."
    }
  },
  "report": {},
  "explanation": {}
}
```

## Status

The envelope preserves the producer's gate result:

| Status | Exit code | Meaning |
| --- | ---: | --- |
| `passed` | `0` | The architecture policy passed. |
| `failed` | `1` | The policy ran and found blocking violations. |
| `error` | `2` | The policy or source could not be evaluated. |

An artifact with `status: error` contains `error` and may omit `report` and
`explanation`. A successful or failed artifact contains both nested artifacts.
Nublar should report the status and retain the nested evidence; it must not
reinterpret findings or turn an explanation into an authoritative verdict.

`sha256` values identify exact input files used for the run. They are integrity
references, not an attestation by themselves. A producer may use the named
top-level fields for well-known inputs and `inputs` for additional files. A
consumer should preserve unknown input names and must not infer correctness
from a hash alone.

## Paddock producer

Paddock writes this envelope with:

```sh
paddock ci . \
  --policy paddock.yaml \
  --policy-lock paddock.lock.json \
  --baseline paddock-baseline.json \
  --output paddock-ci-result.json
```

The command exits with the envelope's `exit_code`, including after writing a
failed result. This makes the file available to a CI collector even when the
gate fails.

For locked-policy execution, `--policy-lock` may be supplied without
`--policy`; Paddock evaluates the canonical policy embedded in the lock and
retains the original policy path and source hash as provenance.

## Sorna producer

Sorna adapts its verified bundle gate to the same envelope:

```sh
sorna gate --format ci-result .artifacts/document-pipeline-run \
  > .artifacts/document-pipeline-ci-result.json
```

The envelope uses `tool: "sorna"` and `kind: "behavioral-verification"`.
`report` contains the producer-owned `ingen.gate/v1` result, while
`explanation` contains `sorna.gate-explanation/v1`. The `status` and
`exit_code` remain the authoritative coordinator fields: a killed mutation is
a passing Sorna sensitivity result even though its nested contract verdict is
expected to be a failure.

Sorna indexes the verified manifest, run or oracle artifact, event streams, and
policy copy under `inputs` when those files exist. This makes the envelope
useful to a coordinator without pretending that a bundle path or hash proves
isolation, correctness, or complete observation.

If the bundle cannot be verified, Sorna emits the envelope's `error` form when
`--format ci-result` is requested, preserving exit code `2` for the collector.

## Nublar consumer

Nublar composes multiple envelopes without importing their producer packages:

```sh
nublar aggregate --output nublar-result.json \
  sorna-ci-result.json paddock-ci-result.json
```

Its producer-owned aggregate schema is `ingen.nublar-result/v1`. The result
preserves each complete input artifact under `results` and computes only the
aggregate status and exit code:

| Input status present | Aggregate status | Exit code |
| --- | --- | ---: |
| all `passed` | `passed` | `0` |
| any `failed`, no `error` | `failed` | `1` |
| any `error` | `error` | `2` |

This is severity composition, not finding interpretation. Nublar does not
decide whether a mutation was killed, whether an architecture rule is valid,
or whether an observation gap is acceptable; those decisions remain in the
producer envelope and report.

Nublar can make the input set explicit with `ingen.nublar-workflow/v1`:

```yaml
schema: ingen.nublar-workflow/v1
id: document-pipeline-ci
checks:
  - id: behavioral-verification
    tool: sorna
    result: .artifacts/document-pipeline-ci-result.json
    required: true
```

Checks are required by default. A missing required result or a result whose
`tool` does not match the declaration produces an aggregate `error` with exit
code `2`; an explicitly optional missing result becomes a warning. The
aggregate records the workflow file path and SHA-256, which identifies the
exact collection policy used for the decision. Each consumed CI-result file is
also recorded with its own SHA-256.
