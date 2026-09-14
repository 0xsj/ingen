export const VERSION = 1;
export const MAX_REFERENCES = 32;
export const MAX_STRING_LENGTH = 256;

export type ID = string;

export type Origin = "local" | "incoming" | "retry" | "replay";

export type Mode =
  | { readonly kind: "normal" }
  | { readonly kind: "retry"; readonly of_execution_id: ID }
  | {
      readonly kind: "replay";
      readonly of_execution_id: ID;
      readonly replay_id: ID;
    };

export type Causation = {
  readonly kind: "execution" | "event" | "message" | "request" | "artifact";
  readonly id: ID;
  readonly type?: string;
};

export type Actor = {
  readonly id: string;
  readonly type: string;
};

export type Attribution = {
  readonly initiated_by?: Actor;
  readonly executed_by?: Actor;
  readonly on_behalf_of?: Actor;
  readonly tenant_id?: string;
};

export type Reference = {
  readonly type: string;
  readonly id: string;
  readonly relation?: string;
};

export type IdFactory = () => ID;

export type StartOptions = {
  readonly attribution?: Attribution;
  readonly references?: readonly Reference[];
  readonly idFactory?: IdFactory;
};

export type ChildOptions = {
  readonly cause?: Causation;
  readonly attribution?: Attribution;
  readonly origin?: "local" | "incoming";
  readonly references?: readonly Reference[];
};

export type WireProvenance = {
  readonly version: number;
  readonly work_id: ID;
  readonly execution_id: ID;
  readonly correlation_id: ID;
  readonly causation?: Causation;
  readonly origin: Origin;
  readonly depth: number;
  readonly attempt: number;
  readonly attribution?: Attribution;
  readonly mode: Mode;
  readonly references?: readonly Reference[];
};

export class AmberValidationError extends Error {
  constructor(message: string) {
    super(`invalid amber provenance: ${message}`);
    this.name = "AmberValidationError";
  }
}

export class AmberTransitionError extends Error {
  constructor(message: string) {
    super(`invalid amber transition: ${message}`);
    this.name = "AmberTransitionError";
  }
}

type State = {
  readonly version: number;
  readonly work_id: ID;
  readonly execution_id: ID;
  readonly correlation_id: ID;
  readonly causation?: Causation;
  readonly origin: Origin;
  readonly depth: number;
  readonly attempt: number;
  readonly attribution?: Attribution;
  readonly mode: Mode;
  readonly references: readonly Reference[];
};

type CryptoLike = {
  readonly randomUUID?: () => string;
  readonly getRandomValues?: (array: Uint8Array) => Uint8Array;
};

