# Shared CI exit-code contract

A shared CI envelope needs one status-to-exit-code mapping so coordinators do
not reimplement producer semantics.

## Origin

This note came from comparing Sorna and Nublar's independently maintained
status mappings at the alpha CI boundary.

## What changed

The shared `core/ciresult` package now owns the status mapping used by the
language-neutral envelope:

| Status | Exit code | Meaning |
| --- | ---: | --- |
| `passed` | `0` | The producer evaluated its gate successfully. |
| `failed` | `1` | The producer evaluated its gate and found a blocking result. |
| `error` | `2` | The producer could not evaluate its gate or verify its inputs. |

Sorna's behavioral, provider-review, and mutation-campaign envelopes and
Nublar's aggregate now consume the same mapping. Unknown statuses are rejected
instead of being silently treated as infrastructure errors.

## Why it matters

Nublar should compose producer decisions without importing producer-specific
semantics. That only works if the outer status and exit code have one meaning
across tools. Sorna can still retain richer distinctions such as a killed,
survived, or inconclusive mutation inside its nested report and explanation;
those details do not change the coordinator-level mapping.

## Finding

Exit codes are an orchestration contract, not a complete explanation. A
successful mutation campaign has a `passed` envelope even though the nested
subject run for each killed mutation is intentionally contract-failing. The
producer report remains the place to interpret that domain-specific result.

The mapping is implemented by `ciresult.ExitCodeForStatus` and validated by
the shared envelope tests.

## Used in

The shared `core/ciresult` envelope, Sorna CI producers, and Nublar aggregate
composition.
