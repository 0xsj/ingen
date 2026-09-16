# Sorna frozen-oracle validation

## Why the load boundary matters

The frozen oracle is the executable test authority for `sorna run --oracle`.
If Sorna accepts an empty oracle, the runner can evaluate zero rules and report
a green verdict. If it accepts duplicate rule IDs, mutation classification and
coverage can collapse multiple observations into one map entry. Both are
integrity problems at the artifact boundary, not subject behavior.

## Invariants

`oracle.Validate` now requires:

- at least one materialized case;
- a non-empty, unique `case_id` for every case;
- a non-empty, unique `rule_id` for every case;
- one of the contract strength values supported by `ingen.contract/v1`.

The validator does not require a subject for `unspecified` metadata. That
distinction remains deliberate: structural oracle validation should not erase
contract metadata that may be useful to a later execution policy.

## Canonical bytes and semantics

`oracle.LoadFile` still performs both checks: it validates the decoded shape and
requires the file bytes to equal Sorna's canonical JSON encoding. The new
identity checks run before canonical acceptance, so a syntactically valid but
ambiguous oracle cannot enter the runner or campaign planner.

The result is still an integrity guard, not an attestation that the oracle is
correct or that its author was independent of implementation source.
