import { ProvenanceContext } from "./context.js";
import {
  AMBER_PROVENANCE_HEADER,
  MAX_ENCODED_HEADER_BYTES,
  decodeHeader,
  decodeHeaderWithValidator,
  encodeHeader,
  httpMiddleware,
  httpMiddlewareWithValidator,
  setOutgoingHeader,
  withIncomingHeaders,
  withIncomingRequest,
  withIncomingRequestWithValidator,
  withOutgoingRequest,
} from "./http.js";
import { Provenance } from "./provenance.js";

function assert(condition: unknown, message: string): asserts condition {
  if (!condition) {
    throw new Error(message);
  }
}

const root = Provenance.start();
const encoded = encodeHeader(root);
assert(!/[{}"]/.test(encoded), "HTTP header must use base64url encoding");
const decoded = decodeHeader(encoded, "reject");
assert(decoded.present && decoded.provenance?.execution_id === root.execution_id, "header round trip failed");

const headers = new Headers([[AMBER_PROVENANCE_HEADER, encoded]]);
const empty = ProvenanceContext.empty();
const installed = withIncomingHeaders(empty, headers, "reject");
assert(installed.present && installed.context.provenance?.execution_id === root.execution_id, "incoming header was not installed");

const absent = withIncomingHeaders(installed.context, new Headers(), "reject");
assert(!absent.present && absent.context === installed.context, "absent header must preserve context");

assert(!decodeHeader("not-base64", "ignore").present, "invalid ignored header must be absent");
let rejectedHeader = false;
try {
  decodeHeader("not-base64", "reject");
} catch {
  rejectedHeader = true;
}
assert(rejectedHeader, "invalid rejected header must throw");

const oversized = "A".repeat(MAX_ENCODED_HEADER_BYTES + 1);
assert(!decodeHeader(oversized, "ignore").present, "oversized ignored header must be absent");

const source = new Request("https://example.test");
const outgoing = withOutgoingRequest(source, root);
assert(source.headers.get(AMBER_PROVENANCE_HEADER) === null, "source request was mutated");
assert(outgoing.headers.get(AMBER_PROVENANCE_HEADER) === encoded, "outgoing request has wrong header");
const incoming = withIncomingRequest(outgoing, empty, "reject");
assert(incoming.present && incoming.request === outgoing, "incoming request handling failed");

const mutableHeaders = new Headers();
setOutgoingHeader(mutableHeaders, root);
assert(mutableHeaders.get(AMBER_PROVENANCE_HEADER) === encoded, "outgoing header was not set");

let called = false;
const wrapped = httpMiddleware(
  (request, context) => {
    called = true;
    assert(request === outgoing, "middleware should preserve the incoming request object");
    assert(context.provenance?.execution_id === root.execution_id, "middleware context is incorrect");
    return new Response("ok", { status: 201 });
  },
  "reject",
);
const response = await wrapped(outgoing, empty);
assert(called && response.status === 201, "middleware did not call the next handler");
assert(response.headers.get(AMBER_PROVENANCE_HEADER) === encoded, "middleware did not propagate the response header");
assert((await response.text()) === "ok", "middleware changed the response body");

let rejectedCalled = false;
const rejected = await httpMiddleware(
  () => {
    rejectedCalled = true;
    return new Response("unexpected");
  },
  "reject",
)(new Request("https://example.test", { headers: [[AMBER_PROVENANCE_HEADER, "not-base64"]] }), empty);
assert(!rejectedCalled && rejected.status === 400, "rejected input should stop the handler chain");
assert(rejected.headers.get(AMBER_PROVENANCE_HEADER) === null, "rejected input returned an Amber header");

let ignoredCalled = false;
const ignored = await httpMiddleware(
  (_request, context) => {
    ignoredCalled = true;
    assert(context.provenance === undefined, "ignored input should not install provenance");
    return new Response(null, { status: 204 });
  },
  "ignore",
)(new Request("https://example.test", { headers: [[AMBER_PROVENANCE_HEADER, "not-base64"]] }), empty);
assert(ignoredCalled && ignored.status === 204, "ignored input should continue the handler chain");
assert(ignored.headers.get(AMBER_PROVENANCE_HEADER) === null, "ignored input returned an Amber header");

const validator = (provenance: Provenance): void => {
  if (provenance.execution_id !== root.execution_id) {
    throw new Error("execution is not trusted");
  }
};
const verifiedHeader = decodeHeaderWithValidator(encoded, "reject", validator);
assert(verifiedHeader.present, "trusted header was rejected");
const untrusted = Provenance.start();
const untrustedEncoded = encodeHeader(untrusted);
assert(!decodeHeaderWithValidator(untrustedEncoded, "ignore", validator).present, "ignored untrusted header must be absent");
const verifiedRequest = withIncomingRequestWithValidator(outgoing, empty, "reject", validator);
assert(verifiedRequest.present, "trusted request was rejected");

let untrustedCalled = false;
const untrustedResponse = await httpMiddlewareWithValidator(
  () => {
    untrustedCalled = true;
    return new Response("unexpected");
  },
  "reject",
  validator,
)(new Request("https://example.test", { headers: [[AMBER_PROVENANCE_HEADER, untrustedEncoded]] }), empty);
assert(!untrustedCalled && untrustedResponse.status === 400, "untrusted header reached handler");

console.log("TypeScript HTTP adapter tests passed");
