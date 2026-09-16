# Replay results can cross the shared CI boundary

## Finding

Replay has richer producer states than the language-neutral CI envelope. A
coordinator needs to preserve the replay report while using only the shared
three-state decision:

| Replay state | CI state | Meaning |
| --- | --- | --- |
| `matched` | `passed` | Contract-visible outcomes matched. |
| `drifted` | `failed` | A rule or contract verdict changed. |
| `error` / `inconclusive` | `error` | The new subject could not be evaluated completely. |

An observation-only change remains a passed CI result when the
contract-visible outcome still matches, but the nested report and explanation
retain the changed observation count.

## Implementation

`BuildReplayCIResult` calls the existing `Replay` boundary and writes the full
`sorna.replay/v1` report into the `report` field of an
`ingen.ci-result/v1` artifact with `kind: behavioral-replay`.

The envelope inputs include the canonical oracle, evidence manifest,
checksums, and every artifact named by the verified checksum stream. This
makes the coordinator's input view agree with what Sorna actually consumed.

When loading or verification fails before a replay report exists,
`BuildReplayCIErrorResult` emits the shared error form and retains hashes for
any available inputs.

## Command

```sh
sorna evidence replay --format ci-result \
  --oracle <frozen-oracle.json> --base-url <equivalent-subject-url> \
  --output replay-ci-result.json <evidence-directory>
```

The report is written with exclusive creation, so a failed CI decision remains
available to the collector and an existing result is not silently replaced.
