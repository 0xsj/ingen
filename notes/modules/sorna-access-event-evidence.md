# Host access telemetry is evidence, not an absence proof

The sandbox policy is the enforcement mechanism. Host access events are an
observation of what the operating system reported while that mechanism was in
effect.

## Origin

The Seatbelt backend already denied undeclared project access, but its evidence
bundle only recorded the policy and process lifecycle. That made a denied
implementation read visible in a manual log without making it part of the
artifact that Sorna verifies.

## What changed

On macOS, Sorna records the oracle child PID and queries the unified log for the
exact process execution window. Primary Seatbelt messages are normalized into
`events/access.jsonl` with the process, decision (`allow` or `deny`), operation,
and resource. Duplicate-report envelopes are ignored rather than counted as
separate accesses.

The oracle manifest records `access_telemetry` and a separate assurance status:
`host-enforced-observed` means the host log query completed; it does not mean
that every possible access was observed. Parse gaps produce a distinct
`host-enforced-observed-with-gaps` status. An unavailable collector produces
`telemetry-unavailable`.

## Why this matters

This separates three claims that are easy to collapse:

1. the policy was declared;
2. the host applied the policy;
3. the host reported particular access decisions.

The current oracle assurance remains level 0 because unified-log telemetry is
observational and applies only to oracle generation. The subject run still
cannot claim capability-isolated assurance merely because the oracle was
generated under Seatbelt.

## Findings

- A live `log stream` can miss a very short-lived child, so the Darwin backend
  queries `log show` after exit using a padded start/end window and exact PID
  predicate.
- The first real freeze recorded denials for the child executable read and the
  system log socket while still producing the oracle. These are useful signals
  that bootstrap allowances and denied network behavior need review; they are
  not silently discarded.
- An empty access file is a valid captured observation, not proof that no
  access occurred.

## Used in

- [`sorna/internal/sandbox`](../../sorna/internal/sandbox/)
- [`sorna/internal/evidence`](../../sorna/internal/evidence/)
- [`sorna oracle freeze`](../../sorna/cmd/sorna/)

## Related

- [`A host policy backend needs bootstrap permissions and canonical paths`](sorna-sandbox-enforcement.md)
- [`A policy declaration is not an enforcement result`](sorna-policy-definition.md)
- [`A checksum-verified bundle proves artifact integrity, not isolation or correctness`](sorna-evidence-bundle.md)
