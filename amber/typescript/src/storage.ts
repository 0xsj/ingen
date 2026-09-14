import { Causation, ID, Provenance } from "./provenance.js";

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
  listByWorkId(workId: ID): Promise<readonly Provenance[]>;
  listByCausation(kind: Causation["kind"], id: ID): Promise<readonly Provenance[]>;
  listByCorrelationId(correlationId: ID): Promise<readonly Provenance[]>;
}

/**
 * Minimal async backend for a durable or externally managed key-value store.
 * `putIfAbsent` must not replace an existing value, even under concurrent
 * callers; this preserves the append-only storage contract.
 */
export interface KeyValueBackend {
  get(key: string): Promise<string | undefined>;
  putIfAbsent(key: string, value: string): Promise<boolean>;
  list(prefix: string): Promise<readonly string[]>;
}

/** Small process-local backend useful for examples and adapter tests. */
export class MapKeyValueBackend implements KeyValueBackend {
  private readonly records = new Map<string, string>();

  async get(key: string): Promise<string | undefined> {
    return this.records.get(key);
  }

  async putIfAbsent(key: string, value: string): Promise<boolean> {
    if (this.records.has(key)) {
      return false;
    }
    this.records.set(key, value);
    return true;
  }

  async list(prefix: string): Promise<readonly string[]> {
    return [...this.records.keys()].filter((key) => key.startsWith(prefix)).sort();
  }
}

/** ProvenanceStore adapter for browser, Node, or service-provided key-value stores. */
export class KeyValueStore implements ProvenanceStore {
  constructor(
    private readonly backend: KeyValueBackend,
    private readonly namespace = "amber/provenance",
  ) {}

  async put(provenance: Provenance): Promise<void> {
    if (!(provenance instanceof Provenance)) {
      throw new TypeError("store value must be a Provenance instance");
    }
    provenance.validate();
    const serialized = JSON.stringify(provenance);
    const key = this.key(provenance.execution_id);
    if (await this.backend.putIfAbsent(key, serialized)) {
      return;
    }
    const existing = await this.backend.get(key);
    if (existing === serialized) {
      return;
    }
    throw new StorageConflictError(provenance.execution_id);
  }

  async get(executionId: ID): Promise<Provenance | undefined> {
    const serialized = await this.backend.get(this.key(executionId));
    return serialized === undefined ? undefined : Provenance.fromJSON(serialized);
  }

  async listByWorkId(workId: ID): Promise<readonly Provenance[]> {
    const prefix = `${this.namespace}/`;
    const records: Provenance[] = [];
    for (const key of await this.backend.list(prefix)) {
      const serialized = await this.backend.get(key);
      if (serialized === undefined) {
        continue;
      }
      const provenance = Provenance.fromJSON(serialized);
      if (provenance.work_id === workId) {
        records.push(provenance);
      }
    }
    return sortHistory(records);
  }

  async listByCausation(kind: Causation["kind"], id: ID): Promise<readonly Provenance[]> {
    const prefix = `${this.namespace}/`;
    const records: Provenance[] = [];
    for (const key of await this.backend.list(prefix)) {
      const serialized = await this.backend.get(key);
      if (serialized === undefined) {
        continue;
      }
      const provenance = Provenance.fromJSON(serialized);
      if (provenance.causation?.kind === kind && provenance.causation.id === id) {
        records.push(provenance);
      }
    }
    return sortHistory(records);
  }

  async listByCorrelationId(correlationId: ID): Promise<readonly Provenance[]> {
    const prefix = `${this.namespace}/`;
    const records: Provenance[] = [];
    for (const key of await this.backend.list(prefix)) {
      const serialized = await this.backend.get(key);
      if (serialized === undefined) {
        continue;
      }
      const provenance = Provenance.fromJSON(serialized);
      if (provenance.correlation_id === correlationId) {
        records.push(provenance);
      }
    }
    return sortHistory(records);
  }

  private key(executionId: ID): string {
    return `${this.namespace}/${executionId}`;
  }
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

  async listByWorkId(workId: ID): Promise<readonly Provenance[]> {
    const records: Provenance[] = [];
    for (const serialized of this.records.values()) {
      const provenance = Provenance.fromJSON(serialized);
      if (provenance.work_id === workId) {
        records.push(provenance);
      }
    }
    return sortHistory(records);
  }

  async listByCausation(kind: Causation["kind"], id: ID): Promise<readonly Provenance[]> {
    const records: Provenance[] = [];
    for (const serialized of this.records.values()) {
      const provenance = Provenance.fromJSON(serialized);
      if (provenance.causation?.kind === kind && provenance.causation.id === id) {
        records.push(provenance);
      }
    }
    return sortHistory(records);
  }

  async listByCorrelationId(correlationId: ID): Promise<readonly Provenance[]> {
    const records: Provenance[] = [];
    for (const serialized of this.records.values()) {
      const provenance = Provenance.fromJSON(serialized);
      if (provenance.correlation_id === correlationId) {
        records.push(provenance);
      }
    }
    return sortHistory(records);
  }
}

function sortHistory(records: Provenance[]): readonly Provenance[] {
  return records.sort((left, right) =>
    left.depth - right.depth ||
    left.attempt - right.attempt ||
    left.execution_id.localeCompare(right.execution_id),
  );
}
