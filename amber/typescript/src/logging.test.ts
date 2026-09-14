import { mergeLogFields, toLogFields } from "./logging.js";
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
const fields = toLogFields(root);
assert(fields["amber.work_id"] === root.work_id, "identity fields are missing");
assert(fields["amber.tenant_id"] === undefined, "tenant attribution must be opt-in");
assert(fields["amber.reference_count"] === undefined, "typed references must be omitted by default");

const existing = { message: "created", "amber.work_id": "stale" };
const merged = mergeLogFields(existing, root);
assert(existing["amber.work_id"] === "stale", "existing fields were mutated");
assert(merged["amber.work_id"] === root.work_id, "Amber fields did not override stale values");

const retry = root.retry();
const retryFields = toLogFields(retry);
assert(retryFields["amber.mode.of_execution_id"] === root.execution_id, "retry source was not projected");

console.log("TypeScript logging adapter tests passed");
