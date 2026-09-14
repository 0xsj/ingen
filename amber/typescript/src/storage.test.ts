import {
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

console.log("TypeScript storage adapter tests passed");
