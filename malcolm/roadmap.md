# Malcolm roadmap

Status: experimental Rust-to-Sorna vertical slice  
Last reviewed: 2026-09-17

This document is the working map for Malcolm-specific progress. Detailed
language decisions and implementation notes live in
[`notes/modules/malcolm-*.md`](../notes/modules/).

## Where we are

Malcolm currently has one complete executable path:

~~~text
Malcolm source
  → Rust parser and semantic validation
  → malcolm.ir/v1 JSON
  → Sorna Go adapter
  → ingen.contract/v1 contract
  → HTTP execution and evidence
~~~

The path is proven with a stateful document flow. The clean subject passes all
8 executable cases. A controlled defect that removes only the
`document.accepted` event produces 2 expected failures (event presence and
event order), so the mutation is killed while the other 6 cases still pass.

The current slice supports:

- named specs and scenarios
- `given` request bodies with escaped strings, signed integers, booleans,
  nested objects and arrays (including multiline literals), and bounded repeat
  generators
- named setup requests, stateful execution, top-level response captures, and
  URL substitution
- capture interpolation in later request bodies
- dotted object-path response selectors for assertions and captures
- status, top-level response-field existence, and top-level response-field
  equality assertions
- positive and negative requirements
- event-presence and same-response relative-order assertions
- one implementation-plane response-status mutation declaration with an
  explicit expected failing rule
- one specification-wide execution-ID provenance declaration observed through
  the public Amber-Provenance response header
- one specification-wide per-scenario isolation declaration observed through
  a subject-owned reset request before each executable case
- digest-pinned oracle fixture declarations with explicit ownership and
  purpose, lowered to Sorna contract fixture metadata
- provider-supplied relative fixture paths verified against the sealed
  contract digest and emitted as a versioned fixture handoff artifact
- strict translation into the Sorna contract format
- HTTP execution, evidence bundles, and a controlled mutation proof

The implementation is intentionally narrower than the syntax shown in the
original design direction. Malcolm does not yet support array-index response
selectors or richer provenance declarations. Mutation declarations remain
limited to the response-status handoff documented below.

## Completed milestones

- [x] Scaffold a dependency-free Rust crate and CLI under `malcolm/`.
- [x] Parse the first line-oriented language slice: `spec`, `subject`,
  `scenario`, `given`, `when`, `must`, and `must_not`.
- [x] Add line-aware parse errors and semantic validation for missing
  structure, duplicate names, and duplicate fields.
- [x] Define and emit the language-neutral `malcolm.ir/v1` JSON
  intermediate representation with deterministic handwritten serialization.
- [x] Lower typed scalar request data into the IR.
- [x] Extend request data to recursive inline objects and arrays.
- [x] Add named setup requests, state, positive and negative setup
  requirements, response captures, and URL substitution.
- [x] Add target status/body assertions and event presence/order assertions.
- [x] Add deterministic bounded repeat generators for request-body values.
- [x] Add capture interpolation in later request bodies with strict reference
  validation.
- [x] Add dotted object-path response selectors with explicit null and missing
  value semantics.
- [x] Extend quoted-string escapes and allow multiline nested object and array
  request-body literals with line-aware delimiter diagnostics.
- [x] Add typed response-status mutation declarations and lower them into
  Sorna's versioned mutation catalogue.
- [x] Add a typed execution-ID provenance declaration and observe it through
  Sorna's public HTTP evidence boundary.
- [x] Add a typed per-scenario isolation declaration and record the
  subject-owned reset boundary before each executable case.
- [x] Add digest-pinned oracle fixture declarations with explicit ownership,
  purpose, and Sorna contract lowering.
- [x] Verify provider-supplied oracle fixture paths against the sealed contract
  and emit a portable `ingen.fixture-handoff/v1` artifact.
- [x] Align the README and notes with executable negative setup and event
  assertions.
- [x] Build a strict Sorna adapter that rejects unsupported IR rather than
  guessing at its meaning.
- [x] Build the Sorna HTTP runner with setup lifecycle, target/negative
  semantics, event observation, and evidence output.
- [x] Freeze a flow oracle and prove that a controlled event-removal defect is
  detected.
