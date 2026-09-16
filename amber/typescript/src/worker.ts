import { Provenance } from "./provenance.js";
import type { ProvenanceStore } from "./storage.js";

export interface WorkerResult {
  child: Provenance;
  retry: Provenance;
  history: readonly Provenance[];
}

/** An application-owned background worker with explicit retry provenance. */
export async function processWorker(
  store: ProvenanceStore,
  parent: Provenance,
): Promise<WorkerResult> {
  const child = parent.child({ origin: "incoming" });
  await store.put(child);

  const retry = child.retry();
  await store.put(retry);

  const history = await store.listByWorkId(child.work_id);
  return { child, retry, history };
}
