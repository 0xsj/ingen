# Malcolm

Malcolm is an experimental executable specification language and adversarial verification tool for AI-generated software.

The project is named for Ian Malcolm: its purpose is to challenge systems that appear correct, expose shared assumptions, and make verification boundaries visible.

## Why it exists

When an AI generates both an implementation and its tests, the two can agree because they share the same mistaken assumptions. Malcolm is intended to provide an independent contract layer for describing what must be true, what evidence must exist, and which deliberate defects must be detected.

Malcolm should help answer questions such as:

- Did the implementation satisfy the declared contract?
- Did it preserve the required provenance relationships?
- Did the evidence support the verification result?
- Did the verification process detect the defects it was designed to catch?

## Example direction

```text
spec document_api v1 {
  subject "documents"

  scenario submit_document {
    given document is valid
    when POST "/documents"

    must response.status == 202
    must response.body.document_id exists
    must emit "document.accepted"
  }

  provenance {
    must create request_id
    must create execution_id
    must preserve tenant_id
    must link event to request
  }

  mutation "return-200" {
    target submit_document
    change response.status from 202 to 200
    expect rule submit_document.requirement.1 to fail
  }
}
```

The syntax is intentionally declarative and readable. The language is not intended to become a general-purpose programming language.

## Role in InGen

Malcolm would provide the specification surface for the wider InGen ecosystem:

- **Sorna** executes contracts, checks behavior, and runs mutation campaigns.
- **Amber** records provenance for subjects, executions, agents, and artifacts.
- **Paddock** evaluates architecture and policy constraints.
- **Nublar** coordinates verification results in CI.
- **Lockwood** preserves evidence bundles and artifacts.
- **Hammond** can eventually govern contract ownership, approval, and versioning.

The intended flow is:

```text
Malcolm spec → typed model → verification plan → evidence and provenance → CI result
```

## Initial scope

The first version should stay deliberately small:

- parse named specifications and scenarios
- support `given`, `when`, `must`, and `must_not` clauses
- model typed identifiers and relationships
- declare evidence requirements
- declare simple mutations
- emit a language-neutral intermediate representation
- integrate with one Sorna example

## Non-goals

Malcolm is not intended to:

- claim universal correctness for an entire system
- replace formal tools such as TLA+, Alloy, or Dafny
- generate tests without an independent contract or oracle
- bind specifications to one implementation language

## Implementation direction

The first implementation is planned in Rust. The likely pipeline is:

```text
source text → parser → typed AST → semantic validation → compiled outputs
```

Possible outputs include Sorna verification plans, Amber requirements, JSON fixtures, Markdown documentation, and Nublar CI envelopes.

## Try the first slice

The initial Rust crate parses and validates the core syntax, then emits a
language-neutral JSON intermediate representation:

```sh
cargo run --manifest-path malcolm/Cargo.toml -- \
  malcolm/examples/document_api.malcolm
```

Write the JSON to a file with `-o` or `--output`; use `-` as the input path to
read the specification from standard input.

## Executable request data

The first executable request slice uses a typed body block instead of treating
request JSON as free-form text:

~~~text
scenario create_document {
  given body {
    name = "welcome.md"
    published = true
    retries = 2
    metadata = {"source": "malcolm", "reviewed": true}
    tags = ["docs", "contract"]
  }
  when POST "/documents"
  must response.status == 202
}
~~~

Stateful scenarios can add named setup requests and capture a top-level
response field for a later URL:

~~~text
scenario read_document {
  state document_accepted
  setup accept_document {
    given body {
      name = "welcome.md"
    }
    when POST "/documents"
    must response.status == 202
    must_not response.body.error exists
    capture document_id = response.body.id
  }
  when GET "/documents/{document_id}"
  must response.status == 200
}
~~~

Run the cross-language validation proof with:

~~~sh
make malcolm-sorna-flow-contract
~~~

The executable HTTP slice also supports event presence assertions. The
subject must expose the event as a public response signal:

