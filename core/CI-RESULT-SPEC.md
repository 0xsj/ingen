# InGen CI result artifact

Status: working contract

`ingen.ci-result/v1` is the language-neutral envelope that lets Nublar and
other CI surfaces consume deterministic tool results without reimplementing the
tool's verification semantics.

The producer owns the nested report and explanation schemas. The envelope owns
run identity, status, exit-code semantics, source identity, and input hashes.

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

`sha256` values identify the exact policy and optional baseline inputs used for
the run. They are integrity references, not an attestation by themselves.

## Paddock producer

Paddock writes this envelope with:

```sh
paddock ci . \
  --policy paddock.yaml \
  --baseline paddock-baseline.json \
  --output paddock-ci-result.json
```

The command exits with the envelope's `exit_code`, including after writing a
failed result. This makes the file available to a CI collector even when the
gate fails.
