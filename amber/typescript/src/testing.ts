import { Provenance } from "./provenance.js";
import { StorageConflictError } from "./storage.js";
import type { ProvenanceStore } from "./storage.js";

function assert(condition: unknown, message: string): asserts condition {
  if (!condition) {
    throw new Error(message);
  }
}

/** Run the portable storage contract against an application-owned store. */
export async function runProvenanceStoreContract(store: ProvenanceStore): Promise<void> {
  const root = Provenance.start();
  const retry = root.retry();
  const child = root.child({ origin: "incoming" });

  await expectRejected(
    () => store.put({} as Provenance),
    (error) => error instanceof TypeError,
    "store must reject non-Provenance values",
  );
  for (const value of [child, retry, root]) {
    await store.put(value);
  }
  await store.put(root);

  const conflicting = Provenance.fromJSON({ ...root.toJSON(), origin: "incoming" });
  await expectRejected(
    () => store.put(conflicting),
    (error) => error instanceof StorageConflictError,
    "store must reject conflicting values",
  );
  const stored = await store.get(root.execution_id);
  assert(stored?.origin === root.origin, "conflict changed the stored value");

  assertHistory(
    await store.listByWorkId(root.work_id),
    root.execution_id,
    retry.execution_id,
  );
  assertHistory(
    await store.listByCausation("execution", root.execution_id),
    retry.execution_id,
    child.execution_id,
  );
  assertHistory(
    await store.listByCorrelationId(root.correlation_id),
    root.execution_id,
    retry.execution_id,
    child.execution_id,
  );

  const missingId = "99999999-9999-4999-8999-999999999999";
  assert((await store.get(missingId)) === undefined, "missing lookup must be undefined");
  assert((await store.listByWorkId(missingId)).length === 0, "missing history must be empty");
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

function assertHistory(history: readonly Provenance[], ...expectedIds: string[]): void {
  assert(history.length === expectedIds.length, `history length=${history.length}`);
  for (const [index, expectedId] of expectedIds.entries()) {
    assert(history[index]?.execution_id === expectedId, `history[${index}] was not deterministic`);
  }
}
