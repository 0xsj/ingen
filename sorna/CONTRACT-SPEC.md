# Sorna Contract Specification

Status: Draft design specification

This document defines the contract artifact consumed by Sorna. A contract is
the human-reviewable statement of externally observable behavior that an
implementation is expected to satisfy.

The contract is the authority for oracle generation. It must not be inferred
from the implementation under test.

## 1. Scope

The first contract format targets a frontend/backend boundary exposed through
HTTP and JSON, but the model is intended to support other public interfaces
later, including command-line interfaces and browser interactions.

A contract describes:

- public subjects and operations;
- valid and invalid inputs;
- observable outputs and errors;
- state transitions and invariants;
- explicitly permitted variation;
- intentionally unspecified behavior;
- fixtures and environment assumptions.

A contract does not describe private classes, internal call graphs, preferred
implementation patterns, or tests derived from source code.

## 2. Normative language

Contract text uses these strengths:

- `must`: a required behavior; violation is a contract failure.
- `must_not`: a prohibited behavior; occurrence is a contract failure.
- `may`: an allowed variation; the oracle must accept it.
- `should`: a recommendation that may be reported separately from a failure.
- `unspecified`: deliberately outside the contract; the oracle must not create
  an expectation for it.

The machine-readable field is `strength`. Natural-language descriptions may
explain a rule but must not contradict its strength.

## 3. Top-level shape

The canonical representation is YAML or JSON. Serialization rules must be
stable so the sealed contract can be hashed.

The structural cross-language schema is
[`spec/ingen.contract-v1.schema.json`](spec/ingen.contract-v1.schema.json).
It fixes the outer shape and `ingen.contract/v1` identity; Sorna's runtime
validator remains authoritative for semantic rules such as duplicate IDs,
stateful setup completeness, generated-value limits, and fixture hashes.

Illustrative contract:

```yaml
contract:
  schema: ingen.contract/v1
  id: todo-api
  version: 1
  status: draft
  title: Todo HTTP boundary
  owner: product-team
  interface:
    kind: http-json
    base_url: http://sut.invalid
    public_entrypoints:
      - method: POST
        path: /api/todos
      - method: GET
        path: /api/todos

  vocabulary:
    todo_id: non-empty string
    title: non-empty string, maximum 200 characters

  rules:
    - id: todo.create.accepts-valid
      strength: must
      subject: POST /api/todos
      given:
        body:
          title: Read the contract
      expect:
        status: 201
        body:
          type: object
          required: [id, title, completed]
          properties:
            id: { type: string, non_empty: true }
            title: { equals: Read the contract }
            completed: { type: boolean, equals: false }

    - id: todo.create.rejects-empty-title
      strength: must
      subject: POST /api/todos
      given:
        body: { title: "" }
      expect:
        status: 400
        error:
          code: invalid_title

    - id: todo.list-includes-created
      strength: must
      subject: GET /api/todos
      invariant: A successfully created todo is observable in a later list.

  unspecified:
    - generated id format
    - created_at field format
    - ordering of unrelated todos

  fixtures:
    - path: fixtures/empty-database.json
      purpose: known initial state
```

## 4. Required fields

Every contract must contain:

- `schema`: contract schema identifier and version;
- `id`: stable logical identity;
- `version`: positive integer or equivalent immutable version label;
- `status`: `draft`, `sealed`, or `superseded`;
- `interface`: public boundary and adapter information;
- `rules`: normative and advisory behavior;
- `unspecified`: known areas where the oracle must not over-assert.

The following fields are recommended:

- `title` and `summary`;
- `owner` and reviewers;
- `vocabulary` for shared terms and constraints;
- `fixtures` with hashes and purposes;
- `assumptions` about environment and state;
- `amendments` linking later versions;
- `tags` for coverage and reporting.

## 5. Rule structure

Each rule has a stable `id` and should have these fields:

```yaml
- id: account.create.rejects-duplicate-email
  strength: must
  subject: POST /accounts
  tags: [validation, uniqueness]
  given:
    existing:
      - account: { email: user@example.test }
  when:
    body:
      email: user@example.test
      name: Another User
  expect:
    status: 409
    error:
      code: email_already_exists
  rationale: A user identity must not be silently duplicated.
```

Rules may express:

- exact values;
- types and required fields;
- ranges, lengths, and enums;
- regular expressions where the format is normative;
- relationships between fields;
- response headers and status;
- error category and public message policy;
- state transitions;
- properties over generated values;
- metamorphic relationships between executions.

For an object response shape, `additional_properties: false` closes the shape:
every returned key must appear in `required` or `properties`. When omitted,
the shape is open and extra keys are accepted. This must be written
deliberately; required-field checks alone do not prohibit undocumented output.

