# Lockwood remote/object-storage semantics

Status: provider-neutral contract with a local reference/fetch verification
seam implemented. No provider client, credential, or automatic custody-import
integration is implemented.

The local seam is in [`internal/remote/remote.go`](internal/remote/remote.go).
It validates canonical references and verifies caller-supplied fetch results;
it does not publish blobs or custody records.

This document defines the boundary between Lockwood's local custody store and
an external object provider. It does not make a provider's URI, version, ETag,
or successful response a substitute for local content integrity.

## Responsibility and identity

The local artifact digest remains authoritative for the bytes Lockwood accepts:

```text
sha256:<64 lowercase hexadecimal characters>
```

Remote metadata identifies where a caller obtained those bytes. It does not
replace the local artifact digest and does not, by itself, authenticate the
provider or object owner.

The current custody v2 `source.uri` and `source.version` fields are
credential-free provenance metadata. They do not cause Lockwood to fetch,
cache, authenticate, or attest a remote object. Additional remote retrieval
metadata must use a versioned contract or a separate artifact; unknown fields
must not be added to the closed custody v2 record.

## Proposed remote reference

A provider-neutral remote reference may be represented separately as:

```json
{
  "schema": "lockwood.remote-object/v1",
  "uri": "https://objects.example/runs/run-0001/result.json",
  "version": "provider-version-0001",
  "etag": "\"strong-validator\"",
  "expected_digest": "sha256:<artifact-bytes>",
  "expected_size_bytes": 1234
}
```

Rules:

- `uri` is absolute and contains no credentials, access tokens, signed-query
  secrets, or mutable local filesystem path.
- `version` is an opaque provider locator. When supplied, it pins the provider
  version the caller intends to retrieve; it never becomes artifact identity.
- `etag` is an observed provider validator. A strong ETag may help pin a
  retrieval, but a weak or absent ETag cannot establish immutability.
- `expected_digest` is required before remote bytes can be accepted as trusted
  custody. A retrieval without an expected digest may be staged for caller
  inspection, but it must not silently become accepted remote evidence.
- `expected_size_bytes`, when supplied, is an additional pre-publication
  check. The measured byte count must match exactly.

Remote reference identity is the normalized tuple `(uri, version, etag)` only
when the provider declares those values stable for the requested object. The
artifact digest remains the identity of the retrieved bytes even when the
remote tuple changes or is absent.

## Caller-owned fetch boundary

Lockwood should consume a caller-owned fetcher with semantics equivalent to:

```text
Fetch(ctx, remote_reference) -> remote_fetch_result | remote_fetch_error
```

The result supplies a finite byte stream, the observed URI/version/ETag, and
the measured size when the provider exposes it. The fetcher owns:

- credentials, signing keys, tokens, and provider-specific authentication;
- endpoint selection, redirects, and provider API details;
- bounded retry and backoff policy;
- rate limits, connection pooling, and cancellation;
- provider-specific interpretation of version and ETag values.

The fetcher must not return credentials in the result or embed them in the
remote reference. A successful fetch only means that the caller's provider
adapter returned bytes; Lockwood still performs local digest and size checks.

## Remote acceptance sequence

A future remote-import path should be read-only with respect to the provider
and atomic with respect to local publication:

1. Validate the credential-free remote reference and required expected digest.
2. Ask the caller-owned fetcher for one finite byte stream.
3. Compare observed URI/version/ETag to the requested pin when one was given.
4. Stream the bytes into a local temporary object while computing SHA-256 and
   size; do not publish or create custody yet.
5. Reject on digest or size mismatch, provider-pin mismatch, truncation, or
   stream failure.
6. Atomically publish the verified local blob, then append the custody record
   with the remote source metadata.
7. If custody publication fails after blob publication, return the same
   recoverable pending record flow used by local intake.

No failed remote retrieval may create an accepted custody record. A successful
HTTP response, matching ETag, or provider version without a matching local
content digest is insufficient.

## Version and ETag semantics

- A provider version is preferred for immutable retrieval. Reusing a URI with a
  different version is a new remote observation and must be checked against its
  own expected digest.
- A strong ETag can detect an unexpected replacement when the provider's
  semantics guarantee that it represents the complete object. It is not a
  cryptographic substitute for the expected digest.
- Weak ETags, last-modified timestamps, and provider listing order are
  descriptive or cache validators only. They must not be used as artifact
  identity or lineage identity.
- If a provider returns a different version or ETag than requested, the fetch
  fails closed unless the caller explicitly requested an unpinned observation.
- The raw remote reference and observed metadata should be preserved in a
  separate receipt or versioned record if later audit needs to distinguish the
  requested pin from what the provider returned.

## Retry, cache, and failure behavior

The external fetcher decides whether a transport failure is retryable. It must
not retry authorization failures, digest mismatches, or pin mismatches as if
they were transient. Lockwood receives one verified result or a classified
error; it does not hide repeated provider failures behind an accepted record.

Any cache must be keyed by provider identity plus the requested remote tuple
and must retain the expected local digest. A cache hit still requires local
digest and size verification. A stale cache entry cannot be presented as a
fresh provider read without provider revalidation.

Minimum failure classes are:

- invalid or credential-bearing reference;
- provider authentication or authorization failure;
- object not found or requested version unavailable;
- transport timeout, cancellation, or bounded retry exhaustion;
- observed URI/version/ETag mismatch;
- digest mismatch or size mismatch;
- truncated, unreadable, or otherwise incomplete byte stream.

Each class must fail closed for trusted custody. The caller may retain a
diagnostic receipt outside the custody record, but Lockwood must not convert a
provider error into a successful local record.

## Lineage and custody relationship

Remote source metadata may explain how an accepted local artifact was obtained.
Lineage still uses local artifact digests and explicit custody relationships;
the remote URI, version, and ETag are not parent digests. A remote object may
participate in custody acceptance only after its bytes pass the same local
integrity and durability rules as locally supplied bytes.

Remote retrieval must not mutate an existing custody record, replace a blob at
an existing digest, or silently refresh a source URI. A new retrieval with
different bytes is a new artifact; a new remote observation of the same bytes
may receive a separate custody record if the workflow needs that history.

## Explicit non-goals

This contract does not define:

- a provider SDK or network client;
- credential storage, token exchange, or key management;
- hosted retention, remote deletion, lifecycle rules, or legal holds;
- provider-specific authorization or object-owner identity;
- eventual-consistency guarantees beyond the caller's fetch result;
- automatic remote synchronization or background refresh.

## Open decisions

- Should the remote reference and fetch receipt be a first-class v3 custody
  source shape or a separate detached artifact?
- Which providers, if any, need a provider namespace in the remote tuple?
- Is an unpinned URI-only observation allowed for any trusted workflow, or must
  every remote import carry an explicit version or strong ETag?
- Should the fetcher return a stream only, or also a provider-signed metadata
  assertion for independent audit?
- Which remote failures should be retained as diagnostic artifacts outside
  accepted custody?
