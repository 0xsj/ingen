import { Provenance } from "./provenance.js";
import { processWorker } from "./worker.js";
import { MemoryStore } from "./storage.js";

const result = await processWorker(new MemoryStore(), Provenance.start());
if (
  result.history.length !== 2 ||
  result.history[0]?.execution_id !== result.child.execution_id ||
  result.history[1]?.execution_id !== result.retry.execution_id
) {
  throw new Error("worker history was not ordered as child then retry");
}

console.log(`worker child execution: ${result.child.execution_id}`);
console.log(`worker retry execution: ${result.retry.execution_id}`);
console.log(`worker history records: ${result.history.length}`);