Rules must identify observable subjects. A rule such as “uses a repository
transaction” is not a contract rule unless the transaction is externally
observable through a supported interface.

For the HTTP/JSON adapter, a domain event may be represented by an explicit
response signal such as:

~~~yaml
expect:
  events:
    required: [document.accepted]
~~~

The signal and its transport must be part of the public subject contract.
Sorna lifecycle events and host-access telemetry are verifier-owned evidence;
they do not prove that the subject emitted a domain event.

## 6. Inputs and boundaries

The contract should make meaningful boundaries explicit. For a string with a
maximum length of 200, the contract should state whether the following are
valid or invalid:

- empty string;
- whitespace-only string;
- one-character string;
- exactly 200 characters;
- 201 characters;
- non-ASCII input;
- malformed encoding, if relevant.

If the contract does not define a boundary, Sorna may report it as
`inconclusive`, but must not invent a requirement from common convention.

## 7. Errors

Errors should be specified at the most stable public level available:

- status code;
- public error code;
- field-level error locations;
- retryability or recovery hint;
- whether state was changed.

Exact prose should be avoided unless wording itself is a product requirement.
The contract should normally specify an error code and permitted message
variation instead.

## 8. State and invariants

State rules describe observable transitions, not private storage:

```yaml
- id: todo.delete-is-observable
  strength: must
  subject: DELETE /api/todos/{id}
  given:
    state: todo_exists
  expect:
    status: 204
    subsequent:
      - request: GET /api/todos/{id}
        expect: { status: 404 }
```

State setup must be reproducible. A fixture or public setup operation should
be named explicitly. Hidden database manipulation is not an oracle input
unless the experiment declares that the database itself is the public system
boundary.

## 9. Unspecified behavior

Unspecified behavior is a first-class part of the contract. It prevents the
oracle from turning accidental implementation details into guarantees.

Examples include:

- generated identifiers;
- timestamps unless precision is normative;
- ordering where the contract only promises membership;
- internal error messages;
- database and framework choice;
- latency unless a bound is explicitly required.

The contract may define an allowed set or predicate when behavior is variable.
It should use `unspecified` only when the behavior is genuinely irrelevant to
the product requirement.

## 10. Fixtures and public examples

Fixtures are contract inputs only when they are explicitly included, named,
and hashed. Each fixture needs:

- stable path or logical identifier;
- purpose;
- format and schema version;
- SHA-256 digest at seal time;
- whether it is normative or illustrative.

An implementation-generated snapshot is not a valid contract fixture merely
because it is convenient. It must be reviewed and promoted into the contract
as an independent artifact first.

## 11. Sealing and versioning

The lifecycle is:

```text
draft -> review -> sealed -> superseded
```

Before sealing, contract authors may edit rules. After sealing:

- the serialized contract is canonicalized and hashed;
- fixtures are hashed;
- reviewers and timestamp are recorded;
- an oracle may be generated against that exact version;
- changes require a new version or an explicitly recorded amendment.

An amendment must state whether it is:

- clarifying, with no intended behavior change;
- additive, adding a new requirement;
- restrictive, narrowing allowed behavior;
- corrective, changing an incorrect requirement;
- breaking, invalidating prior compatibility.

Prior run results remain attached to the old contract version.

## 12. Validation rules

The contract validator should reject:

- duplicate rule IDs;
- missing subjects for executable rules;
- contradictory expectations within one rule;
- references to undeclared fixtures;
- invalid strength values;
- unbounded generated inputs without a seed or limit;
- a sealed contract with missing hash metadata;
- a rule that asserts a private implementation detail.

Warnings should flag:

- rules with no negative or boundary case where one appears relevant;
- large snapshot expectations;
- unspecified fields that are later asserted by an oracle;
- rules that cannot be mapped to the selected adapter;
- duplicate or overlapping rules.

## 13. Contract quality checklist

Before sealing, reviewers should confirm:

- Every required behavior has a stable rule ID.
- Public subjects and input boundaries are named.
- Error behavior is specified at a stable level.
- State transitions have reproducible setup.
- Accidental implementation details are listed as unspecified.
- Examples include both valid and invalid behavior where relevant.
- The contract can be evaluated without implementation source access.
- Each rule can be traced to a product or user requirement.
- The contract does not encode the implementation's existing behavior merely
  because it is already present.

## 14. MVP subset

The first Sorna implementation should support:

- YAML and JSON serialization;
- HTTP/JSON subjects;
- `must`, `must_not`, `may`, and `unspecified`;
- status, headers, JSON shape, types, enums, ranges, and presence;
- fixed fixtures with hashes;
- stateful sequences of public requests;
- draft/sealed/superseded status;
- canonicalization and SHA-256 hashing.

Property expressions, richer state models, and multiple adapters can build on
this subset without changing the core contract identity or version rules.
