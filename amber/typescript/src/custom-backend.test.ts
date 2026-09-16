import { Provenance, StorageConflictError } from "./index.js";
import { ApplicationKeyValueBackend } from "./custom-backend.js";
import { KeyValueStore } from "./storage.js";

function assert(condition: unknown, message: string): asserts condition {
  if (!condition) {
    throw new Error(message);
  }
}

const backend = new ApplicationKeyValueBackend();
const results = await Promise.all(
  Array.from({ length: 32 }, () => backend.putIfAbsent("race", "value")),
);
assert(results.filter(Boolean).length === 1, "putIfAbsent must insert exactly once");

const store = new KeyValueStore(backend, "contract/provenance");
const root = Provenance.start();
const retry = root.retry();
const child = root.child();
for (const value of [child, retry, root]) {
  await store.put(value);
}
await store.put(root);

const conflicting = Provenance.fromJSON({ ...root.toJSON(), origin: "incoming" });
let conflictProtected = false;
try {
  await store.put(conflicting);
} catch (error) {
  conflictProtected = error instanceof StorageConflictError;
}
assert(conflictProtected, "conflicting values must be rejected");

const history = await store.listByWorkId(root.work_id);
assert(history.length === 2, "work history must contain root and retry");
assert(history[0]?.execution_id === root.execution_id, "root must sort before retry");
assert(history[1]?.execution_id === retry.execution_id, "retry must sort after root");
assert((await store.listByCorrelationId(root.correlation_id)).length === 3, "correlation history must include all values");

console.log("TypeScript custom backend contract example passed");