~~~text
when POST "/documents"
must emit "document.accepted"
must emit in order ["document.accepted", "document.queued"]
~~~

Sorna's HTTP runner observes this through the `X-InGen-Event` response header.
The ordered form requires the named events to appear in relative order in the
same response; unrelated events may appear between them.

## Deterministic generated values

Request bodies can use the bounded repeat generator for large or boundary
inputs:

~~~text
given body {
  name = "too-large.txt"
  content = repeat("a", 4097)
}
~~~

The count must be between 1 and 1,000,000, and the repeated value must be a
string. Sorna materializes the value while freezing the oracle, so replay uses
the concrete request body. The body field name `generated` is reserved by the
cross-language contract representation.

## Captured values in later request bodies

A later setup or target request body can substitute a string captured by an
earlier setup:

~~~text
setup create_seed {
  given body {
    name = "seed copy.txt"
  }
  when POST "/documents"
  must response.status == 202
  capture document_name = response.body.name
}
given body {
  name = "{document_name}"
}
~~~

Placeholders use the form `{capture_name}`. A setup can use captures from
earlier setups; the target request can use captures from any setup in its
scenario. Captures must be strings, missing or malformed references fail
validation, and body substitution preserves the captured text. URL-path
substitution remains path-escaped. This is data substitution only, not an
expression language.

## Nested response selectors

Assertions and captures can follow a dotted path through JSON objects:

~~~text
setup inspect_document {
  when GET "/documents"
  must response.body.metadata.owner.id exists
  capture owner_id = response.body.metadata.owner.id
}
~~~

Each path segment must be an identifier. This slice supports object traversal
only; array indexes are not supported yet. An exists assertion passes when the
final key is present, including when its value is JSON null. A missing key or
non-object intermediate value fails. Captures preserve the selected JSON
value, so a null or non-string capture cannot be used for string body
interpolation.

## Parser robustness

Quoted strings support the basic escapes `\"`, `\\`, `\b`, `\f`, `\n`, `\r`,
and `\t`. The parser decodes these escapes before validation and IR emission;
unsupported escapes, literal control characters, and unescaped inner quotes
produce line-aware parse errors.

Nested object and array request-body values may span multiple lines:

~~~text
given body {
  metadata = {
    "source": "import",
    "labels": [
      "docs",
      "contract"
    ]
  }
}
~~~

The surrounding specification, scenario, setup, and request-body block
grammar remains line-oriented. Multiline quoted strings, Unicode escape
forms, and array-index response selectors are not supported yet.

## Mutation declarations

The first mutation slice declares one implementation-plane HTTP status
mutation and the rule that should observe it:

~~~text
mutation "return-200" {
  target submit_document
  change response.status from 202 to 200
  expect rule submit_document.requirement.1 to fail
}
~~~

`target` names an existing scenario. The supported operator is
`response.status.replace`, and both status values must be between 100 and 599
and differ. The expected rule reference is checked against the target
scenario's generated requirement IDs.

Mutation declarations are preserved in `malcolm.ir/v1` and lowered into a
separate Sorna `ingen.mutation-catalogue/v1` artifact. Sorna remains
responsible for provider selection, campaign planning, execution, and
killed/survived evidence. Use the checked-in example with:

~~~sh
cargo run --manifest-path malcolm/Cargo.toml -- \
  malcolm/examples/document_mutation.malcolm \
  --output .artifacts/document-mutation.ir.json
go run ./sorna/cmd/sorna-malcolm \
  .artifacts/document-mutation.ir.json \
  --output .artifacts/document-mutation-contract.json \
  --mutations-output .artifacts/document-mutation-catalogue.json
go run ./sorna/cmd/sorna mutation validate \
  .artifacts/document-mutation-catalogue.json \
  --contract .artifacts/document-mutation-contract.json
~~~

## Provenance declarations

The first provenance slice declares that every target response must expose an
Amber `execution_id` through the public `Amber-Provenance` HTTP response
header:

~~~text
provenance {
  must create execution_id
}
~~~

