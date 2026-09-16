import { Provenance } from "./provenance.js";
import { processWorker } from "./worker.js";
import { MemoryStore } from "./storage.js";

function assert(condition: unknown, message: string): asserts condition {
  if (!condition) {
    throw new Error(message);
  }
}

const result = await processWorker(new MemoryStore(), Provenance.start());
assert(result.child.origin === "incoming", "worker child must be incoming");
assert(result.retry.origin === "retry", "worker retry must be marked retry");
assert(result.retry.work_id === result.child.work_id, "retry must preserve work ID");
assert(result.retry.causation?.id === result.child.execution_id, "retry must point to child execution");
assert(result.history.length === 2, "worker history must contain child and retry");
assert(result.history[0]?.execution_id === result.child.execution_id, "child must sort before retry");
assert(result.history[1]?.execution_id === result.retry.execution_id, "retry must sort after child");

console.log("TypeScript worker contract example passed");