- [x] Record the implementation decisions in the Malcolm notes corpus.

## Next steps

### P2 — Provenance expansion

The first execution-ID declaration now has a typed IR, a Sorna contract
expectation, and a public-header evidence mapping. Richer provenance fields
and cross-request relationships still need an explicit evidence model and
should remain unsupported until Amber, Lockwood, and Nublar can consume them
without duplicated semantics.

Completion signal: a declaration has a typed IR representation, a clear
consumer, a contract/evidence mapping, and a controlled defect or negative
proof.

### P2 — Fixtures and isolation

Make test data and state reset semantics explicit. The reset hook now covers
repeatability between generated cases for the in-memory document fixture, and
digest-pinned oracle fixture identity and a file-based provider handoff are
explicit. The remaining fixture work should support an in-memory byte handoff,
cover setup cleanup beyond a single reset operation, and preserve the boundary
between Malcolm-declared data and subject-owned fixtures.

Completion signal for the reset sub-slice: repeated cases produce equivalent
results and reset evidence, with no accidental dependence on state left by a
previous scenario. Completion signal for the fixture-identity sub-slice:
fixture ownership and digest survive the Rust-to-Sorna handoff without
materializing subject-owned data. Completion signal for the file-handoff
sub-slice: a provider path resolves inside its root, its observed bytes match
the sealed contract digest, and `ingen.fixture-handoff/v1` records the binding.
The broader fixture milestone still needs provider-bound byte buffers or richer
cleanup semantics.

### P3 — Broader execution surfaces

Consider additional transports or subject integrations only after the
language-neutral IR and Sorna boundary are stable. Keep transport-specific
behavior in adapters and runners so Malcolm source remains portable.

## Definition of done for the next slice

Every new language feature should satisfy all of these:

1. The source syntax and its limits are documented with a small example.
2. The Rust parser and semantic layer preserve the intended typed meaning.
3. The IR version and compatibility rules are explicit.
4. The Sorna adapter rejects unsupported or ambiguous input.
5. The runner and evidence output demonstrate the behavior.
6. The feature has focused unit tests and one end-to-end example.
7. A controlled negative or mutation case proves that the assertion can fail.
8. The corresponding note is added or updated, and the repository diff is
   clean apart from the intended change.

## Boundaries to preserve

- Do not interpret free-form `given` text by guessing what it means.
- Keep the IR as a transport contract; do not hide evaluation logic in it.
- Keep the frozen oracle separate from the implementation under test.
- Treat setup failure as `inconclusive` for the target rather than as a target
  assertion failure.
- Treat public subject events as contract signals, distinct from internal
  telemetry or broker delivery.
- Do not claim that a passing Malcolm flow proves universal correctness.
- Keep unsupported syntax explicit until its semantics and evidence model are
  well-defined.

## Recommended order

1. Add an in-memory fixture-byte handoff only if a concrete consumer needs it.
2. Expand provenance only when a concrete Amber-backed evidence case requires
   it.

## References

- [Malcolm README](README.md)
- [First parser note](../notes/modules/malcolm-first-parser.md)
- [Parser robustness note](../notes/modules/malcolm-parser-robustness.md)
- [Mutation declarations note](../notes/modules/malcolm-mutation-declarations.md)
- [Provenance declarations note](../notes/modules/malcolm-provenance-declarations.md)
- [Semantic validation note](../notes/modules/malcolm-semantic-validation.md)
- [Request-body lowering note](../notes/modules/malcolm-request-body-lowering.md)
- [Capture interpolation note](../notes/modules/malcolm-capture-interpolation.md)
- [Response selector note](../notes/modules/malcolm-response-selectors.md)
- [Sorna bridge note](../notes/modules/malcolm-sorna-bridge.md)
- [Sorna runner note](../notes/modules/malcolm-sorna-run.md)
- [Isolation declarations note](../notes/modules/malcolm-isolation.md)
- [Fixture declarations note](../notes/modules/malcolm-fixtures.md)
- [Event assertions note](../notes/modules/malcolm-event-assertions.md)
- [Event mutation note](../notes/modules/malcolm-event-mutation.md)
- [End-to-end flow note](../notes/modules/malcolm-sorna-flow-run.md)