/** Generate a UUIDv4 using the host's Web Crypto implementation. */
export function defaultIdFactory(): ID {
  const cryptoSource = (globalThis as { crypto?: CryptoLike }).crypto;
  if (cryptoSource?.randomUUID) {
    return cryptoSource.randomUUID();
  }
  if (!cryptoSource?.getRandomValues) {
    throw new Error("Amber requires a Web Crypto implementation to generate IDs");
  }

  const bytes = new Uint8Array(16);
  cryptoSource.getRandomValues(bytes);
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const hex = Array.from(bytes, (value) => value.toString(16).padStart(2, "0")).join("");
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

/**
 * An immutable application-level provenance value. Transitions return new
 * values and never mutate the receiver.
 */
export class Provenance {
  private constructor(
    private readonly state: State,
    private readonly idFactory: IdFactory,
  ) {}

  /** Create a new logical unit of work. */
  static start(options: StartOptions = {}): Provenance {
    const idFactory = options.idFactory ?? defaultIdFactory;
    return Provenance.create(
      {
        version: VERSION,
        work_id: idFactory(),
        execution_id: idFactory(),
        correlation_id: idFactory(),
        origin: "local",
        depth: 0,
        attempt: 1,
        mode: { kind: "normal" },
        attribution: cloneAttribution(options.attribution),
        references: cloneReferences(options.references ?? []),
      },
      idFactory,
    );
  }

  /** Create a new logical unit of work derived from this value. */
  child(options: ChildOptions = {}): Provenance {
    this.validate();
    const origin = options.origin ?? "local";
    const cause = cloneCausation(
      options.cause ?? { kind: "execution", id: this.execution_id },
    );
    return Provenance.create(
      {
        version: VERSION,
        work_id: this.idFactory(),
        execution_id: this.idFactory(),
        correlation_id: this.correlation_id,
        causation: cause,
        origin,
        depth: this.depth + 1,
        attempt: 1,
        attribution:
          options.attribution === undefined
            ? cloneAttribution(this.state.attribution)
            : cloneAttribution(options.attribution),
        mode: { kind: "normal" },
        references: cloneReferences(options.references ?? []),
      },
      this.idFactory,
    );
  }

  /** Create another execution of the same logical work. */
  retry(): Provenance {
    this.validate();
    if (this.attempt === Number.MAX_SAFE_INTEGER) {
      throw new AmberTransitionError("attempt overflow");
    }
    return Provenance.create(
      {
        version: VERSION,
        work_id: this.work_id,
        execution_id: this.idFactory(),
        correlation_id: this.correlation_id,
        causation: { kind: "execution", id: this.execution_id },
        origin: "retry",
        depth: this.depth,
        attempt: this.attempt + 1,
        attribution: cloneAttribution(this.state.attribution),
        mode: { kind: "retry", of_execution_id: this.execution_id },
        references: cloneReferences(this.references),
      },
      this.idFactory,
    );
  }

  /** Create an intentional re-execution of the same logical work. */
  replay(): Provenance {
    this.validate();
    return Provenance.create(
      {
        version: VERSION,
        work_id: this.work_id,
        execution_id: this.idFactory(),
        correlation_id: this.correlation_id,
        causation: { kind: "execution", id: this.execution_id },
        origin: "replay",
        depth: this.depth,
        attempt: this.attempt,
        attribution: cloneAttribution(this.state.attribution),
        mode: {
          kind: "replay",
          of_execution_id: this.execution_id,
          replay_id: this.idFactory(),
        },
        references: cloneReferences(this.references),
      },
      this.idFactory,
    );
  }

  get version(): number {
    return this.state.version;
  }

  get work_id(): ID {
    return this.state.work_id;
  }

  get execution_id(): ID {
    return this.state.execution_id;
  }

  get correlation_id(): ID {
    return this.state.correlation_id;
  }

  get origin(): Origin {
    return this.state.origin;
  }

  get depth(): number {
    return this.state.depth;
  }

  get attempt(): number {
    return this.state.attempt;
  }

  get mode(): Mode {
    return cloneMode(this.state.mode);
  }

  get causation(): Causation | undefined {
    return cloneCausation(this.state.causation);
  }

  get attribution(): Attribution | undefined {
    return cloneAttribution(this.state.attribution);
  }

  get references(): readonly Reference[] {
    return cloneReferences(this.state.references);
  }

  /** Validate this value against the Amber v1 invariants. */
  validate(): void {
    validateState(this.state);
  }

  /** Return the canonical JSON-compatible representation. */
  toJSON(): WireProvenance {
    this.validate();
    return {
      version: this.version,
      work_id: this.work_id,
      execution_id: this.execution_id,
      correlation_id: this.correlation_id,
      causation: cloneCausation(this.causation),
      origin: this.origin,
      depth: this.depth,
      attempt: this.attempt,
      attribution: cloneAttribution(this.attribution),
      mode: cloneMode(this.mode),
      references: this.references.length === 0 ? undefined : cloneReferences(this.references),
    };
  }

  /** Decode and validate a JSON string or already-parsed JSON object. */
  static fromJSON(input: string | unknown, idFactory: IdFactory = defaultIdFactory): Provenance {
    let value: unknown = input;
    if (typeof input === "string") {
      try {
        value = JSON.parse(input) as unknown;
      } catch (error) {
        throw new AmberValidationError(`invalid JSON: ${String(error)}`);
      }
    }
    if (!isRecord(value)) {
      throw new AmberValidationError("value must be a JSON object");
    }

    const state: State = {
      version: value.version as number,
      work_id: value.work_id as ID,
      execution_id: value.execution_id as ID,
      correlation_id: value.correlation_id as ID,
      causation: value.causation as Causation | undefined,
      origin: value.origin as Origin,
      depth: value.depth as number,
      attempt: value.attempt as number,
      attribution: value.attribution as Attribution | undefined,
      mode: value.mode as Mode,
      references: (value.references === undefined
        ? []
        : value.references) as readonly Reference[],
    };
    const provenance = new Provenance(immutableState(state), idFactory);
    provenance.validate();
    return provenance;
  }

  private static create(state: State, idFactory: IdFactory): Provenance {
    const provenance = new Provenance(immutableState(state), idFactory);
    provenance.validate();
    return provenance;
  }
}

function validateState(value: unknown): asserts value is State {
  if (!isRecord(value)) {
    throw new AmberValidationError("value must be an object");
  }
  if (value.version !== VERSION) {
    throw new AmberValidationError(`unsupported version ${String(value.version)}`);
  }
  for (const [name, id] of [
    ["work_id", value.work_id],
    ["execution_id", value.execution_id],
    ["correlation_id", value.correlation_id],
  ] as const) {
    validateId(id, name);
  }
  if (!["local", "incoming", "retry", "replay"].includes(String(value.origin))) {
    throw new AmberValidationError(`unsupported origin ${String(value.origin)}`);
  }
  validateNumber(value.depth, "depth", 0);
  validateNumber(value.attempt, "attempt", 1);
  validateMode(value.mode, value.execution_id as ID);

  const origin = value.origin as Origin;
  const mode = value.mode as Mode;
  if ((origin === "retry") !== (mode.kind === "retry")) {
    throw new AmberValidationError("retry origin and mode must agree");
  }
  if ((origin === "replay") !== (mode.kind === "replay")) {
    throw new AmberValidationError("replay origin and mode must agree");
  }
  if (mode.kind === "normal" && origin !== "local" && origin !== "incoming") {
    throw new AmberValidationError("normal mode requires local or incoming origin");
  }

  if (value.causation !== undefined) {
    validateCausation(value.causation);
  }
  if (value.attribution !== undefined) {
    validateAttribution(value.attribution);
  }
  if (!Array.isArray(value.references)) {
    throw new AmberValidationError("references must be an array");
  }
  if (value.references.length > MAX_REFERENCES) {
    throw new AmberValidationError(`references exceed ${MAX_REFERENCES}`);
  }
  value.references.forEach((reference, index) => {
    try {
      validateReference(reference);
    } catch (error) {
      throw new AmberValidationError(`reference ${index}: ${String(error)}`);
    }
  });
}

function validateMode(value: unknown, executionId: ID): void {
  if (!isRecord(value) || typeof value.kind !== "string") {
    throw new AmberValidationError("mode must contain a kind");
  }
  switch (value.kind) {
    case "normal":
      if (value.of_execution_id !== undefined || value.replay_id !== undefined) {
        throw new AmberValidationError("normal mode cannot contain source IDs");
      }
      return;
    case "retry":
      validateSourceId(value.of_execution_id, executionId, "of_execution_id");
      if (value.replay_id !== undefined) {
        throw new AmberValidationError("retry mode cannot contain replay_id");
      }
      return;
    case "replay":
      validateSourceId(value.of_execution_id, executionId, "of_execution_id");
      validateSourceId(value.replay_id, executionId, "replay_id");
      return;
    default:
      throw new AmberValidationError(`unsupported mode ${value.kind}`);
  }
}

function validateSourceId(value: unknown, executionId: ID, field: string): void {
  validateId(value, field);
  if (value === executionId) {
    throw new AmberValidationError(`${field} must differ from execution_id`);
  }
}

function validateCausation(value: unknown): void {
  if (!isRecord(value) || typeof value.kind !== "string") {
    throw new AmberValidationError("causation must contain a kind");
  }
  if (!["execution", "event", "message", "request", "artifact"].includes(value.kind)) {
    throw new AmberValidationError(`unsupported causation kind ${value.kind}`);
  }
  validateId(value.id, "causation.id");
  if (value.type !== undefined) {
    validateString(value.type, "causation.type");
  }
}

function validateAttribution(value: unknown): void {
  if (!isRecord(value)) {
    throw new AmberValidationError("attribution must be an object");
  }
  for (const [name, actor] of [
    ["initiated_by", value.initiated_by],
    ["executed_by", value.executed_by],
    ["on_behalf_of", value.on_behalf_of],
  ] as const) {
    if (actor !== undefined) {
      validateActor(actor, name);
    }
  }
  if (value.tenant_id !== undefined) {
    validateString(value.tenant_id, "tenant_id");
  }
}

function validateActor(value: unknown, field: string): void {
  if (!isRecord(value) || typeof value.id !== "string" || typeof value.type !== "string") {
    throw new AmberValidationError(`${field} requires id and type`);
  }
  validateString(value.id, `${field}.id`);
  validateString(value.type, `${field}.type`);
}

function validateReference(value: unknown): void {
  if (!isRecord(value) || typeof value.type !== "string" || typeof value.id !== "string") {
    throw new AmberValidationError("type and id are required");
  }
  validateString(value.type, "type");
  validateString(value.id, "id");
  if (value.relation !== undefined) {
    validateString(value.relation, "relation");
  }
}

function validateId(value: unknown, field: string): asserts value is ID {
  if (typeof value !== "string" || !/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(value)) {
    throw new AmberValidationError(`${field} must be a canonical lowercase UUIDv4`);
  }
}

function validateNumber(value: unknown, field: string, minimum: number): void {
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < minimum) {
    throw new AmberValidationError(`${field} must be a safe integer >= ${minimum}`);
  }
}

