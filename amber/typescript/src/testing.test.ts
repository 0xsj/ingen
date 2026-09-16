import { runProvenanceStoreContract } from "./testing.js";
import { KeyValueStore, MapKeyValueBackend, MemoryStore } from "./storage.js";

await runProvenanceStoreContract(new MemoryStore());
await runProvenanceStoreContract(new KeyValueStore(new MapKeyValueBackend(), "contract/testing"));

console.log("TypeScript reusable storage contract helper passed");
