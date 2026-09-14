import { ProvenanceContext } from "./context.js";
import {
  AMBER_PROVENANCE_HEADER,
  MAX_ENCODED_HEADER_BYTES,
  decodeHeader,
  encodeHeader,
  setOutgoingHeader,
  withIncomingHeaders,
  withIncomingRequest,
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
let rejected = false;
try {
  decodeHeader("not-base64", "reject");
} catch {
  rejected = true;
}
assert(rejected, "invalid rejected header must throw");

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

console.log("TypeScript HTTP adapter tests passed");