function validateString(value: unknown, field: string): asserts value is string {
  if (typeof value !== "string" || value.length === 0) {
    throw new AmberValidationError(`${field} must be a non-empty string`);
  }
  if (Array.from(value).length > MAX_STRING_LENGTH) {
    throw new AmberValidationError(`${field} exceeds ${MAX_STRING_LENGTH} Unicode scalar values`);
  }
}

function cloneMode(value: Mode): Mode {
  return { ...value } as Mode;
}

function cloneCausation(value: Causation | undefined): Causation | undefined {
  return value === undefined ? undefined : { ...value };
}

function cloneActor(value: Actor | undefined): Actor | undefined {
  return value === undefined ? undefined : { ...value };
}

function cloneAttribution(value: Attribution | undefined): Attribution | undefined {
  if (value === undefined) {
    return undefined;
  }
  return {
    initiated_by: cloneActor(value.initiated_by),
    executed_by: cloneActor(value.executed_by),
    on_behalf_of: cloneActor(value.on_behalf_of),
    tenant_id: value.tenant_id,
  };
}

function cloneReference(value: Reference): Reference {
  return { ...value };
}

function cloneReferences(values: readonly Reference[]): readonly Reference[] {
  return values.map(cloneReference);
}

function immutableState(state: State): State {
  const attribution = cloneAttribution(state.attribution);
  if (attribution) {
    Object.freeze(attribution.initiated_by);
    Object.freeze(attribution.executed_by);
    Object.freeze(attribution.on_behalf_of);
    Object.freeze(attribution);
  }
  const causation = cloneCausation(state.causation);
  if (causation) {
    Object.freeze(causation);
  }
  const mode = cloneMode(state.mode);
  Object.freeze(mode);
  const references = state.references.map((reference) => Object.freeze(cloneReference(reference)));
  Object.freeze(references);
  return Object.freeze({
    ...state,
    causation,
    attribution,
    mode,
    references,
  });
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
