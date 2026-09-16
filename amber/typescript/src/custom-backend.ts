import type { KeyValueBackend } from "./storage.js";

/**
 * A copyable application-owned backend for the adapter-authoring example.
 * Replace this map with a database, cache, or service client while preserving
 * atomic putIfAbsent and complete namespace listing.
 */
export class ApplicationKeyValueBackend implements KeyValueBackend {
  private readonly values = new Map<string, string>();

  async get(key: string): Promise<string | undefined> {
    return this.values.get(key);
  }

  async putIfAbsent(key: string, value: string): Promise<boolean> {
    if (this.values.has(key)) {
      return false;
    }
    this.values.set(key, value);
    return true;
  }

  async list(prefix: string): Promise<readonly string[]> {
    return [...this.values.keys()].filter((key) => key.startsWith(prefix)).sort();
  }
}
