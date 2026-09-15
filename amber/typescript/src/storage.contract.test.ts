import {
  KeyValueStore,
  MapKeyValueBackend,
  MemoryStore,
  StorageConflictError,
} from "./storage.js";
import type { ProvenanceStore } from "./storage.js";
import { Provenance } from "./provenance.js";

function assert(condition: unknown, message: string): asserts condition {
  if (!condition) {
    throw new Error(message);
  }
}

async function expectRejected(
  action: () => Promise<void>,
  predicate: (error: unknown) => boolean,
  message: string,
): Promise<void> {
  try {
    await action();
  } catch (error) {
    if (!predicate(error)) {
      throw new Error(`${message}: unexpected error ${String(error)}`);
    }
    return;
  }
  throw new Error(`${message}: operation was accepted`);
}

async function runStoreContract(name: string, store: ProvenanceStore): Promise<void> {
  const root = Provenance.start();
  const retry = root.retry();
  const child = root.child({ origin: "incoming" });

  await expectRejected(
    () => store.put({} as Provenance),
    (error) => error instanceof TypeError,
    `${name} must reject non-Provenance values`,
  );

  // Insert out of semantic order so implementations cannot accidentally pass
  // by returning insertion order from history queries.
  for (const value of [child, retry, root]) {
    await store.put(value);
  }
  await store.put(root);

  const stored = await store.get(root.execution_id);
  assert(stored?.execution_id === root.execution_id, `${name} could not read root`);

  const conflicting = Provenance.fromJSON({ ...root.toJSON(), origin: "incoming" });
  await expectRejected(
    () => store.put(conflicting),
    (error) => error instanceof StorageConflictError,
    `${name} must reject conflicting immutable values`,
  );
  const preserved = await store.get(root.execution_id);
  assert(preserved?.origin === root.origin, `${name} allowed a conflict to overwrite root`);

  const workHistory = await store.listByWorkId(root.work_id);
  assertHistory(name, "work", workHistory, root.execution_id, retry.execution_id);

  const causalHistory = await store.listByCausation("execution", root.execution_id);
  assertHistory(name, "causation", causalHistory, retry.execution_id, child.execution_id);

  const correlationHistory = await store.listByCorrelationId(root.correlation_id);
  assertHistory(
    name,
    "correlation",
    correlationHistory,
    root.execution_id,
    retry.execution_id,
    child.execution_id,
  );

  const missingId = "99999999-9999-4999-8999-999999999999";
  const missing = await store.get(missingId);
  assert(missing === undefined, `${name} must distinguish a missing lookup`);
  assert(
    (await store.listByWorkId(missingId)).length === 0,
    `${name} must return an empty history for an unknown work ID`,
  );
}

async function runConcurrentStoreCheck(name: string, store: ProvenanceStore): Promise<void> {
  const root = Provenance.start();
  const values = Array.from({ length: 24 }, () => root.child());
  await Promise.all(values.flatMap((value) => [store.put(value), store.put(value)]));

  await Promise.all(
    Array.from({ length: 8 }, async () => {
      for (let index = 0; index < 32; index += 1) {
        await store.listByCorrelationId(root.correlation_id);
      }
    }),
  );

  const history = await store.listByCorrelationId(root.correlation_id);
  assert(
    history.length === values.length,
    `${name} concurrent writes stored ${history.length} records, want ${values.length}`,
  );
}

function assertHistory(
  name: string,
  query: string,
  history: readonly Provenance[],
  ...expectedIds: string[]
): void {
  assert(
    history.length === expectedIds.length,
    `${name} ${query} history length=${history.length}, want ${expectedIds.length}`,
  );
  for (const [index, expectedId] of expectedIds.entries()) {
    assert(
      history[index]?.execution_id === expectedId,
      `${name} ${query} history[${index}] is ${history[index]?.execution_id}, want ${expectedId}`,
    );
  }
}

await runStoreContract("memory", new MemoryStore());
await runStoreContract("key-value", new KeyValueStore(new MapKeyValueBackend(), "contract/provenance"));
await runConcurrentStoreCheck("memory", new MemoryStore());
await runConcurrentStoreCheck("key-value", new KeyValueStore(new MapKeyValueBackend(), "concurrent/provenance"));

console.log("TypeScript storage contract tests passed");
