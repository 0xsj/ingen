import assert from "node:assert/strict";
import test from "node:test";
import { buildProviderManifest, sha256Hex } from "../src/provider.ts";

const plan = {
  schema: "ingen.mutation-plan/v1",
  status: "ready",
  mutations: [
    {
      sequence: 1,
      spec: {
        id: "status-200-create",
        plane: "implementation",
        operator: "response.status.replace",
        target: "POST /documents",
      },
    },
    {
      sequence: 2,
      spec: {
        id: "remove-name-create",
        plane: "implementation",
        operator: "response.field.remove",
        target: "POST /documents",
      },
    },
    {
      sequence: 3,
      spec: {
        id: "status-200-again",
        plane: "implementation",
        operator: "response.status.replace",
        target: "POST /documents",
      },
    },
  ],
};

function bytes(value) {
  return new TextEncoder().encode(JSON.stringify(value));
}

test("emits exact plan binding, deduplicated capabilities, and one entry per mutation", async () => {
  const planBytes = bytes(plan);
  const result = await buildProviderManifest(planBytes, {
    id: "typescript-fixture-provider",
    command: "/bin/echo",
    args: ["--address", "${SORA_ADDR}"],
    subjectRoot: ".",
  });
  const provider = result.mutation_provider;

  assert.equal(provider.schema, "ingen.mutation-provider/v1");
  assert.equal(provider.plan_schema, "ingen.mutation-plan/v1");
  assert.equal(provider.plan_sha256, await sha256Hex(planBytes));
  assert.deepEqual(provider.capabilities, [
    {
      plane: "implementation",
      operator: "response.status.replace",
      target: "POST /documents",
    },
    {
      plane: "implementation",
      operator: "response.field.remove",
      target: "POST /documents",
    },
  ]);
  assert.deepEqual(provider.entries.map((entry) => entry.mutation_id), [
    "status-200-create",
    "remove-name-create",
    "status-200-again",
  ]);
  assert.deepEqual(provider.entries[0].args, ["--address", "${SORA_ADDR}"]);
  assert.equal(provider.entries[0].variant, "status-200-create");
});

test("binds the exact bytes rather than a reserialized plan", async () => {
  const first = bytes(plan);
  const second = new TextEncoder().encode(`${JSON.stringify(plan)}\n`);
  const firstProvider = await buildProviderManifest(first, {
    id: "provider",
    command: "/bin/echo",
  });
  const secondProvider = await buildProviderManifest(second, {
    id: "provider",
    command: "/bin/echo",
  });

  assert.notEqual(
    firstProvider.mutation_provider.plan_sha256,
    secondProvider.mutation_provider.plan_sha256,
  );
});

test("rejects a plan outside the frozen schema or readiness state", async () => {
  await assert.rejects(
    buildProviderManifest(bytes({ ...plan, schema: "ingen.mutation-plan/v2" }), {
      id: "provider",
      command: "/bin/echo",
    }),
    /campaign plan\.schema must be ingen\.mutation-plan\/v1/,
  );
  await assert.rejects(
    buildProviderManifest(bytes({ ...plan, status: "draft" }), {
      id: "provider",
      command: "/bin/echo",
    }),
    /campaign plan\.status must be ready/,
  );
});
