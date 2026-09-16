import { Provenance, StorageConflictError } from "./index.js";
import { ApplicationKeyValueBackend } from "./custom-backend.js";
import { KeyValueStore } from "./storage.js";

const backend = new ApplicationKeyValueBackend();
const store = new KeyValueStore(backend, "orders/provenance");
const root = Provenance.start();
const retry = root.retry();
const child = root.child({ origin: "incoming" });

// Insertion order is deliberately different from semantic history order.
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
if (!conflictProtected) {
  throw new Error("conflicting value was accepted");
}

const history = await store.listByWorkId(root.work_id);
if (history.length !== 2 || history[0]?.execution_id !== root.execution_id || history[1]?.execution_id !== retry.execution_id) {
  throw new Error("work history was not deterministic");
}
const correlated = await store.listByCorrelationId(root.correlation_id);
if (correlated.length !== 3) {
  throw new Error(`correlation history length=${correlated.length}, want 3`);
}

console.log("custom backend idempotency: true");
console.log("custom backend conflict protection: true");
console.log(`custom backend correlation records: ${correlated.length}`);
