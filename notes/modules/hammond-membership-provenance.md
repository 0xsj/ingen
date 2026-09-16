# Decision provenance should name the membership snapshot

An injected authority verifier can affect approval state, so the decision
history should identify the membership snapshot the caller says it used.

## Origin

Hammond added signed, fresh, time-scoped membership snapshots and allowed
callers to inject their verifier into policy evaluation. The event history did
not yet carry the snapshot reference, leaving an audit reader unable to connect
an approval to its authority input.

## What

Decision events may carry an optional `membership` reference. Hammond validates
its membership schema, identity, and artifact digest shape and preserves it
through strict JSON decoding. `MembershipSnapshot.VerifierAtWithProvenance`
and the matching HTTP provider helper return a verifier that lets Hammond
enforce the reference match during policy evaluation.

## Why

The event history can now retain the authority source named by the caller while
keeping network access and freshness policy outside record validation. This
supports later audit tooling without coupling every record load to a live
provider.

## Gotchas

- The caller must attach the reference that actually produced the injected
  verifier. Hammond compares it when the caller uses its provenance-carrying
  membership verifier, but cannot inspect an opaque custom verifier.
- Hammond validates the reference but does not automatically load, signature-
  check, or freshness-check the membership artifact during record validation.
- The reference identifies the normalized membership envelope; raw provider
  provenance still belongs with the caller's integration record.
- References are valid only on approval and rejection events, not lifecycle or
  lineage events.

## Used in

- `hammond/internal/governance/types.go`
- `hammond/internal/governance/validate.go`
- `hammond/internal/governance/governance_test.go`
- `hammond/spec/ingen.hammond-governance-v1.schema.json`

## Related

- [An authenticated membership response is still a provider boundary](hammond-membership-provider.md)
- [Freshness is a caller policy, not signature metadata](hammond-membership-freshness.md)
- [Normalization can reject an incomplete provider view](hammond-membership-normalization.md)
