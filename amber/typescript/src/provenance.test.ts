import { Provenance } from "./provenance.js";
import { ProvenanceContext } from "./context.js";

function assert(condition: unknown, message: string): asserts condition {
  if (!condition) {
    throw new Error(message);
  }
}

const root = Provenance.start({
  attribution: {
    initiated_by: { id: "user-42", type: "user" },
    executed_by: { id: "worker", type: "service" },
    tenant_id: "tenant-7",
  },
  references: [{ type: "invoice", id: "invoice-123", relation: "subject" }],
});
const child = root.child();
const retry = root.retry();
const replay = root.replay();

assert(child.work_id !== root.work_id, "child must have a fresh work_id");
assert(child.correlation_id === root.correlation_id, "child must preserve correlation_id");
assert(child.depth === 1 && child.attempt === 1, "child depth/attempt is incorrect");
assert(retry.work_id === root.work_id, "retry must preserve work_id");
assert(retry.attempt === 2 && retry.mode.kind === "retry", "retry state is incorrect");
assert(replay.attempt === 1 && replay.mode.kind === "replay", "replay state is incorrect");

const json = JSON.stringify(root);
const roundTrip = Provenance.fromJSON(json);
assert(JSON.stringify(roundTrip) === json, "JSON round trip changed the value");

const withUnknown = {
  ...root.toJSON(),
  future_field: "accepted-and-ignored",
  mode: { ...root.mode, future_mode_field: true },
};
const unknownRoundTrip = Provenance.fromJSON(withUnknown);
assert(
  !JSON.stringify(unknownRoundTrip).includes("future_field") &&
    !JSON.stringify(unknownRoundTrip).includes("future_mode_field"),
  "unknown fields should be omitted by the non-lossless SDK",
);

let rejected = false;
try {
  Provenance.fromJSON({ version: 1, work_id: "bad" });
} catch {
  rejected = true;
}
assert(rejected, "invalid JSON value must be rejected");

const emptyContext = ProvenanceContext.empty();
const rootContext = emptyContext.withProvenance(root);
const childContext = rootContext.withProvenance(child);
assert(emptyContext.provenance === undefined, "empty context must remain empty");
assert(rootContext.provenance === root, "root context must retain its value");
assert(childContext.provenance === child, "child context must contain its value");

console.log("TypeScript provenance tests passed");
