# Conformance fixtures

[`v1.json`](v1.json) contains shared valid and invalid v1 wire values plus
deterministic `Child`, `Retry`, and `Replay` transition cases. IDs are fixed
only so implementations can compare decoded values; newly created executions
MUST still generate fresh IDs according to the specification.

Conformance runners SHOULD:

1. decode every value in `valid` and accept it;
2. decode every value in `invalid` and reject it; and
3. run every case in `transitions` with its `generated_ids` sequence and
   compare the resulting semantic value with `expected`; and
4. compare the semantic fields of equivalent values, ignoring JSON whitespace
   and object-key order.

The transition fixture uses `input` names from the `valid` cases. Implementations
may inject the listed IDs only in tests or controlled tooling; production
defaults must continue to use secure random UUIDv4 generation.
