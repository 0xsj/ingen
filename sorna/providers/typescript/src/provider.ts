export const PROVIDER_SCHEMA = "ingen.mutation-provider/v1" as const;
export const PLAN_SCHEMA = "ingen.mutation-plan/v1" as const;

export type MutationSpec = {
  readonly id: string;
  readonly plane: string;
  readonly operator: string;
  readonly target: string;
};

export type MutationPlan = {
  readonly schema: string;
  readonly status: string;
  readonly mutations: readonly {
    readonly sequence: number;
    readonly spec: MutationSpec;
  }[];
};

export type ProviderCapability = {
  readonly plane: string;
  readonly operator: string;
  readonly target: string;
};

export type ProviderEntry = {
  readonly mutation_id: string;
  readonly command: string;
  readonly args?: readonly string[];
  readonly subject_root?: string;
  readonly subject_dir?: string;
  readonly variant?: string;
};

export type ProviderManifest = {
  readonly schema: typeof PROVIDER_SCHEMA;
  readonly id: string;
  readonly version: 1;
  readonly plan_schema: typeof PLAN_SCHEMA;
  readonly plan_sha256: string;
  readonly capabilities: readonly ProviderCapability[];
  readonly entries: readonly ProviderEntry[];
};

export type ProviderEnvelope = {
  readonly mutation_provider: ProviderManifest;
};

export type ProviderBuildOptions = {
  readonly id: string;
  readonly command: string;
  readonly args?: readonly string[];
  readonly subjectRoot?: string;
  readonly subjectDir?: string;
};

function asRecord(value: unknown, name: string): Record<string, unknown> {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw new Error(`${name} must be an object`);
  }
  return value as Record<string, unknown>;
}

function nonEmptyString(value: unknown, name: string): string {
  if (typeof value !== "string" || value.trim() === "") {
    throw new Error(`${name} must be a non-empty string`);
  }
  return value;
}

function parsePlan(planBytes: Uint8Array): MutationPlan {
  let decoded: unknown;
  try {
    decoded = JSON.parse(new TextDecoder().decode(planBytes));
  } catch (error) {
    throw new Error(`parse campaign plan: ${String(error)}`);
  }

  const plan = asRecord(decoded, "campaign plan");
  if (plan.schema !== PLAN_SCHEMA) {
    throw new Error(`campaign plan.schema must be ${PLAN_SCHEMA}`);
  }
  if (plan.status !== "ready") {
    throw new Error("campaign plan.status must be ready");
  }
  if (!Array.isArray(plan.mutations) || plan.mutations.length === 0) {
    throw new Error("campaign plan.mutations must contain at least one mutation");
  }

  const seen = new Set<string>();
  const mutations = plan.mutations.map((rawMutation, index) => {
    const mutation = asRecord(rawMutation, `campaign plan.mutations[${index}]`);
    if (mutation.sequence !== index + 1) {
      throw new Error(`campaign plan.mutations[${index}].sequence must be ${index + 1}`);
    }
    const spec = asRecord(mutation.spec, `campaign plan.mutations[${index}].spec`);
    const parsedSpec: MutationSpec = {
      id: nonEmptyString(spec.id, `campaign plan.mutations[${index}].spec.id`),
      plane: nonEmptyString(spec.plane, `campaign plan.mutations[${index}].spec.plane`),
      operator: nonEmptyString(spec.operator, `campaign plan.mutations[${index}].spec.operator`),
      target: nonEmptyString(spec.target, `campaign plan.mutations[${index}].spec.target`),
    };
    if (seen.has(parsedSpec.id)) {
      throw new Error(`campaign plan mutation ID duplicates ${JSON.stringify(parsedSpec.id)}`);
    }
    seen.add(parsedSpec.id);
    return { sequence: index + 1, spec: parsedSpec };
  });

  return { schema: PLAN_SCHEMA, status: "ready", mutations };
}

export async function sha256Hex(bytes: Uint8Array): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", bytes as BufferSource);
  return Array.from(new Uint8Array(digest), (value) => value.toString(16).padStart(2, "0")).join("");
}

export async function buildProviderManifest(
  planBytes: Uint8Array,
  options: ProviderBuildOptions,
): Promise<ProviderEnvelope> {
  const id = nonEmptyString(options.id, "provider.id");
  const command = nonEmptyString(options.command, "provider.command");
  const plan = parsePlan(planBytes);
  const capabilities: ProviderCapability[] = [];
  const capabilityKeys = new Set<string>();
  const entries: ProviderEntry[] = [];
  const args = options.args ? [...options.args] : [];

  for (const mutation of plan.mutations) {
    const capability = {
      plane: mutation.spec.plane,
      operator: mutation.spec.operator,
      target: mutation.spec.target,
    };
    const capabilityKey = `${capability.plane}|${capability.operator}|${capability.target}`;
    if (!capabilityKeys.has(capabilityKey)) {
      capabilities.push(capability);
      capabilityKeys.add(capabilityKey);
    }
    entries.push({
      mutation_id: mutation.spec.id,
      command,
      ...(args.length > 0 ? { args: [...args] } : {}),
      ...(options.subjectRoot ? { subject_root: options.subjectRoot } : {}),
      ...(options.subjectDir ? { subject_dir: options.subjectDir } : {}),
      variant: mutation.spec.id,
    });
  }

  return {
    mutation_provider: {
      schema: PROVIDER_SCHEMA,
      id,
      version: 1,
      plan_schema: plan.schema,
      plan_sha256: await sha256Hex(planBytes),
      capabilities,
      entries,
    },
  };
}