This is intentionally a small handoff. Malcolm preserves the typed
requirement in `malcolm.ir/v1`; Sorna lowers it into each target rule's
provenance expectation and records the observed header in run evidence.
Amber remains responsible for the full provenance value, including UUID
validity, causation, attribution, transitions, and storage. Lockwood retains
the resulting evidence artifacts, and Nublar consumes the producer result
without reinterpreting the provenance semantics.

The current boundary checks only a non-empty `execution_id` field in the
unpadded-base64url JSON header. It does not create provenance, send an
incoming context, or validate the complete Amber v1 object. Richer fields,
request propagation, and cross-request relationships remain future work.

Use the checked-in example and acceptance target with:

~~~sh
make malcolm-sorna-provenance-contract
~~~

## Scenario isolation and fixture reset

A specification can require a subject-owned reset operation before each
independently executable scenario case:

~~~text
isolation per scenario {
  reset POST "/__malcolm/reset"
}
~~~

The reset endpoint belongs to the subject fixture, not to Malcolm's document
API. Malcolm preserves the typed declaration in `malcolm.ir/v1`; Sorna lowers
it into each rule's `given.isolation` data, invokes the reset before setup and
target execution, and records the reset request and response as isolation
evidence. A failed reset makes the case `inconclusive` and prevents the target
request from running.

This slice proves process-local state reset for the document-pipeline fixture.
It does not claim that an arbitrary external database, queue, or filesystem
was cleared, and Malcolm does not yet declare fixture file contents or cleanup
for subject-owned resources beyond the explicit reset request.

The stateful flow uses this hook and can be checked with:

~~~sh
make malcolm-sorna-flow-run
~~~

## Input fixture declarations

Malcolm can identify an oracle-owned input fixture without embedding or
guessing its bytes:

~~~text
fixture "welcome-document" {
  owner oracle
  purpose "canonical document input bytes"
  sha256 "637cecb53db658da5231f933fad366fce94f982501e59d7ddc7688cfaf81b825"
}
~~~

The fixture ID, owner, purpose, and lowercase SHA-256 digest are preserved in
`malcolm.ir/v1` and lowered to Sorna's contract-level `fixtures` list. The
digest pins the external bytes by identity; Sorna validates the shape and
sealing retains the digest. The current Malcolm declaration does not load a
file, copy fixture bytes, or infer a path, so request bodies must still use an
explicit typed `given body` block.

Only `owner oracle` is supported in Malcolm. Subject-owned implementation
fixtures belong to the subject/provider boundary and must not be represented as
oracle inputs.

The provider-side path handoff is separate from Malcolm source. Given a sealed
contract and a provider manifest, Sorna verifies each relative provider path's
bytes against the contract digest and writes a versioned handoff artifact:

~~~yaml
fixture_provider:
  schema: ingen.fixture-provider/v1
  id: document-fixtures
  version: 1
  fixtures:
    - id: welcome-document
      path: welcome-document.txt
      sha256: 637cecb53db658da5231f933fad366fce94f982501e59d7ddc7688cfaf81b825
~~~

The provider root defaults to the manifest's directory and can be supplied
explicitly with `--root`. Sorna records only the normalized relative path,
observed digest, byte count, provider identity, and sealed-contract identity;
it does not embed fixture bytes or rewrite the portable contract with a local
filesystem path. Use the checked-in example with:

~~~sh
make malcolm-sorna-fixture-handoff
~~~

The current handoff is file-based. An in-memory byte handoff and automatic
materialization into request bodies remain future work.

Use the checked-in example and acceptance target with:

~~~sh
make malcolm-sorna-fixture-contract
~~~

## Status

The first Rust slice now parses and validates executable request bodies and
stateful setup metadata, then emits them in `malcolm.ir/v1` JSON. Sorna's
adapter lowers this supported subset into `ingen.contract/v1`; the syntax, IR,
and compatibility rules are still experimental.

See [roadmap.md](roadmap.md) for the current implementation status,
limitations, and prioritized next steps.
