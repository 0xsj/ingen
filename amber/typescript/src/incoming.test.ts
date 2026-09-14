import { ProvenanceContext } from "./context.js";
import {
  MAX_INCOMING_JSON_BYTES,
  inspectIncomingJSON,
  withIncomingJSON,
} from "./incoming.js";
import { Provenance } from "./provenance.js";

function assert(condition: unknown, message: string): asserts condition {
  if (!condition) {
    throw new Error(message);
  }
}

const root = Provenance.start();
const input = JSON.stringify(root);
const accepted = inspectIncomingJSON(input, "reject");
assert(accepted.present && accepted.provenance?.execution_id === root.execution_id, "valid input was not accepted");

const ignored = inspectIncomingJSON('{"version":1}', "ignore");
assert(!ignored.present && ignored.provenance === undefined, "invalid ignored input must be absent");

let rejected = false;
try {
  inspectIncomingJSON('{"version":1}', "reject");
} catch {
  rejected = true;
}
assert(rejected, "invalid rejected input must throw");

const oversized = "x".repeat(MAX_INCOMING_JSON_BYTES + 1);
assert(!inspectIncomingJSON(oversized, "ignore").present, "oversized ignored input must be absent");
rejected = false;
try {
  inspectIncomingJSON(oversized, "reject");
} catch {
  rejected = true;
}
assert(rejected, "oversized rejected input must throw");

const empty = ProvenanceContext.empty();
const installed = withIncomingJSON(empty, input, "reject");
assert(installed.present && installed.context.provenance?.execution_id === root.execution_id, "incoming context was not installed");
const unchanged = withIncomingJSON(installed.context, "", "reject");
assert(!unchanged.present && unchanged.context === installed.context, "absent input must preserve context");

console.log("TypeScript incoming context tests passed");
