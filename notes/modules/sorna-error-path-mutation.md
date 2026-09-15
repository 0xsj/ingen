# Sorna error-path mutation checkpoint

## What changed

The document-pipeline Go provider now supports a targeted error-path mutation:
`response.error.status.replace`. The first instance changes the contracted
`400` response for `unsupported_document_type` to `500`.

This is deliberately narrower than “change any error status.” The provider
must resolve the named error code to exactly one AST target, record the source
location and before/after values, and reject a missing or ambiguous target.

## Why it matters

Error paths are part of the public contract. A subject can pass every happy-path
test while returning the wrong class of failure to callers. The mutation should
be killed directly by the rule that asserts unsupported document types are
rejected with the contracted status; unrelated invalid-JSON behavior should
remain unaffected.

## Infrastructure finding

Each mutation variant runs as a fresh isolated subject. The campaign allocates
successive localhost ports, so adding the third mutation required extending the
fixture subject policy allowlist from `8080`–`8082` to include `8083`.

This makes the port-allocation policy an explicit campaign-scaling concern. A
future coordinator should either derive the approved range from the campaign
plan or allocate ports from a declared pool, rather than relying on an implicit
fixed count.

## Current boundary

This proves one error-path mutation end to end in the Go source-provider path.
It does not yet establish a general error taxonomy, transport-level mutation
operators, or independent attestation that a provider never inspected private
implementation details. Those remain separate design and verification work.
