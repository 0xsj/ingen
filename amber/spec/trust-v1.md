# Amber v1 trust boundary

Amber separates structural validity from authenticity. A value can be valid
Amber data and still be supplied by an untrusted sender.

## Default contract

The v1 HTTP and messaging envelopes carry unsigned canonical provenance JSON.
Decoding and validation establish only that the value conforms to the Amber v1
shape and limits. They do not establish who sent it, whether it was authorized,
or whether it is fresh.

Applications MUST NOT use `work_id`, `execution_id`, `correlation_id`, actor
fields, tenant fields, or any other Amber value as an authorization credential
without an independent trust decision.

## Validator seam

Both SDKs expose an optional `IncomingValidator` hook. The hook runs after a
present value has passed structural decoding and before it is installed in a
context. A validator can enforce an application allowlist or connect to a
deployment-specific trust decision. Existing callers that do not provide one
retain the unsigned v1 behavior.

Validator failures follow the incoming policy:

- `reject` returns an error and does not invoke the handler;
- `ignore` treats the value as absent and continues without installing it.

The validator is not an authorization system. Handlers still need their normal
authentication and authorization checks.

## Authenticated extensions

Amber v1 does not define a signature algorithm, signature field, key ID, key
distribution mechanism, rotation protocol, timestamp, nonce, or replay cache.
An application that needs cryptographic authenticity must authenticate the raw
transport envelope with its own protocol and then apply an Amber validator, or
define a separately versioned authenticated adapter contract. It MUST NOT infer
authenticity from a structurally valid unsigned value.

## Operational guidance

- Treat incoming attribution and references as untrusted descriptive data until
  independently verified.
- Prefer `reject` when provenance is required for a trusted workflow and
  `ignore` when provenance is optional metadata.
- Do not log secrets or signature material through the default projections.
- Keep trust decisions at the boundary so downstream context consumers receive
  only accepted values.
