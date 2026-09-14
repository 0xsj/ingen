import { AmberTransitionError, Provenance } from "./provenance.js";

/**
 * An immutable context value for explicit provenance propagation. A derived
 * context leaves its parent untouched, which makes restoration exact and safe
 * across asynchronous boundaries when callers pass the intended context.
 */
export class ProvenanceContext {
  private constructor(private readonly value?: Provenance) {}

  static empty(): ProvenanceContext {
    return new ProvenanceContext();
  }

  get provenance(): Provenance | undefined {
    return this.value;
  }

  withProvenance(provenance: Provenance): ProvenanceContext {
    if (!(provenance instanceof Provenance)) {
      throw new AmberTransitionError("context value must be a Provenance instance");
    }
    provenance.validate();
    return new ProvenanceContext(provenance);
  }
}
