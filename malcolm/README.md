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
    change response.status to 200
    expect contract to fail
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
~~~

Sorna's HTTP runner observes this through the `X-InGen-Event` response header.

## Status

The first Rust slice now parses and validates executable request bodies and
stateful setup metadata, then emits them in `malcolm.ir/v1` JSON. Sorna's
adapter lowers this supported subset into `ingen.contract/v1`; the syntax, IR,
and compatibility rules are still experimental.
