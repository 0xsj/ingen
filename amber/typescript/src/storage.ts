import { ID, Provenance } from "./provenance.js";

export class StorageNotFoundError extends Error {
  constructor(executionId: ID) {
    super(`amber provenance not found: execution_id ${executionId}`);
    this.name = "StorageNotFoundError";
  }
}

export class StorageConflictError extends Error {
  constructor(executionId: ID) {
    super(`amber provenance storage conflict: execution_id ${executionId}`);
    this.name = "StorageConflictError";
  }
}

/** Persistence contract for immutable provenance values keyed by execution ID. */
export interface ProvenanceStore {
  put(provenance: Provenance): Promise<void>;
  get(executionId: ID): Promise<Provenance | undefined>;
}

/** Process-local reference store; it provides no durability guarantee. */
export class MemoryStore implements ProvenanceStore {
  private readonly records = new Map<ID, string>();

  async put(provenance: Provenance): Promise<void> {
    if (!(provenance instanceof Provenance)) {
      throw new TypeError("store value must be a Provenance instance");
    }
    provenance.validate();
    const serialized = JSON.stringify(provenance);
    const existing = this.records.get(provenance.execution_id);
    if (existing !== undefined) {
      if (existing === serialized) {
        return;
      }
      throw new StorageConflictError(provenance.execution_id);
    }
    this.records.set(provenance.execution_id, serialized);
  }

  async get(executionId: ID): Promise<Provenance | undefined> {
    const serialized = this.records.get(executionId);
    return serialized === undefined ? undefined : Provenance.fromJSON(serialized);
  }
}
