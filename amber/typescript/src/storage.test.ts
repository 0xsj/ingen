import {
  KeyValueStore,
  MapKeyValueBackend,
  MemoryStore,
  StorageConflictError,
} from "./storage.js";
import { Provenance } from "./provenance.js";

function assert(condition: unknown, message: string): asserts condition {
  if (!condition) {
    throw new Error(message);
  }
}

const store = new MemoryStore();
const root = Provenance.start();
await store.put(root);
await store.put(root);

const stored = await store.get(root.execution_id);
assert(stored?.execution_id === root.execution_id, "stored value could not be read");
assert((await store.get("11111111-1111-4111-8111-111111111111")) === undefined, "missing value must be absent");

const conflicting = Provenance.fromJSON({ ...root.toJSON(), origin: "incoming" });
let rejected = false;
try {
  await store.put(conflicting);
} catch (error) {
  rejected = error instanceof StorageConflictError;
}
assert(rejected, "same execution_id with different data must conflict");

const backend = new MapKeyValueBackend();
const durable = new KeyValueStore(backend, "test/amber");
await durable.put(root);
const reopened = new KeyValueStore(backend, "test/amber");
const reopenedValue = await reopened.get(root.execution_id);
assert(reopenedValue?.execution_id === root.execution_id, "key-value store did not persist across instances");
const history = await reopened.listByWorkId(root.work_id);
assert(history.length === 1 && history[0].execution_id === root.execution_id, "key-value store returned wrong history");
await reopened.put(root);
const retry = root.retry();
await reopened.put(retry);
const causal = await reopened.listByCausation("execution", root.execution_id);
assert(causal.length === 1 && causal[0].execution_id === retry.execution_id, "key-value store returned wrong causal history");
const correlated = await reopened.listByCorrelationId(root.correlation_id);
assert(correlated.length === 2, "key-value store returned wrong correlation history");

let durableRejected = false;
try {
  await reopened.put(conflicting);
} catch (error) {
  durableRejected = error instanceof StorageConflictError;
}
assert(durableRejected, "key-value store must preserve append-only conflicts");

console.log("TypeScript storage adapter tests passed");
