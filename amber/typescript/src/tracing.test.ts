import { mergeTraceAttributes, toTraceAttributes } from "./tracing.js";
import { Provenance } from "./provenance.js";

function assert(condition: unknown, message: string): asserts condition {
  if (!condition) {
    throw new Error(message);
  }
}

const root = Provenance.start({
  attribution: { initiated_by: { id: "user-42", type: "user" }, tenant_id: "tenant-7" },
  references: [{ type: "invoice", id: "invoice-123" }],
});
const attributes = toTraceAttributes(root);
assert(attributes["amber.work_id"] === root.work_id, "identity attributes are missing");
assert(attributes["amber.tenant_id"] === undefined, "tenant attribution must be opt-in");
assert(attributes["amber.reference_count"] === undefined, "typed references must be omitted by default");

const existing = { "span.kind": "consumer", "amber.work_id": "stale" };
const merged = mergeTraceAttributes(existing, root);
assert(existing["amber.work_id"] === "stale", "existing attributes were mutated");
assert(merged["amber.work_id"] === root.work_id, "Amber attributes did not override stale values");

const replay = root.replay();
const replayAttributes = toTraceAttributes(replay);
const replayMode = replay.mode;
assert(replayMode.kind === "replay" && replayAttributes["amber.mode.replay_id"] === replayMode.replay_id, "replay ID was not projected");

console.log("TypeScript tracing adapter tests passed");
