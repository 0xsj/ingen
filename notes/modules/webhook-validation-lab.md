# A webhook lab needs idempotency before delivery infrastructure

A useful webhook contract must make duplicate delivery behavior observable
before it introduces signatures, queues, retries, or external services.

## Origin

This note came from starting the first post-alpha Sorna example after the
document-pipeline checkpoint.

## What

The webhook-validation lab exposes one `POST /webhooks/events` boundary. It
accepts supported events, rejects missing identifiers and unsupported event
types, and remembers accepted event IDs so a repeated delivery returns an
explicit duplicate response.

## Why

Webhook systems are delivery boundaries, so retry and duplicate semantics are
part of behavior rather than implementation detail. A contract that only tests
one successful POST leaves the most important operational question unspecified:
what happens when the sender retries? The lab keeps that question concrete with
an in-memory store and a four-rule black-box oracle.

Authentication, signature verification, queueing, retry timing, and retention
are deliberately unspecified. Adding them before the contract needs them
would make the example look realistic while weakening its reviewability.

## Example

The duplicate rule first accepts `evt-duplicate` with `202`, then submits the
same event again and requires `200` with `status: duplicate` and
`duplicate: true`.

## Result

The first baseline run froze 4 oracle cases and passed all 4 without
inconclusive results. The measured identities were:

- contract: `ae042080df176ffd44e1a8fecf03cc9cbe9f4b729ab9ac908a064ca25072b5ea`;
- isolation policy: `494eaa1fb0d6818e2195918d3b453d7537ac5e81171ce4c31ca95c8f580d7fa5`;
- oracle: `e113da0ab83dce9c89a866ca7e594956692791ed2ed93bb52ccfb09be03099fe`;
- CI result: `b9cb091b1352545b8e24f0ccefc597e157b7c53ee7515462f94617f4b409c6b8`.

## Used in

The webhook-validation subject, its contract and policies, and the
`make webhook-alpha` baseline workflow.

## Related

- [`sorna/MUTATION-SPEC.md`](../../sorna/MUTATION-SPEC.md)
- [`document-pipeline-subject.md`](document-pipeline-subject.md)
